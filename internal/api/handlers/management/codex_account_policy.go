package management

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func codexAccountRoutingState(id string) gin.H {
	if id == "" {
		return gin.H{"mode": "automatic", "auth_id": nil}
	}
	return gin.H{"mode": "manual", "auth_id": id}
}

func (h *Handler) GetCodexAccountRouting(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil || h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing is unavailable"})
		return
	}
	state := codexAccountRoutingState(h.cfg.Routing.CodexOAuthAuthID)
	state["applied"] = h.authManager.CodexOAuthAuthID() == h.cfg.Routing.CodexOAuthAuthID
	c.JSON(http.StatusOK, state)
}

func (h *Handler) PutCodexAccountRouting(c *gin.Context) {
	var body map[string]json.RawMessage
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	raw, exists := body["auth_id"]
	var id string
	if !exists || (string(raw) != "null" && json.Unmarshal(raw, &id) != nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth_id must be a string or null"})
		return
	}
	id = strings.TrimSpace(id)
	h.mu.Lock()
	if h.cfg == nil || h.authManager == nil {
		h.mu.Unlock()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing is unavailable"})
		return
	}
	manager := h.authManager
	if id != "" {
		target, ok := manager.GetByID(id)
		if !ok {
			h.mu.Unlock()
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		if !strings.EqualFold(strings.TrimSpace(target.Provider), "codex") || target.AuthKind() != coreauth.AuthKindOAuth || coreauth.IsPluginVirtualAuth(target) || coreauth.IsConfigAPIKeyAuth(target) || target.AuthSourceKind() == coreauth.AuthSourceConfig || isRuntimeOnlyAuth(target) || strings.TrimSpace(target.FileName) == "" {
			h.mu.Unlock()
			c.JSON(http.StatusBadRequest, gin.H{"error": "target must be a persistent Codex OAuth account"})
			return
		}
		if target.Disabled {
			h.mu.Unlock()
			c.JSON(http.StatusConflict, gin.H{"error": "account is disabled; enable it before selecting"})
			return
		}
		if manager.HomeEnabled() {
			h.mu.Unlock()
			c.JSON(http.StatusConflict, gin.H{"error": "manual account routing is unavailable while Home is enabled"})
			return
		}
	}
	previous := h.cfg.Routing.CodexOAuthAuthID
	if previous == id {
		h.mu.Unlock()
		if manager.CodexOAuthAuthID() != id {
			c.JSON(http.StatusConflict, gin.H{"error": "saved routing has not been applied", "saved": true})
			return
		}
		c.JSON(http.StatusOK, codexAccountRoutingState(id))
		return
	}
	h.cfg.Routing.CodexOAuthAuthID = id
	snapshot, ok := h.saveConfigAndSnapshotLocked(c)
	if !ok {
		h.cfg.Routing.CodexOAuthAuthID = previous
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()
	h.reloadConfigAfterManagementSave(context.WithoutCancel(c.Request.Context()), snapshot)
	saved, err := config.LoadConfig(h.configFilePath)
	if err != nil || saved.Routing.CodexOAuthAuthID != id || manager.CodexOAuthAuthID() != id {
		c.JSON(http.StatusConflict, gin.H{"error": "routing saved but disk or runtime verification failed", "saved": true})
		return
	}
	c.JSON(http.StatusOK, codexAccountRoutingState(id))
}
