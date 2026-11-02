package auth

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func codexQuotaTestAuth(id string, now time.Time, shortUsed, longUsed float64, longReset time.Duration) *Auth {
	auth := &Auth{ID: id, FileName: id + ".json", Provider: "codex", Status: StatusActive,
		Metadata: map[string]any{"access_token": "synthetic", "account_id": "account-" + id}}
	auth.Quota.ObserveResponseHeadersForProvider("codex", http.Header{
		"X-Codex-Primary-Used-Percent":     {strconv.FormatFloat(shortUsed, 'f', -1, 64)},
		"X-Codex-Primary-Window-Minutes":   {"300"},
		"X-Codex-Primary-Reset-At":         {strconv.FormatInt(now.Add(time.Hour).Unix(), 10)},
		"X-Codex-Secondary-Used-Percent":   {strconv.FormatFloat(longUsed, 'f', -1, 64)},
		"X-Codex-Secondary-Window-Minutes": {"10080"},
		"X-Codex-Secondary-Reset-At":       {strconv.FormatInt(now.Add(longReset).Unix(), 10)},
	}, now)
	return auth
}

func TestCodexQuotaSelectorRanksResetThenRemainingThenID(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	selector := NewCodexQuotaSelector(&FillFirstSelector{})
	selector.now = func() time.Time { return now }
	for _, tc := range []struct {
		name  string
		auths []*Auth
		want  string
	}{
		{"earliest long reset", []*Auth{codexQuotaTestAuth("a", now, 10, 10, 48*time.Hour), codexQuotaTestAuth("b", now, 90, 90, 2*time.Hour)}, "b"},
		{"short quota tie", []*Auth{codexQuotaTestAuth("a", now, 70, 10, 2*time.Hour), codexQuotaTestAuth("b", now, 30, 80, 2*time.Hour)}, "b"},
		{"long quota tie", []*Auth{codexQuotaTestAuth("a", now, 20, 70, 2*time.Hour), codexQuotaTestAuth("b", now, 20, 30, 2*time.Hour)}, "b"},
		{"stable id tie", []*Auth{codexQuotaTestAuth("b", now, 20, 30, 2*time.Hour), codexQuotaTestAuth("a", now, 20, 30, 2*time.Hour)}, "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			picked, errPick := selector.Pick(context.Background(), "codex", "", cliproxyexecutor.Options{}, tc.auths)
			if errPick != nil || picked.ID != tc.want {
				t.Fatalf("picked %v, error %v, want %s", picked, errPick, tc.want)
			}
		})
	}
}

