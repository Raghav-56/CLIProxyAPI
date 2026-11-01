package api

import (
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/api/handlers/management"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodexAccountPolicyV8AccessControl(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, authorized := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.RemoteManagement.SecretKey = "synthetic-enabled-management"
		m := coreauth.NewManager(nil, nil, nil)
		m.SetConfig(cfg)
		h := management.NewHandlerWithoutConfigFilePath(cfg, m)
		h.SetLocalPassword("synthetic-management-password")
		s := &Server{cfg: cfg, engine: gin.New(), mgmt: h}
		s.managementRoutesEnabled.Store(true)
		s.registerManagementRoutes()
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			req := httptest.NewRequest(method, "/v8/management/routing/codex-account", strings.NewReader(`{"auth_id":null}`))
			req.RemoteAddr = "127.0.0.1:1234"
			req.Header.Set("Content-Type", "application/json")
			if authorized {
				req.Header.Set("Authorization", "Bearer synthetic-management-password")
			}
			rec := httptest.NewRecorder()
			s.engine.ServeHTTP(rec, req)
			want := 401
			if authorized {
				want = 200
			}
			if rec.Code != want {
				t.Fatalf("%s authorized=%v: %d %s", method, authorized, rec.Code, rec.Body)
			}
		}
	}
}
