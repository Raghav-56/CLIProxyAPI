package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/codex"
	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	log "github.com/sirupsen/logrus"
)

const codexQuotaUsageURL = "https://chatgpt.com/backend-api/wham/usage"

func (m *Manager) CodexOAuthStrategy() string {
	if m == nil {
		return ""
	}
	cfg, _ := m.runtimeConfig.Load().(*internalconfig.Config)
	if cfg == nil {
		return ""
	}
	return cfg.Routing.CodexOAuthStrategy
}

func (m *Manager) automaticCodexQuotaEnabled() bool {
	if m == nil {
		return false
	}
	cfg, _ := m.runtimeConfig.Load().(*internalconfig.Config)
	return cfg != nil && cfg.Routing.CodexOAuthStrategy == "soonest-reset" &&
		cfg.Routing.CodexOAuthAuthID == "" && !cfg.Home.Enabled
}

// RunCodexQuotaRefresh refreshes automatic routing observations until ctx is cancelled.
// A sweep completes before another starts, bounding work even when an upstream stalls.
func (m *Manager) RunCodexQuotaRefresh(ctx context.Context) {
	if m == nil || ctx == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		m.refreshCodexQuotas(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) refreshCodexQuotas(ctx context.Context) {
	if ctx.Err() != nil || !m.automaticCodexQuotaEnabled() {
		return
	}
	auths := m.List()
	jobs := make(chan *Auth)
	var workers sync.WaitGroup
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for auth := range jobs {
				if ctx.Err() == nil && m.automaticCodexQuotaEnabled() {
					m.refreshCodexQuota(ctx, auth)
				}
			}
		}()
	}
	for _, auth := range auths {
		if !codexQuotaProbeEligible(auth) {
			continue
		}
		select {
		case jobs <- auth:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func codexQuotaProbeEligible(auth *Auth) bool {
	return isPersistentCodexOAuth(auth) && !auth.Disabled && auth.Status != StatusDisabled && !auth.RequiresSignIn
}

func (m *Manager) refreshCodexQuota(ctx context.Context, auth *Auth) {
	startedAt := time.Now()
	ctx = WithCodexClientRequest(ctx, true)
	req, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, codexQuotaUsageURL, nil)
	if errRequest != nil {
		return
	}
	req.Header.Set("Accept", "application/json")
	if accountID := codexQuotaAccountID(auth); accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}
	resp, errResponse := m.HttpRequest(ctx, auth, req)
	var headers http.Header
	if resp != nil {
		if resp.Body != nil {
			defer func() {
				if errClose := resp.Body.Close(); errClose != nil {
					log.Debug("codex quota response body close failed")
				}
			}()
		}
		if errResponse == nil && resp.StatusCode == http.StatusOK && resp.Body != nil {
			const maxBody = 1 << 20
			body, errRead := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
			if errRead == nil && len(body) <= maxBody {
				headers, _ = parseCodexUsageHeaders(body)
			}
		}
	}
	if ctx.Err() != nil {
		return
	}
	m.applyCodexQuotaProbe(auth, startedAt, headers)
}

func codexQuotaAccountID(auth *Auth) string {
	for _, key := range []string{"account_id", "chatgpt_account_id"} {
		if value := strings.TrimSpace(authMetadataString(auth, key)); value != "" {
			return value
		}
	}
	if claims, errParse := codex.ParseJWTToken(authMetadataString(auth, "id_token")); errParse == nil {
		return strings.TrimSpace(claims.GetAccountID())
	}
	return ""
}

func (m *Manager) applyCodexQuotaProbe(snapshot *Auth, startedAt time.Time, headers http.Header) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.auths[snapshot.ID]
	if !m.automaticCodexQuotaEnabled() || !codexQuotaProbeEligible(current) ||
		current.RegistrationEpoch != snapshot.RegistrationEpoch || current.CredentialVersion != snapshot.CredentialVersion ||
		secretGrantChanged(current, snapshot) || current.Quota.ObservedAt.After(startedAt) {
		return
	}
	if len(headers) == 0 {
		if current.Quota.ClearObservationSignals() {
			current.Generation++
		}
		return
	}
	if current.Quota.ObserveResponseHeadersForProvider("codex", headers, startedAt) {
		current.Generation++
	}
}

func parseCodexUsageHeaders(body []byte) (http.Header, error) {
	var root struct {
		Plan      json.RawMessage            `json:"plan_type"`
		RateLimit map[string]json.RawMessage `json:"rate_limit"`
	}
	if errDecode := json.Unmarshal(body, &root); errDecode != nil || root.RateLimit == nil {
		return nil, errors.New("invalid Codex quota response")
	}
	headers := make(http.Header)
	if len(root.Plan) > 0 {
		var plan string
		if json.Unmarshal(root.Plan, &plan) == nil {
			headers.Set("X-Codex-Plan-Type", plan)
		}
	}
	for _, flag := range []struct{ field, header string }{{"allowed", "Allowed"}, {"limit_reached", "Limit-Reached"}} {
		if raw, exists := root.RateLimit[flag.field]; exists {
			headers.Set("X-Codex-"+flag.header, string(raw))
		}
	}
	for _, name := range []string{"primary", "secondary"} {
		raw := root.RateLimit[name+"_window"]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		prefix := "X-Codex-" + strings.ToUpper(name[:1]) + name[1:] + "-"
		var window map[string]json.RawMessage
		if json.Unmarshal(raw, &window) != nil || window == nil {
			headers.Set(prefix+"Used-Percent", "invalid")
			continue
		}
		// Preserve malformed reported fields so ranking cannot mistake them for absent windows.
		headers.Set(prefix+"Used-Percent", string(window["used_percent"]))
		var seconds int64
		if json.Unmarshal(window["limit_window_seconds"], &seconds) == nil && seconds > 0 && seconds%60 == 0 {
			headers.Set(prefix+"Window-Minutes", strconv.FormatInt(seconds/60, 10))
		} else {
			headers.Set(prefix+"Window-Minutes", "invalid")
		}
		for _, field := range []struct{ field, header string }{{"reset_at", "Reset-At"}, {"reset_after_seconds", "Reset-After-Seconds"}} {
			if value, exists := window[field.field]; exists {
				headers.Set(prefix+field.header, string(value))
			}
		}
	}
	if len(headers) == 0 {
		return nil, errors.New("missing Codex quota observations")
	}
	return headers, nil
}
