package auth

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

const codexQuotaMaxAge = 5 * time.Minute

// CodexQuotaSelector prefers fresh subscription quota for new session bindings.
// It belongs inside SessionAffinitySelector so healthy bindings stay on their account.
type CodexQuotaSelector struct {
	fallback Selector
	now      func() time.Time
}

func NewCodexQuotaSelector(fallback Selector) *CodexQuotaSelector {
	if fallback == nil {
		fallback = &RoundRobinSelector{}
	}
	return &CodexQuotaSelector{fallback: fallback, now: time.Now}
}

func (s *CodexQuotaSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	if (provider != "codex" && provider != "mixed") || accountPolicyFromContext(ctx).authID != "" {
		return s.fallback.Pick(ctx, provider, model, opts, auths)
	}
	for _, auth := range auths {
		if !isPersistentCodexOAuth(auth) {
			return s.fallback.Pick(ctx, provider, model, opts, auths)
		}
	}
	if usesWeightedRouting(s.fallback) {
		auths = positiveWeightAuths(auths)
	}
	now := s.now()
	available, errAvailable := getSelectorAvailableAuths(ctx, auths, provider, model, now)
	if errAvailable != nil {
		return nil, errAvailable
	}
	available = preferCodexWebsocketAuths(ctx, "codex", available)
	var best *Auth
	var bestRank codexQuotaRank
	var unknown []*Auth
	var earliest time.Time
	for _, auth := range available {
		rank, state := codexQuotaRankAt(auth, now)
		switch state {
		case codexQuotaUsable:
			if best == nil || rank.less(bestRank) || (rank == bestRank && auth.ID < best.ID) {
				best, bestRank = auth, rank
			}
		case codexQuotaUnknown:
			unknown = append(unknown, auth)
		case codexQuotaDepleted:
			if !rank.resetsAt.IsZero() && (earliest.IsZero() || rank.resetsAt.Before(earliest)) {
				earliest = rank.resetsAt
			}
		}
	}
	if best != nil {
		return best, nil
	}
	if len(unknown) > 0 {
		return s.fallback.Pick(ctx, provider, model, opts, unknown)
	}
	return nil, newAuthUnavailableError(earliest, now)
}

func isPersistentCodexOAuth(auth *Auth) bool {
	return auth != nil && strings.EqualFold(strings.TrimSpace(auth.Provider), "codex") &&
		auth.AuthKind() == AuthKindOAuth && strings.TrimSpace(auth.FileName) != "" &&
		auth.AuthSourceKind() != AuthSourceConfig && auth.AuthSourceKind() != AuthSourceMemory &&
		!IsPluginVirtualAuth(auth) && !IsConfigAPIKeyAuth(auth)
}

func usesWeightedRouting(selector Selector) bool {
	switch current := selector.(type) {
	case *WeightedRoundRobinSelector:
		return true
	case *CodexQuotaSelector:
		return usesWeightedRouting(current.fallback)
	default:
		return false
	}
}

type codexQuotaState uint8

const (
	codexQuotaUnknown codexQuotaState = iota
	codexQuotaUsable
	codexQuotaDepleted
)

type codexQuotaRank struct {
	resetsAt       time.Time
	shortRemaining float64
	longRemaining  float64
}

func (r codexQuotaRank) less(other codexQuotaRank) bool {
	if !r.resetsAt.Equal(other.resetsAt) {
		return r.resetsAt.Before(other.resetsAt)
	}
	if r.shortRemaining != other.shortRemaining {
		return r.shortRemaining > other.shortRemaining
	}
	return r.longRemaining > other.longRemaining
}

func codexQuotaRankAt(auth *Auth, now time.Time) (codexQuotaRank, codexQuotaState) {
	q := auth.Quota
	if q.ObservedAt.IsZero() || q.ObservedAt.After(now) || now.Sub(q.ObservedAt) > codexQuotaMaxAge {
		return codexQuotaRank{}, codexQuotaUnknown
	}
	depleted := q.Signals["X-Codex-Allowed"] == "false" || q.Signals["X-Codex-Limit-Reached"] == "true"
	malformed, crossed := false, false
	for _, flag := range []string{"X-Codex-Allowed", "X-Codex-Limit-Reached"} {
		if value, exists := q.Signals[flag]; exists && value != "true" && value != "false" {
			malformed = true
		}
	}
	var rank codexQuotaRank
	var shortest, longest int64
	count := 0
	for _, name := range []string{"Primary", "Secondary"} {
		prefix := "X-Codex-" + name + "-"
		reported := false
		for key := range q.Signals {
			if strings.HasPrefix(key, prefix) {
				reported = true
				break
			}
		}
		if !reported {
			continue
		}
		used, errUsed := strconv.ParseFloat(q.Signals[prefix+"Used-Percent"], 64)
		validUsed := errUsed == nil && !math.IsNaN(used) && !math.IsInf(used, 0) && used >= 0 && used <= 100
		minutes, errMinutes := strconv.ParseInt(q.Signals[prefix+"Window-Minutes"], 10, 64)
		reset, validReset := codexQuotaReset(q.Signals, prefix, q.ObservedAt)
		if validUsed && used == 100 {
			depleted = true
		}
		if !validUsed || errMinutes != nil || minutes <= 0 || !validReset {
			malformed = true
			continue
		}
		if !reset.After(now) {
			crossed = true
			continue
		}
		if rank.resetsAt.IsZero() || reset.Before(rank.resetsAt) {
			// A depleted account can be retried after its first known reset.
			rank.resetsAt = reset
		}
		if shortest == 0 || minutes < shortest {
			shortest, rank.shortRemaining = minutes, 100-used
		}
		if minutes > longest {
			longest, rank.longRemaining = minutes, 100-used
		}
		count++
	}
	if crossed {
		return codexQuotaRank{}, codexQuotaUnknown
	}
	if depleted {
		return rank, codexQuotaDepleted
	}
	if malformed || count == 0 {
		return codexQuotaRank{}, codexQuotaUnknown
	}
	// Rank by the longest subscription window, independent of primary/secondary ordering.
	for _, name := range []string{"Primary", "Secondary"} {
		prefix := "X-Codex-" + name + "-"
		minutes, _ := strconv.ParseInt(q.Signals[prefix+"Window-Minutes"], 10, 64)
		if minutes == longest {
			rank.resetsAt, _ = codexQuotaReset(q.Signals, prefix, q.ObservedAt)
			break
		}
	}
	return rank, codexQuotaUsable
}

func codexQuotaReset(signals map[string]string, prefix string, observedAt time.Time) (time.Time, bool) {
	if raw, exists := signals[prefix+"Reset-At"]; exists {
		seconds, errSeconds := strconv.ParseInt(raw, 10, 64)
		if errSeconds != nil || seconds <= 0 {
			return time.Time{}, false
		}
		return time.Unix(seconds, 0), true
	}
	seconds, errSeconds := strconv.ParseInt(signals[prefix+"Reset-After-Seconds"], 10, 64)
	if errSeconds != nil || seconds < 0 || seconds > int64(math.MaxInt64)/int64(time.Second) {
		return time.Time{}, false
	}
	return observedAt.Add(time.Duration(seconds) * time.Second), true
}
