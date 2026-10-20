package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/usage"
)

const recentRequestCapacity = 200
const recentRequestTextLimit = 512

// RecentRequest is an allow-listed execution summary. Request payloads, keys,
// upstream error bodies, and credential material never enter this store.
type RecentRequest struct {
	ID                  string    `json:"id"`
	TraceID             string    `json:"trace_id"`
	RequestedAt         time.Time `json:"requested_at"`
	Provider            string    `json:"provider"`
	AuthID              string    `json:"auth_id"`
	Account             string    `json:"account"`
	Model               string    `json:"model"`
	RequestedModel      string    `json:"requested_model"`
	ResponseModel       string    `json:"response_model"`
	DownstreamTransport string    `json:"downstream_transport"`
	UpstreamTransport   string    `json:"upstream_transport"`
	LatencyMS           float64   `json:"latency_ms"`
	TTFTMS              *float64  `json:"ttft_ms"`
	Success             bool      `json:"success"`
	StatusCode          *int      `json:"status_code"`
	InputTokens         *int64    `json:"input_tokens"`
	OutputTokens        *int64    `json:"output_tokens"`
	CachedInputTokens   *int64    `json:"cached_input_tokens"`
	TotalTokens         *int64    `json:"total_tokens"`
	UsageKnown          bool      `json:"usage_known"`
}

type recentRequestHistory struct {
	mu      sync.RWMutex
	entries []RecentRequest
	next    int
	host    string
}

var processRecentRequests = newRecentRequestHistory()
var registerRecentRequestsOnce sync.Once

func newRecentRequestHistory() *recentRequestHistory {
	host, _ := os.Hostname()
	return &recentRequestHistory{host: host, entries: make([]RecentRequest, 0, recentRequestCapacity)}
}

func (h *recentRequestHistory) HandleUsage(ctx context.Context, record usage.Record) {
	if h == nil {
		return
	}
	downstream := "http"
	if cliproxyexecutor.DownstreamWebsocket(ctx) {
		downstream = "websocket"
	} else if record.Stream {
		downstream = "sse"
	}
	upstream := "unknown"
	switch record.ExecutorType {
	case "CodexWebsocketsExecutor":
		upstream = "websocket"
	case "CodexExecutor":
		upstream = "http"
	}
	cachedInput := record.Detail.CacheReadTokens
	if cachedInput == 0 && record.Provider == "codex" {
		cachedInput = record.Detail.CachedTokens
	}
	entry := RecentRequest{
		ID: diagnosticText(record.RequestID), TraceID: diagnosticText(record.TraceID), RequestedAt: record.RequestedAt,
		Provider: diagnosticText(record.Provider), AuthID: diagnosticText(record.AuthID), Account: diagnosticText(record.AccountEmail),
		Model: diagnosticText(record.Model), RequestedModel: diagnosticText(record.Alias), ResponseModel: diagnosticText(record.ResponseModel),
		DownstreamTransport: downstream, UpstreamTransport: upstream,
		LatencyMS:  float64(record.Latency) / float64(time.Millisecond),
		Success:    !record.Failed,
		UsageKnown: record.Detail.TotalTokens > 0 || record.Detail.InputTokens > 0 || record.Detail.OutputTokens > 0,
	}
	if record.Fail.StatusCode > 0 {
		value := record.Fail.StatusCode
		entry.StatusCode = &value
	}
	if record.TTFT > 0 {
		value := float64(record.TTFT) / float64(time.Millisecond)
		entry.TTFTMS = &value
	}
	if entry.UsageKnown {
		input := record.Detail.InputTokens
		output := record.Detail.OutputTokens
		total := record.Detail.TotalTokens
		entry.InputTokens = &input
		entry.OutputTokens = &output
		entry.CachedInputTokens = &cachedInput
		entry.TotalTokens = &total
	}
	h.mu.Lock()
	if len(h.entries) < recentRequestCapacity {
		h.entries = append(h.entries, entry)
	} else {
		h.entries[h.next] = entry
	}
	h.next = (h.next + 1) % recentRequestCapacity
	h.mu.Unlock()
}

func diagnosticText(value string) string {
	if len(value) > recentRequestTextLimit {
		value = value[:recentRequestTextLimit]
		for !utf8.ValidString(value) && len(value) > 0 {
			value = value[:len(value)-1]
		}
	}
	return strings.Clone(value)
}

func (h *recentRequestHistory) snapshot() []RecentRequest {
	if h == nil {
		return []RecentRequest{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	entries := make([]RecentRequest, len(h.entries))
	for i := range entries {
		index := (h.next - 1 - i + len(h.entries)) % len(h.entries)
		entries[i] = h.entries[index]
	}
	return entries
}

func (s *Server) getRecentRequests(c *gin.Context) {
	host := ""
	if s.recentRequests != nil {
		host = s.recentRequests.host
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"host": host, "capacity": recentRequestCapacity, "retention": "process", "requests": s.recentRequests.snapshot(),
	})
}