func TestCodexQuotaRankWindowOrderDurationAndInvalidData(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	for _, tc := range []struct {
		name   string
		change func(*Auth)
		want   codexQuotaState
	}{
		{"fresh", func(*Auth) {}, codexQuotaUsable},
		{"stale", func(a *Auth) { a.Quota.ObservedAt = now.Add(-codexQuotaMaxAge - time.Nanosecond) }, codexQuotaUnknown},
		{"future observation", func(a *Auth) { a.Quota.ObservedAt = now.Add(time.Second) }, codexQuotaUnknown},
		{"reset crossed", func(a *Auth) { a.Quota.Signals["X-Codex-Primary-Reset-At"] = strconv.FormatInt(now.Unix(), 10) }, codexQuotaUnknown},
		{"null percentage", func(a *Auth) { a.Quota.Signals["X-Codex-Primary-Used-Percent"] = "null" }, codexQuotaUnknown},
		{"NaN percentage", func(a *Auth) { a.Quota.Signals["X-Codex-Primary-Used-Percent"] = "NaN" }, codexQuotaUnknown},
		{"invalid reset", func(a *Auth) { a.Quota.Signals["X-Codex-Secondary-Reset-At"] = "null" }, codexQuotaUnknown},
		{"invalid flag", func(a *Auth) { a.Quota.Signals["X-Codex-Allowed"] = `"false"` }, codexQuotaUnknown},
		{"depleted short", func(a *Auth) { a.Quota.Signals["X-Codex-Primary-Used-Percent"] = "100" }, codexQuotaDepleted},
		{"depleted long", func(a *Auth) { a.Quota.Signals["X-Codex-Secondary-Used-Percent"] = "100" }, codexQuotaDepleted},
		{"depleted malformed reset", func(a *Auth) {
			a.Quota.Signals["X-Codex-Primary-Used-Percent"] = "100"
			a.Quota.Signals["X-Codex-Primary-Reset-At"] = "null"
		}, codexQuotaDepleted},
		{"disallowed", func(a *Auth) { a.Quota.Signals["X-Codex-Allowed"] = "false" }, codexQuotaDepleted},
		{"limit reached", func(a *Auth) { a.Quota.Signals["X-Codex-Limit-Reached"] = "true" }, codexQuotaDepleted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := codexQuotaTestAuth("a", now, 10, 20, 3*time.Hour)
			tc.change(auth)
			_, state := codexQuotaRankAt(auth, now)
			if state != tc.want {
				t.Fatalf("state = %v, want %v", state, tc.want)
			}
		})
	}
	for _, primaryMinutes := range []string{"10080", "43200"} {
		auth := codexQuotaTestAuth("a", now, 10, 20, 3*time.Hour)
		auth.Quota.Signals["X-Codex-Primary-Window-Minutes"] = primaryMinutes
		auth.Quota.Signals["X-Codex-Secondary-Window-Minutes"] = "300"
		rank, state := codexQuotaRankAt(auth, now)
		if state != codexQuotaUsable || !rank.resetsAt.Equal(now.Add(time.Hour)) || rank.longRemaining != 90 || rank.shortRemaining != 80 {
			t.Fatalf("primary duration %s produced rank %+v state %v", primaryMinutes, rank, state)
		}
	}
	auth := codexQuotaTestAuth("relative", now, 10, 20, 3*time.Hour)
	delete(auth.Quota.Signals, "X-Codex-Secondary-Reset-At")
	auth.Quota.Signals["X-Codex-Secondary-Reset-After-Seconds"] = "7200"
	rank, _ := codexQuotaRankAt(auth, now.Add(time.Minute))
	if !rank.resetsAt.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("relative reset was rebased: %v", rank.resetsAt)
	}
}

func TestCodexQuotaSelectorFallbackManualAndMixedPools(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	selector := NewCodexQuotaSelector(&FillFirstSelector{})
	selector.now = func() time.Time { return now }
	depleted := codexQuotaTestAuth("a", now, 100, 20, 3*time.Hour)
	unknown := codexQuotaTestAuth("b", now, 10, 20, time.Hour)
	unknown.Quota.ClearObservationSignals()
	usable := codexQuotaTestAuth("c", now, 10, 20, 4*time.Hour)
	for _, tc := range []struct {
		name     string
		ctx      context.Context
		provider string
		auths    []*Auth
		want     string
	}{
		{"known beats unknown", context.Background(), "codex", []*Auth{unknown, usable}, "c"},
		{"unknown fallback", context.Background(), "codex", []*Auth{depleted, unknown}, "b"},
		{"manual ignores observed depletion", context.WithValue(context.Background(), codexAccountPolicyContextKey{}, codexAccountPolicy{authID: "a"}), "codex", []*Auth{depleted}, "a"},
		{"other provider", context.Background(), "gemini", []*Auth{depleted, usable}, "a"},
		{"mixed provider", context.Background(), "mixed", []*Auth{depleted, {ID: "gemini", Provider: "gemini"}}, "a"},
		{"mixed label Codex-only pool", context.Background(), "mixed", []*Auth{depleted, usable}, "c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			picked, errPick := selector.Pick(tc.ctx, tc.provider, "", cliproxyexecutor.Options{}, tc.auths)
			if errPick != nil || picked.ID != tc.want {
				t.Fatalf("picked %v, error %v, want %s", picked, errPick, tc.want)
			}
		})
	}
	if picked, errPick := selector.Pick(context.Background(), "codex", "", cliproxyexecutor.Options{}, []*Auth{depleted}); picked != nil || errPick == nil {
		t.Fatalf("depleted pool selected %v, error %v", picked, errPick)
	}
	apiKey := &Auth{ID: "key", Provider: "codex", Attributes: map[string]string{AttributeAPIKey: "synthetic"}}
	picked, errPick := selector.Pick(context.Background(), "codex", "", cliproxyexecutor.Options{}, []*Auth{depleted, apiKey})
	if errPick != nil || picked.ID != "a" {
		t.Fatalf("mixed API-key pool changed base routing: %v %v", picked, errPick)
	}
	depleted.Attributes = map[string]string{"priority": "10"}
	picked, errPick = selector.Pick(context.Background(), "codex", "", cliproxyexecutor.Options{}, []*Auth{depleted, usable})
	if picked != nil || errPick == nil {
		t.Fatalf("quota ranking escaped priority tier: %v %v", picked, errPick)
	}
}

