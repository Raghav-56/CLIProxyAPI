package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
)

type codexQuotaProbeExecutor struct {
	schedulerTestExecutor
	request func(context.Context, *Auth, *http.Request) (*http.Response, error)
}

func (e *codexQuotaProbeExecutor) HttpRequest(ctx context.Context, auth *Auth, request *http.Request) (*http.Response, error) {
	return e.request(ctx, auth, request)
}

func codexQuotaProbeManager(t *testing.T) (*Manager, *Auth) {
	t.Helper()
	manager := NewManager(nil, &FillFirstSelector{}, nil)
	manager.SetConfig(&internalconfig.Config{Routing: internalconfig.RoutingConfig{CodexOAuthStrategy: "soonest-reset"}})
	auth, errRegister := manager.Register(context.Background(), codexQuotaTestAuth("probe", time.Now(), 20, 30, 2*time.Hour))
	if errRegister != nil {
		t.Fatal(errRegister)
	}
	return manager, auth
}

func codexQuotaProbeBody() string {
	return fmt.Sprintf(`{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":12,"limit_window_seconds":18000,"reset_at":%d},"secondary_window":{"used_percent":34,"limit_window_seconds":604800,"reset_after_seconds":7200}}}`, time.Now().Add(time.Hour).Unix())
}

func TestCodexQuotaProbeUsesOriginalOAuthAndAccountHeader(t *testing.T) {
	for _, withSIWC := range []bool{false, true} {
		t.Run(strconv.FormatBool(withSIWC), func(t *testing.T) {
			manager, auth := codexQuotaProbeManager(t)
			manager.mu.Lock()
			manager.auths[auth.ID].Metadata["use_siwc"] = withSIWC
			manager.auths[auth.ID].Metadata["siwc"] = map[string]any{"access_token": "nested-token"}
			auth = manager.auths[auth.ID].Clone()
			manager.mu.Unlock()
			var called bool
			manager.RegisterExecutor(&codexQuotaProbeExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}, request: func(ctx context.Context, candidate *Auth, req *http.Request) (*http.Response, error) {
				called = true
				if IsCodexUseSIWCForRequest(ctx, candidate) || !IsCodexClientRequest(ctx) {
					t.Error("usage probe used the nested SIWC credential partition")
				}
				if req.URL.String() != "https://chatgpt.com/backend-api/wham/usage" || req.Method != http.MethodGet || req.Header.Get("ChatGPT-Account-Id") != "account-probe" || req.Header.Get("Accept") != "application/json" {
					t.Errorf("incorrect quota request: %s %s headers=%v", req.Method, req.URL, req.Header)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(codexQuotaProbeBody()))}, nil
			}})
			manager.refreshCodexQuota(context.Background(), auth)
			actual, _ := manager.GetByID(auth.ID)
			if !called || actual.Quota.Signals["X-Codex-Primary-Used-Percent"] != "12" || actual.Quota.Signals["X-Codex-Secondary-Window-Minutes"] != "10080" {
				t.Fatalf("probe result was not published: called=%v signals=%v", called, actual.Quota.Signals)
			}
		})
	}
	auth := &Auth{Metadata: map[string]any{"id_token": "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"jwt-account"}}`)) + ".signature"}}
	if got := codexQuotaAccountID(auth); got != "jwt-account" {
		t.Fatalf("JWT account ID = %q", got)
	}
}

func TestCodexQuotaProbeRejectsObsoletePublication(t *testing.T) {
	for _, change := range []string{"removed", "registration", "credential version", "secret", "newer observation", "manual", "disabled strategy", "Home", "disabled auth"} {
		t.Run(change, func(t *testing.T) {
			manager, auth := codexQuotaProbeManager(t)
			entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			manager.RegisterExecutor(&codexQuotaProbeExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}, request: func(context.Context, *Auth, *http.Request) (*http.Response, error) {
				close(entered)
				<-release
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(codexQuotaProbeBody()))}, nil
			}})
			go func() {
				defer close(finished)
				manager.refreshCodexQuota(context.Background(), auth)
			}()
			<-entered
			manager.mu.Lock()
			current := manager.auths[auth.ID]
			switch change {
			case "removed":
				delete(manager.auths, auth.ID)
			case "registration":
				current.RegistrationEpoch++
			case "credential version":
				current.CredentialVersion++
			case "secret":
				current.Metadata["access_token"] = "replacement"
			case "newer observation":
				current.Quota.ObservedAt = time.Now().Add(time.Second)
			case "disabled auth":
				current.Disabled = true
			}
			manager.mu.Unlock()
			cfg := &internalconfig.Config{Routing: internalconfig.RoutingConfig{CodexOAuthStrategy: "soonest-reset"}}
			switch change {
			case "manual":
				cfg.Routing.CodexOAuthAuthID = auth.ID
			case "disabled strategy":
				cfg.Routing.CodexOAuthStrategy = ""
			case "Home":
				cfg.Home.Enabled = true
			}
			manager.SetConfig(cfg)
			close(release)
			<-finished
			actual, exists := manager.GetByID(auth.ID)
			if change == "removed" {
				if exists {
					t.Fatal("probe resurrected removed auth")
				}
			} else if !exists || actual.Quota.Signals["X-Codex-Primary-Used-Percent"] != "20" {
				t.Fatalf("obsolete probe changed observation: %v", actual)
			}
		})
	}
}

