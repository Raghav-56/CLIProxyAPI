package openai

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type interruptAtTerminalWrite struct {
	local    *responsesLocalInterrupt
	observed bool
	accepted bool
}

func (probe *interruptAtTerminalWrite) Append(_ string, payload []byte, _ time.Time) {
	if gjson.GetBytes(payload, "type").String() == "response.completed" {
		probe.observed = true
		probe.accepted = probe.local.deliver([]byte(`{"type":"response.interrupt","response_id":"unknown"}`))
	}
}

func TestResponsesTerminalWriteEndsLocalInterruptWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/responses", nil)
	local := newResponsesLocalInterrupt()
	probe := &interruptAtTerminalWrite{local: local}
	data := make(chan []byte, 1)
	data <- []byte(`{"type":"response.completed","response":{"id":"r1","status":"completed","output":[]}}`)
	close(data)
	handler := NewOpenAIResponsesAPIHandler(nil)
	// The timeline callback observes the boundary immediately before the socket write.
	_, _, _, _, _ = handler.forwardResponsesWebsocket(c, nil, func(...any) {}, data, nil, probe, "test", responsesWebsocketForwardOptions{localInterrupt: local})
	if !probe.observed {
		t.Fatal("terminal event was not written")
	}
	if probe.accepted {
		t.Fatal("terminal response still accepted a local interrupt")
	}
}
