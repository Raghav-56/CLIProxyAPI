package cliproxy

import (
	"context"
	"strconv"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func TestServiceCodexQuotaRoutingUsesConfiguredPreference(t *testing.T) {
	now := time.Now()
	auths := []*coreauth.Auth{}
	for i, id := range []string{"a", "b"} {
		auths = append(auths, &coreauth.Auth{
			ID: id, FileName: id + ".json", Provider: "codex",
			Metadata: map[string]any{"auth_kind": "oauth"},
			Quota: coreauth.QuotaState{ObservedAt: now, Signals: map[string]string{
				"X-Codex-Primary-Used-Percent":   "10",
				"X-Codex-Primary-Window-Minutes": "10080",
				"X-Codex-Primary-Reset-At":       strconv.FormatInt(now.Add(time.Duration(2-i)*time.Hour).Unix(), 10),
			}},
		})
	}
	cfg := &internalconfig.Config{Routing: internalconfig.RoutingConfig{Strategy: "fill-first", CodexOAuthStrategy: "soonest-reset", SessionAffinity: true}}
	selector := newRoutingSelector(normalizedRoutingRuntimeState(cfg))
	sticky, ok := selector.(*coreauth.SessionAffinitySelector)
	if !ok {
		t.Fatalf("root selector %T loses session-affinity integration", selector)
	}
	defer sticky.Stop()
	selected, err := selector.Pick(context.Background(), "codex", "", cliproxyexecutor.Options{}, auths)
	if err != nil || selected == nil || selected.ID != "b" {
		t.Fatalf("automatic config did not choose earlier reset: %v %v", selected, err)
	}
	for _, home := range []bool{false, true} {
		cfg.Home.Enabled = home
		if !home {
			cfg.Routing.CodexOAuthStrategy = ""
		} else {
			cfg.Routing.CodexOAuthStrategy = "soonest-reset"
		}
		fallback := newRoutingSelector(normalizedRoutingRuntimeState(cfg))
		defer fallback.(*coreauth.SessionAffinitySelector).Stop()
		selected, err = fallback.Pick(context.Background(), "codex", "", cliproxyexecutor.Options{}, auths)
		if err != nil || selected == nil || selected.ID != "a" {
			t.Fatalf("base strategy changed with home=%v: %v %v", home, selected, err)
		}
	}
}

func TestServiceRejectsInvalidCodexRoutingUpdate(t *testing.T) {
	original := &internalconfig.Config{}
	service := &Service{cfg: original}
	invalid := &internalconfig.Config{Routing: internalconfig.RoutingConfig{CodexOAuthStrategy: "unknown"}}
	if service.applyConfigUpdateWithAuthSynthesis(nil, invalid, true) || service.cfg != original {
		t.Fatal("invalid strategy replaced the running configuration")
	}
}