func TestCodexQuotaProbeFailureInvalidatesWithoutClearingCooldown(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{}`},
		{"malformed", http.StatusOK, `broken`},
		{"oversized", http.StatusOK, strings.Repeat("x", (1<<20)+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, auth := codexQuotaProbeManager(t)
			manager.mu.Lock()
			manager.auths[auth.ID].Quota.Exceeded = true
			manager.auths[auth.ID].Quota.Reason = "credential_quota"
			manager.auths[auth.ID].Quota.NextRecoverAt = time.Now().Add(time.Hour)
			manager.mu.Unlock()
			manager.RegisterExecutor(&codexQuotaProbeExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}, request: func(context.Context, *Auth, *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}})
			manager.refreshCodexQuota(context.Background(), auth)
			actual, _ := manager.GetByID(auth.ID)
			if len(actual.Quota.Signals) != 0 || !actual.Quota.ObservedAt.IsZero() || !actual.Quota.Exceeded || actual.Quota.Reason != "credential_quota" || actual.Quota.NextRecoverAt.IsZero() {
				t.Fatalf("failed probe changed cooldown or retained observation: %+v", actual.Quota)
			}
		})
	}
}

func TestCodexQuotaRefreshLoopCancelsAndSkipsInactiveMode(t *testing.T) {
	manager, _ := codexQuotaProbeManager(t)
	entered := make(chan struct{})
	manager.RegisterExecutor(&codexQuotaProbeExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}, request: func(ctx context.Context, _ *Auth, _ *http.Request) (*http.Response, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		manager.RunCodexQuotaRefresh(ctx)
	}()
	<-entered
	cancel()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("quota refresh did not stop after cancellation")
	}
	for _, mode := range []string{"disabled", "manual", "Home"} {
		cfg := &internalconfig.Config{Routing: internalconfig.RoutingConfig{CodexOAuthStrategy: "soonest-reset"}}
		switch mode {
		case "disabled":
			cfg.Routing.CodexOAuthStrategy = ""
		case "manual":
			cfg.Routing.CodexOAuthAuthID = "probe"
		case "Home":
			cfg.Home.Enabled = true
		}
		manager.SetConfig(cfg)
		manager.RegisterExecutor(&codexQuotaProbeExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}, request: func(context.Context, *Auth, *http.Request) (*http.Response, error) {
			t.Fatalf("probe ran in %s mode", mode)
			return nil, nil
		}})
		manager.refreshCodexQuotas(context.Background())
	}
}

func TestCodexQuotaRefreshBoundsConcurrentRequests(t *testing.T) {
	manager, _ := codexQuotaProbeManager(t)
	for index := range 7 {
		if _, errRegister := manager.Register(context.Background(), codexQuotaTestAuth(fmt.Sprintf("probe-%d", index), time.Now(), 20, 30, 2*time.Hour)); errRegister != nil {
			t.Fatal(errRegister)
		}
	}
	var active, maximum, count atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	manager.RegisterExecutor(&codexQuotaProbeExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}, request: func(context.Context, *Auth, *http.Request) (*http.Response, error) {
		current := active.Add(1)
		for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
		}
		count.Add(1)
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(codexQuotaProbeBody()))}, nil
	}})
	finished := make(chan struct{})
	go func() { defer close(finished); manager.refreshCodexQuotas(context.Background()) }()
	for range 4 {
		<-entered
	}
	close(release)
	<-finished
	if maximum.Load() != 4 || count.Load() != 8 {
		t.Fatalf("maximum %d, total %d; want 4 concurrent and 8 total", maximum.Load(), count.Load())
	}
}

func TestParseCodexUsageHeadersPreservesMalformedWindows(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	for _, tc := range []struct {
		body string
		want codexQuotaState
	}{
		{`{"rate_limit":{"primary_window":{"used_percent":100,"limit_window_seconds":18000,"reset_at":null}}}`, codexQuotaDepleted},
		{`{"rate_limit":{"primary_window":{"used_percent":null,"limit_window_seconds":18000,"reset_at":2000003600}}}`, codexQuotaUnknown},
		{`{"rate_limit":{"primary_window":"malformed","secondary_window":{"used_percent":5,"limit_window_seconds":604800,"reset_at":2000003600}}}`, codexQuotaUnknown},
		{`{"rate_limit":{"primary_window":{"used_percent":5,"limit_window_seconds":2592000,"reset_at":2000003600},"secondary_window":null}}`, codexQuotaUsable},
	} {
		headers, errParse := parseCodexUsageHeaders([]byte(tc.body))
		if errParse != nil {
			t.Fatal(errParse)
		}
		auth := codexQuotaTestAuth("parse", now, 20, 30, time.Hour)
		auth.Quota.ObserveResponseHeadersForProvider("codex", headers, now)
		if _, state := codexQuotaRankAt(auth, now); state != tc.want {
			t.Fatalf("body %s produced state %v, want %v", tc.body, state, tc.want)
		}
	}
}

func TestCodexQuotaResetRecovery(t *testing.T) {
	for _, scenario := range []string{"manual", "automatic", "newer failure", "unauthorized", "depleted", "malformed", "Home", "independent model cooldown"} {
		t.Run(scenario, func(t *testing.T) {
			manager, auth := codexQuotaProbeManager(t)
			reg := registry.GetGlobalRegistry()
			reg.RegisterClient(auth.ID, "codex", []*registry.ModelInfo{{ID: "reporter"}, {ID: "sibling"}})
			t.Cleanup(func() { reg.UnregisterClient(auth.ID) })
			store := &recordingCooldownStateStore{}
			manager.cooldownStore = store
			retry := time.Hour
			if scenario == "independent model cooldown" {
				manager.MarkResult(context.Background(), Result{AuthID: auth.ID, Provider: "codex", Model: "sibling", Error: &Error{HTTPStatus: 500, Message: "server error"}})
			}
			manager.MarkResult(context.Background(), Result{AuthID: auth.ID, Provider: "codex", Model: "reporter", CredentialScope: true, RetryAfter: &retry, Error: &Error{HTTPStatus: 429, Message: "usage_limit_reached"}})
			auth, _ = manager.GetByID(auth.ID)
			startedAt := time.Now()
			if scenario == "newer failure" {
				startedAt = auth.UpdatedAt.Add(-time.Second)
			}
			if scenario == "unauthorized" {
				manager.MarkResult(context.Background(), Result{AuthID: auth.ID, Provider: "codex", Model: "reporter", Error: &Error{HTTPStatus: 401, Message: "unauthorized"}})
			}
			body := codexQuotaProbeBody()
			if scenario == "depleted" {
				body = strings.Replace(body, `"used_percent":34`, `"used_percent":100`, 1)
			}
			if scenario == "malformed" {
				body = `{}`
			}
			if scenario == "automatic" {
				headers, errParse := parseCodexUsageHeaders([]byte(body))
				if errParse != nil {
					t.Fatal(errParse)
				}
				manager.applyCodexQuotaProbe(auth, startedAt, headers)
			} else {
				cfg := &internalconfig.Config{Routing: internalconfig.RoutingConfig{CodexOAuthAuthID: auth.ID}}
				cfg.Home.Enabled = scenario == "Home"
				manager.SetConfig(cfg)
				manager.ObserveCodexQuotaUsage(auth, startedAt, []byte(body))
			}
			actual, _ := manager.GetByID(auth.ID)
			if scenario == "Home" {
				if actual.Quota.Signals["X-Codex-Primary-Used-Percent"] != "20" {
					t.Fatal("local quota lookup changed Home-managed observation")
				}
				return
			}
			wantBlocked := scenario == "newer failure" || scenario == "unauthorized" || scenario == "depleted" || scenario == "malformed"
			blocked, _, _ := isAuthBlockedForModel(actual, "reporter", time.Now())
			if blocked != wantBlocked {
				t.Fatalf("reporter blocked = %v, want %v; auth=%+v", blocked, wantBlocked, actual)
			}
			if wantBlocked {
				return
			}
			if actual.Quota.Exceeded || !actual.Quota.NextRecoverAt.IsZero() {
				t.Fatalf("credential cooldown survived: %+v", actual.Quota)
			}
			if reg.GetModelCount("reporter") != 1 {
				t.Fatal("registry did not resume reporter")
			}
			if scenario == "independent model cooldown" {
				if blocked, _, _ := isAuthBlockedForModel(actual, "sibling", time.Now()); !blocked {
					t.Fatal("independent cooldown was cleared")
				}
			} else {
				if actual.Unavailable || actual.Status != StatusActive || len(store.savedRecords()) != 0 {
					t.Fatalf("recovered state still blocked or persisted: %+v, records=%v", actual, store.savedRecords())
				}
			}
		})
	}
}
