package auth

import (
	"context"
	"net/http"
	"strings"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

type codexAccountPolicy struct{ authID string }
type codexAccountPolicyContextKey struct{}

func (p codexAccountPolicy) allows(auth *Auth) bool {
	return p.authID == "" || !strings.EqualFold(strings.TrimSpace(auth.Provider), "codex") || auth.AuthKind() != AuthKindOAuth || auth.ID == p.authID
}

func accountPolicyFromContext(ctx context.Context) codexAccountPolicy {
	if ctx == nil {
		return codexAccountPolicy{}
	}
	policy, _ := ctx.Value(codexAccountPolicyContextKey{}).(codexAccountPolicy)
	return policy
}

// CodexOAuthAuthID reports the currently applied subscription routing policy.
func (m *Manager) CodexOAuthAuthID() string {
	if m == nil {
		return ""
	}
	cfg, _ := m.runtimeConfig.Load().(*internalconfig.Config)
	if cfg == nil {
		return ""
	}
	return cfg.Routing.CodexOAuthAuthID
}

func (m *Manager) withAccountPolicySnapshot(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(codexAccountPolicyContextKey{}).(codexAccountPolicy); ok {
		return ctx
	}
	return context.WithValue(ctx, codexAccountPolicyContextKey{}, codexAccountPolicy{authID: m.CodexOAuthAuthID()})
}

func (m *Manager) accountPolicyHomeError(ctx context.Context) error {
	if m.HomeEnabled() && accountPolicyFromContext(ctx).authID != "" {
		return &Error{Code: "codex_account_policy_home", Message: "manual Codex OAuth account routing is unavailable while Home is enabled", HTTPStatus: http.StatusServiceUnavailable}
	}
	return nil
}
