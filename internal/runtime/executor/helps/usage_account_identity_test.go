package helps

import (
	"context"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/usage"
)

func TestUsageReporterCapturesAccountIdentityBeforeAuthChanges(t *testing.T) {
	auth := &cliproxyauth.Auth{ID: "selected-auth", Metadata: map[string]any{"email": "chosen@example.test", "access_token": "private-token"}}
	reporter := NewUsageReporter(context.Background(), "codex", "model", auth)
	auth.Metadata["email"] = "later@example.test"
	record := reporter.buildRecord(usage.Detail{InputTokens: 5}, false)
	if record.AuthID != "selected-auth" || record.AccountEmail != "chosen@example.test" {
		t.Fatalf("request identity not captured: %+v", record)
	}
}
