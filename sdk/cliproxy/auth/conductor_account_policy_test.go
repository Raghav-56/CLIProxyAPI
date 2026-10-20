package auth

import (
	"context"
	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"net/http"
	"testing"
	"time"
)

func policyManager(t *testing.T, selector Selector) *Manager {
	t.Helper()
	m := NewManager(nil, selector, nil)
	m.RegisterExecutor(schedulerTestExecutor{provider: "codex"})
	m.RegisterExecutor(schedulerTestExecutor{provider: "gemini"})
	for _, a := range []*Auth{
		{ID: "policy-a", Provider: "codex", Metadata: map[string]any{"auth_kind": "oauth"}},
		{ID: "policy-b", Provider: "codex", Metadata: map[string]any{"auth_kind": "oauth"}},
		{ID: "policy-c", Provider: "codex", Metadata: map[string]any{"auth_kind": "oauth"}},
		{ID: "policy-key", Provider: "codex", Attributes: map[string]string{"api_key": "synthetic"}},
		{ID: "policy-gemini", Provider: "gemini"},
	} {
		if _, err := m.Register(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	m.SetConfig(&internalconfig.Config{Routing: internalconfig.RoutingConfig{CodexOAuthAuthID: "policy-b"}})
	return m
}

type accountCaptureExecutor struct {
	schedulerTestExecutor
	ids    []string
	change func()
}

func (e *accountCaptureExecutor) Execute(_ context.Context, a *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.ids = append(e.ids, a.ID)
	if e.change != nil {
		e.change()
		return cliproxyexecutor.Response{}, &Error{Code: "upstream", Message: "synthetic failure", HTTPStatus: 500}
	}
	return cliproxyexecutor.Response{}, nil
}
func (e *accountCaptureExecutor) CountTokens(ctx context.Context, a *Auth, r cliproxyexecutor.Request, o cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return e.Execute(ctx, a, r, o)
}
func (e *accountCaptureExecutor) ExecuteStream(ctx context.Context, a *Auth, r cliproxyexecutor.Request, o cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	_, err := e.Execute(ctx, a, r, o)
	if err != nil {
		return nil, err
	}
	chunks := make(chan cliproxyexecutor.StreamChunk, 1)
	chunks <- cliproxyexecutor.StreamChunk{Payload: []byte("ok")}
	close(chunks)
	return &cliproxyexecutor.StreamResult{Chunks: chunks}, nil
}

func TestCodexAccountPolicyExecuteCountStreamAndRetry(t *testing.T) {
	for _, method := range []string{"execute", "count", "stream", "retry"} {
		t.Run(method, func(t *testing.T) {
			m := policyManager(t, &RoundRobinSelector{})
			for _, id := range []string{"policy-a", "policy-b", "policy-c"} {
				registerSchedulerModels(t, "codex", "policy-model", id)
			}
			e := &accountCaptureExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}}
			m.RegisterExecutor(e)
			if method == "retry" {
				m.SetRetryConfig(1, 0, 0)
				m.SetConfig(&internalconfig.Config{RequestRetry: 1, DisableCooling: true, Routing: internalconfig.RoutingConfig{CodexOAuthAuthID: "policy-b"}})
				e.change = func() {
					m.SetConfig(&internalconfig.Config{RequestRetry: 1, DisableCooling: true, Routing: internalconfig.RoutingConfig{CodexOAuthAuthID: "policy-c"}})
				}
			}
			opts := cliproxyexecutor.Options{}
			req := cliproxyexecutor.Request{Model: "policy-model"}
			var err error
			switch method {
			case "count":
				_, err = m.ExecuteCount(context.Background(), []string{"codex"}, req, opts)
			case "stream":
				var result *cliproxyexecutor.StreamResult
				result, err = m.ExecuteStream(context.Background(), []string{"codex"}, req, opts)
				if err == nil {
					for range result.Chunks {
					}
				}
			default:
				_, err = m.Execute(context.Background(), []string{"codex"}, req, opts)
			}
			if method != "retry" && err != nil {
				t.Fatal(err)
			}
			if len(e.ids) == 0 {
				t.Fatal("no upstream execution")
			}
			for _, id := range e.ids {
				if id != "policy-b" {
					t.Fatalf("execution escaped snapshot: %v", e.ids)
				}
			}
			if method == "retry" && len(e.ids) < 2 {
				t.Fatalf("retry not exercised: %v error=%v", e.ids, err)
			}
			if method == "retry" && err == nil {
				t.Fatal("failed selected account silently fell back")
			}
		})
	}
}

