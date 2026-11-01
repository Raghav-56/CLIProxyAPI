package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/api/handlers/management"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/usage"
)

type historyDeliveryProbe chan usage.Record

func (p historyDeliveryProbe) HandleUsage(_ context.Context, record usage.Record) { p <- record }

func TestRecentRequestsObservesUsageWithoutConsumingOtherSinks(t *testing.T) {
	history := newRecentRequestHistory()
	probe := make(historyDeliveryProbe, 1)
	manager := usage.NewManager(8)
	manager.Register(history)
	manager.Register(probe)
	defer manager.Stop()
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager.Publish(ctx, usage.Record{
		RequestID: "actual-attempt", AuthID: "chosen-oauth", AccountEmail: "selected@example.test",
		Provider: "codex", Model: "served-model", Alias: "requested-model", ResponseModel: "returned-model",
		ExecutorType: "CodexWebsocketsExecutor", RequestedAt: time.Unix(123, 0), Latency: 2500 * time.Millisecond,
		TTFT: 125 * time.Millisecond, Detail: usage.Detail{InputTokens: 100, CachedTokens: 40, OutputTokens: 12, TotalTokens: 112},
		APIKey: "never-expose-client-secret", AccessTokenSHA256: "never-expose-fingerprint", Source: "never-expose-source",
	})
	select {
	case record := <-probe:
		if record.RequestID != "actual-attempt" {
			t.Fatal("existing sink missed record")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("usage dispatch did not complete")
	}
	entries := history.snapshot()
	if len(entries) != 1 {
		t.Fatalf("entries=%d", len(entries))
	}
	entry := entries[0]
	if entry.AuthID != "chosen-oauth" || entry.Account != "selected@example.test" || entry.DownstreamTransport != "websocket" || entry.UpstreamTransport != "websocket" || !entry.Success {
		t.Fatalf("identity/transport mismatch: %+v", entry)
	}
	if entry.InputTokens == nil || *entry.InputTokens != 100 || entry.CachedInputTokens == nil || *entry.CachedInputTokens != 40 || entry.LatencyMS != 2500 || entry.TTFTMS == nil || *entry.TTFTMS != 125 {
		t.Fatalf("final usage mismatch: %+v", entry)
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "never-expose") || entry.StatusCode != nil {
		t.Fatalf("secret or invented success status: %s", encoded)
	}
}

func TestRecentRequestsBoundedNewestFirstAndUnknownUsage(t *testing.T) {
	history := newRecentRequestHistory()
	for i := 0; i < recentRequestCapacity+5; i++ {
		history.HandleUsage(context.Background(), usage.Record{RequestID: fmt.Sprint(i), Stream: true, ExecutorType: "CodexExecutor", Failed: true, Fail: usage.Failure{StatusCode: 429, Body: "private upstream detail"}})
	}
	entries := history.snapshot()
	if len(entries) != 200 || entries[0].ID != "204" || entries[199].ID != "5" {
		t.Fatalf("ring order/count incorrect: %+v", entries)
	}
	entry := entries[0]
	if entry.DownstreamTransport != "sse" || entry.UpstreamTransport != "http" || entry.StatusCode == nil || *entry.StatusCode != 429 || entry.InputTokens != nil || entry.TotalTokens != nil || entry.TTFTMS != nil || entry.Success || entry.UsageKnown {
		t.Fatalf("unknown/failure mismatch: %+v", entry)
	}
	entries[0].ID = "client changed snapshot"
	if history.snapshot()[0].ID != "204" {
		t.Fatal("reader mutated history")
	}
	if len(newRecentRequestHistory().snapshot()) != 0 {
		t.Fatal("new process history is not empty")
	}
}

func TestRecentRequestsV8RequiresManagementAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.RemoteManagement.SecretKey = "synthetic-enabled-management"
	h := management.NewHandlerWithoutConfigFilePath(cfg, nil)
	h.SetLocalPassword("synthetic-management-password")
	s := &Server{cfg: cfg, engine: gin.New(), mgmt: h, recentRequests: newRecentRequestHistory()}
	s.managementRoutesEnabled.Store(true)
	s.registerManagementRoutes()
	for _, authorized := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "/v8/management/observability/requests/recent", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		if authorized {
			req.Header.Set("Authorization", "Bearer synthetic-management-password")
		}
		rec := httptest.NewRecorder()
		s.engine.ServeHTTP(rec, req)
		if !authorized && rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous status=%d", rec.Code)
		}
		if authorized && (rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), `"retention":"process"`) || !strings.Contains(rec.Body.String(), `"requests":[]`)) {
			t.Fatalf("authorized response=%d %s", rec.Code, rec.Body.String())
		}
	}
}

func TestRecentRequestsLimitsRetainedMetadata(t *testing.T) {
	history := newRecentRequestHistory()
	history.HandleUsage(context.Background(), usage.Record{RequestID: "bounded", Alias: strings.Repeat("文", 1000), Model: strings.Repeat("m", 1000)})
	entry := history.snapshot()[0]
	if len(entry.RequestedModel) > 512 || !utf8.ValidString(entry.RequestedModel) || len(entry.Model) != 512 {
		t.Fatal("retained metadata is unbounded or invalid UTF-8")
	}
}
