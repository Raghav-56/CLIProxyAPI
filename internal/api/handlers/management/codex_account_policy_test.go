package management

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexAutomaticRoutingSwitchesAndPreservesManualOverride(t *testing.T) {
	h, path := accountPolicyHandler(t)
	for _, step := range []struct {
		body     string
		id       string
		strategy string
		mode     string
	}{
		{`{"auth_id":null,"strategy":"soonest-reset"}`, "", "soonest-reset", "automatic"},
		{`{"auth_id":"b"}`, "b", "soonest-reset", "manual"},
		{`{"auth_id":null}`, "", "soonest-reset", "automatic"},
		{`{"auth_id":null,"strategy":""}`, "", "", "automatic"},
	} {
		r := policyPUT(h, step.body)
		if r.Code != http.StatusOK {
			t.Fatalf("switch %s: %d %s", step.body, r.Code, r.Body)
		}
		var result struct {
			Mode     string `json:"mode"`
			Strategy string `json:"strategy"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil || result.Mode != step.mode || result.Strategy != step.strategy {
			t.Fatalf("wrong mode after %s: %s %v", step.body, r.Body, err)
		}
		disk, err := config.LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if disk.Routing.CodexOAuthAuthID != step.id || disk.Routing.CodexOAuthStrategy != step.strategy || h.authManager.CodexOAuthAuthID() != step.id || h.authManager.CodexOAuthStrategy() != step.strategy {
			t.Fatalf("disk/runtime choice differs after %s", step.body)
		}
	}
}

func TestCodexAutomaticRoutingRejectsInvalidStrategyAndHome(t *testing.T) {
	h, path := accountPolicyHandler(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, strategy := range []string{`null`, `42`, `"round-robin"`, `{}`, `" soonest-reset "`} {
		r := policyPUT(h, `{"auth_id":null,"strategy":`+strategy+`}`)
		if r.Code != http.StatusBadRequest {
			t.Fatalf("accepted strategy %s: %d %s", strategy, r.Code, r.Body)
		}
	}
	homeCfg := h.cfg.CloneForRuntime()
	homeCfg.Home.Enabled = true
	h.authManager.SetConfig(homeCfg)
	r := policyPUT(h, `{"auth_id":null,"strategy":"soonest-reset"}`)
	if r.Code != http.StatusConflict {
		t.Fatalf("Home accepted local automatic routing: %d %s", r.Code, r.Body)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) || h.cfg.Routing.CodexOAuthStrategy != "" {
		t.Fatalf("rejected policy changed saved configuration: %v", err)
	}
}

func TestCodexAutomaticRoutingDetectsRuntimeMismatchAndRollsBackSaveFailure(t *testing.T) {
	h, path := accountPolicyHandler(t)
	h.configFilePath = filepath.Join(path, "missing.yaml")
	r := policyPUT(h, `{"auth_id":null,"strategy":"soonest-reset"}`)
	if r.Code != http.StatusInternalServerError || h.cfg.Routing.CodexOAuthStrategy != "" || h.authManager.CodexOAuthStrategy() != "" {
		t.Fatalf("failed save changed automatic policy: %d %s", r.Code, r.Body)
	}
	h.configFilePath = path
	h.SetConfigReloadHook(func(context.Context, *config.Config) {})
	r = policyPUT(h, `{"auth_id":null,"strategy":"soonest-reset"}`)
	if r.Code != http.StatusConflict {
		t.Fatalf("missed strategy-only runtime mismatch: %d %s", r.Code, r.Body)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	h.GetCodexAccountRouting(c)
	var state struct {
		Applied bool `json:"applied"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil || state.Applied {
		t.Fatalf("GET falsely reports routing applied: %s %v", rec.Body, err)
	}
}

func accountPolicyHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("config-version: 8\nserver: {port: 8317}\nrouting: {strategy: fill-first}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	m := coreauth.NewManager(nil, nil, nil)
	for _, a := range []*coreauth.Auth{
		{ID: "a", FileName: "a.json", Provider: "codex", Metadata: map[string]any{"auth_kind": "oauth"}},
		{ID: "b", FileName: "b.json", Provider: "codex", Metadata: map[string]any{"auth_kind": "oauth"}},
		{ID: "disabled", FileName: "disabled.json", Provider: "codex", Disabled: true, Metadata: map[string]any{"auth_kind": "oauth"}},
		{ID: "key", FileName: "key.json", Provider: "codex", Attributes: map[string]string{"api_key": "synthetic"}},
		{ID: "runtime", Provider: "codex", Attributes: map[string]string{"runtime_only": "true"}, Metadata: map[string]any{"auth_kind": "oauth"}},
		{ID: "virtual", FileName: "virtual.json", Provider: "codex", Attributes: map[string]string{coreauth.AttributePluginVirtual: "true"}, Metadata: map[string]any{"auth_kind": "oauth"}},
		{ID: "other", FileName: "other.json", Provider: "gemini", Metadata: map[string]any{"auth_kind": "oauth"}},
	} {
		if _, err := m.Register(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	m.SetConfig(cfg)
	h := NewHandler(cfg, path, m)
	h.SetConfigReloadHook(func(_ context.Context, cfg *config.Config) { m.SetConfig(cfg) })
	return h, path
}

func policyPUT(h *Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set(ConfigV8ContextKey, true)
	c.Request = httptest.NewRequest(http.MethodPut, "/v8/management/routing/codex-account", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.PutCodexAccountRouting(c)
	return rec
}

func TestCodexAccountPolicyPersistReloadIdempotent(t *testing.T) {
	h, path := accountPolicyHandler(t)
	r := policyPUT(h, `{"auth_id":"b"}`)
	if r.Code != 200 {
		t.Fatalf("select: %d %s", r.Code, r.Body)
	}
	disk, err := config.LoadConfig(path)
	if err != nil || disk.Routing.CodexOAuthAuthID != "b" {
		t.Fatalf("reload: %v %v", disk, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r = policyPUT(h, `{"auth_id":"b"}`)
	if r.Code != 200 {
		t.Fatal(r.Body)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("repeated selection rewrote config")
	}
	for _, id := range []string{"a", "b", "key", "other"} {
		a, _ := h.authManager.GetByID(id)
		if a.Disabled {
			t.Fatalf("selection disabled %s", id)
		}
	}
	r = policyPUT(h, `{"auth_id":null}`)
	if r.Code != 200 || h.authManager.CodexOAuthAuthID() != "" {
		t.Fatalf("clear: %d %s", r.Code, r.Body)
	}
}

func TestCodexAccountPolicyInvalidTargetsDoNotMutate(t *testing.T) {
	h, path := accountPolicyHandler(t)
	before, _ := os.ReadFile(path)
	for _, body := range []string{`{}`, `{"auth_id":42}`, `{"auth_id":"missing"}`, `{"auth_id":"disabled"}`, `{"auth_id":"key"}`, `{"auth_id":"runtime"}`, `{"auth_id":"virtual"}`, `{"auth_id":"other"}`} {
		r := policyPUT(h, body)
		if r.Code < 400 {
			t.Fatalf("invalid accepted %s: %s", body, r.Body)
		}
		after, _ := os.ReadFile(path)
		if string(before) != string(after) || h.cfg.Routing.CodexOAuthAuthID != "" || h.authManager.CodexOAuthAuthID() != "" {
			t.Fatalf("invalid changed policy: %s", body)
		}
	}
}

func TestCodexAccountPolicySaveFailureAndRuntimeMismatch(t *testing.T) {
	h, path := accountPolicyHandler(t)
	h.configFilePath = filepath.Join(path, "missing.yaml")
	r := policyPUT(h, `{"auth_id":"b"}`)
	if r.Code != 500 || h.cfg.Routing.CodexOAuthAuthID != "" || h.authManager.CodexOAuthAuthID() != "" {
		t.Fatalf("save failure changed state: %d %s", r.Code, r.Body)
	}
	h.configFilePath = path
	h.SetConfigReloadHook(func(context.Context, *config.Config) {})
	r = policyPUT(h, `{"auth_id":"b"}`)
	if r.Code != 409 {
		t.Fatalf("silent runtime mismatch: %d %s", r.Code, r.Body)
	}
}