func TestCodexAccountPolicyPluginCandidateFiltering(t *testing.T) {
	m := policyManager(t, &RoundRobinSelector{})
	scheduler := &fakePluginScheduler{handled: true, pick: func(_ context.Context, r pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error) {
		for _, a := range r.Candidates {
			if a.ID == "policy-a" || a.ID == "policy-c" {
				t.Fatalf("forbidden plugin candidate %s", a.ID)
			}
		}
		return pluginapi.SchedulerPickResponse{AuthID: "policy-b"}, true, nil
	}}
	m.SetPluginScheduler(scheduler)
	a, _, err := m.pickNext(context.Background(), "codex", "", cliproxyexecutor.Options{}, nil)
	if err != nil || a.ID != "policy-b" || scheduler.calls == 0 {
		t.Fatalf("plugin selection %v %v", a, err)
	}
}

func TestCodexAccountPolicyHomeFailsClosed(t *testing.T) {
	m := policyManager(t, &RoundRobinSelector{})
	cfg := &internalconfig.Config{Routing: internalconfig.RoutingConfig{CodexOAuthAuthID: "policy-b"}}
	cfg.Home.Enabled = true
	m.SetConfig(cfg)
	_, err := m.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: "policy-model"}, cliproxyexecutor.Options{})
	if err == nil || err.(*Error).Code != "codex_account_policy_home" {
		t.Fatalf("Home did not reject manual policy: %v", err)
	}
}

func TestCodexAccountPolicySelectionPaths(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "scheduler", true: "legacy"}[legacy], func(t *testing.T) {
			var selector Selector = &RoundRobinSelector{}
			if legacy {
				selector = &trackingSelector{}
			}
			m := policyManager(t, selector)
			opts := cliproxyexecutor.Options{}
			for i := 0; i < 9; i++ {
				a, _, err := m.pickNext(context.Background(), "codex", "", opts, nil)
				if err != nil {
					t.Fatal(err)
				}
				if a.ID != "policy-b" && a.ID != "policy-key" {
					t.Fatalf("forbidden subscription selected: %s", a.ID)
				}
			}
			a, _, err := m.pickNext(context.Background(), "gemini", "", opts, nil)
			if err != nil || a.ID != "policy-gemini" {
				t.Fatalf("other provider was constrained: %v %v", a, err)
			}
			a, _, _, err = m.pickNextMixed(context.Background(), []string{"codex", "gemini"}, "", opts, nil)
			if err != nil || a.ID == "policy-a" || a.ID == "policy-c" {
				t.Fatalf("mixed selection bypassed policy: %v %v", a, err)
			}
		})
	}
}

func TestCodexAccountPolicySnapshotSurvivesChangeAndRetry(t *testing.T) {
	m := policyManager(t, &RoundRobinSelector{})
	ctx := m.withAccountPolicySnapshot(context.Background())
	m.SetConfig(&internalconfig.Config{Routing: internalconfig.RoutingConfig{CodexOAuthAuthID: "policy-c"}})
	opts := cliproxyexecutor.Options{Metadata: map[string]any{cliproxyexecutor.PinnedAuthMetadataKey: "policy-b"}}
	a, _, err := m.pickNext(ctx, "codex", "", opts, nil)
	if err != nil || a.ID != "policy-b" {
		t.Fatalf("in-flight snapshot changed: %v %v", a, err)
	}
	if _, _, err = m.pickNext(ctx, "codex", "", opts, map[string]struct{}{"policy-b": {}}); err == nil {
		t.Fatal("retry escaped exhausted selected account")
	}
	if _, _, err = m.pickNext(context.Background(), "codex", "", opts, nil); err == nil {
		t.Fatal("stale explicit pin bypassed new policy")
	}
	opts.Metadata[cliproxyexecutor.PinnedAuthMetadataKey] = "policy-c"
	a, _, err = m.pickNext(context.Background(), "codex", "", opts, nil)
	if err != nil || a.ID != "policy-c" {
		t.Fatalf("next request did not observe switch: %v %v", a, err)
	}
}

func TestCodexAccountPolicyAutomaticRetainsBaseline(t *testing.T) {
	m := policyManager(t, &RoundRobinSelector{})
	m.SetConfig(&internalconfig.Config{})
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		a, _, err := m.pickNext(context.Background(), "codex", "", cliproxyexecutor.Options{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		seen[a.ID] = true
	}
	if !seen["policy-a"] || !seen["policy-c"] {
		t.Fatalf("automatic pool constrained: %v", seen)
	}
}

func TestCodexAccountPolicyOldAffinityCannotBypass(t *testing.T) {
	selector := NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{Fallback: &RoundRobinSelector{}, TTL: time.Hour})
	defer selector.Stop()
	m := policyManager(t, selector)
	registerSchedulerModels(t, "codex", "policy-affinity", "policy-a", "policy-b", "policy-c")
	selector.cache.Set("codex::header:policy-session::policy-affinity", "policy-a")
	opts := cliproxyexecutor.Options{Headers: http.Header{"X-Session-Id": []string{"policy-session"}}}
	a, _, err := m.pickNext(context.Background(), "codex", "policy-affinity", opts, nil)
	if err != nil || a.ID != "policy-b" {
		t.Fatalf("stale affinity bypassed manual choice: %v %v", a, err)
	}
}
