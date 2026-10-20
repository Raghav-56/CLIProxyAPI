package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

func TestAccountConsoleShellAndAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{cfg: &config.Config{}, engine: gin.New()}
	s.engine.GET("/accounts.html", s.serveAccountConsole)
	s.engine.GET("/accounts-assets/:name", s.serveAccountConsoleAsset)
	for _, tc := range []struct {
		path    string
		want    int
		content string
	}{
		{"/accounts.html", http.StatusOK, "Management key"},
		{"/accounts-assets/accounts.css", http.StatusOK, ".account-grid"},
		{"/accounts-assets/accounts.js", http.StatusOK, "/routing/codex-account"},
		{"/accounts-assets/private.json", http.StatusNotFound, ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			s.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if recorder.Code != tc.want || !strings.Contains(recorder.Body.String(), tc.content) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if recorder.Code == http.StatusOK && recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("asset lacks nosniff policy")
			}
			if tc.path == "/accounts.html" {
				if strings.Contains(recorder.Body.String(), "__PROXY_HOST__") || recorder.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("host was not rendered or shell could be cached")
				}
				if !strings.Contains(recorder.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
					t.Fatal("shell lacks a frame policy")
				}
			}
		})
	}
}

func TestAccountConsoleHonorsManagementPanelGuards(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *config.Config
	}{
		{"missing config", nil},
		{"Home owns management", &config.Config{Home: config.HomeConfig{Enabled: true}}},
		{"control panel disabled", &config.Config{RemoteManagement: config.RemoteManagement{DisableControlPanel: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{cfg: tc.cfg, engine: gin.New()}
			s.engine.GET("/accounts.html", s.serveAccountConsole)
			s.engine.GET("/accounts-assets/:name", s.serveAccountConsoleAsset)
			for _, path := range []string{"/accounts.html", "/accounts-assets/accounts.js"} {
				recorder := httptest.NewRecorder()
				s.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
				if recorder.Code != http.StatusNotFound {
					t.Fatalf("%s status=%d want404", path, recorder.Code)
				}
			}
		})
	}
}