func TestCodexQuotaSelectorAffinitySticksAndFailsOver(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	quota := NewCodexQuotaSelector(&FillFirstSelector{})
	quota.now = func() time.Time { return now }
	sticky := NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{Fallback: quota, TTL: time.Hour})
	defer sticky.Stop()
	auths := []*Auth{codexQuotaTestAuth("a", now, 10, 20, time.Hour), codexQuotaTestAuth("b", now, 10, 20, 4*time.Hour)}
	pick := func(session, want string) {
		t.Helper()
		opts := cliproxyexecutor.Options{Headers: http.Header{"Session-Id": {session}}}
		picked, errPick := sticky.Pick(context.Background(), "codex", "", opts, auths)
		if errPick != nil || picked.ID != want {
			t.Fatalf("session %s picked %v error %v, want %s", session, picked, errPick, want)
		}
	}
	pick("warm", "a")
	auths[1].Quota.Signals["X-Codex-Secondary-Reset-At"] = strconv.FormatInt(now.Add(30*time.Minute).Unix(), 10)
	pick("warm", "a")
	pick("cold", "b")
	auths[0].Disabled = true
	pick("warm", "b")
}

func TestCodexQuotaSelectorWeightedAffinityDropsZeroWeight(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	quota := NewCodexQuotaSelector(&WeightedRoundRobinSelector{})
	quota.now = func() time.Time { return now }
	sticky := NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{Fallback: quota, TTL: time.Hour})
	defer sticky.Stop()
	auths := []*Auth{codexQuotaTestAuth("a", now, 10, 20, time.Hour), codexQuotaTestAuth("b", now, 10, 20, 4*time.Hour)}
	opts := cliproxyexecutor.Options{Headers: http.Header{"Session-Id": {"warm"}}}
	picked, errPick := sticky.Pick(context.Background(), "codex", "", opts, auths)
	if errPick != nil || picked.ID != "a" {
		t.Fatalf("initial pick %v %v", picked, errPick)
	}
	auths[0].Attributes = map[string]string{AttributeWeight: "0"}
	picked, errPick = sticky.Pick(context.Background(), "codex", "", opts, auths)
	if errPick != nil || picked.ID != "b" {
		t.Fatalf("zero-weight bound auth was reused: %v %v", picked, errPick)
	}
}

func TestCodexQuotaSelectorExecuteUsesMixedCodexPool(t *testing.T) {
	now := time.Now()
	quota := NewCodexQuotaSelector(&FillFirstSelector{})
	sticky := NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{Fallback: quota, TTL: time.Hour})
	defer sticky.Stop()
	manager := NewManager(nil, sticky, nil)
	capture := &accountCaptureExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}}
	manager.RegisterExecutor(capture)
	for _, auth := range []*Auth{
		codexQuotaTestAuth("mixed-quota-a", now, 10, 20, 4*time.Hour),
		codexQuotaTestAuth("mixed-quota-b", now, 20, 30, 2*time.Hour),
	} {
		if _, errRegister := manager.Register(context.Background(), auth); errRegister != nil {
			t.Fatal(errRegister)
		}
		registerSchedulerModels(t, "codex", "mixed-quota-model", auth.ID)
	}
	_, errExecute := manager.Execute(context.Background(), []string{"codex"},
		cliproxyexecutor.Request{Model: "mixed-quota-model"},
		cliproxyexecutor.Options{Headers: http.Header{"Session-Id": {"mixed-quota-session"}}})
	if errExecute != nil {
		t.Fatal(errExecute)
	}
	if len(capture.ids) != 1 || capture.ids[0] != "mixed-quota-b" {
		t.Fatalf("mixed execution chose %v, want [mixed-quota-b]", capture.ids)
	}
}
