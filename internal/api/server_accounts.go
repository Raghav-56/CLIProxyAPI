package api

import (
	"embed"
	"html"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed assets/accounts.html assets/accounts.css assets/accounts.js
var accountConsoleAssets embed.FS

func (s *Server) serveAccountConsole(c *gin.Context) {
	if s.cfg == nil || s.cfg.Home.Enabled || s.cfg.RemoteManagement.DisableControlPanel {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	page, errRead := accountConsoleAssets.ReadFile("assets/accounts.html")
	if errRead != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	hostname, _ := os.Hostname()
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(strings.ReplaceAll(string(page), "__PROXY_HOST__", html.EscapeString(hostname))))
}

func (s *Server) serveAccountConsoleAsset(c *gin.Context) {
	if s.cfg == nil || s.cfg.Home.Enabled || s.cfg.RemoteManagement.DisableControlPanel {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	name := c.Param("name")
	contentType := ""
	switch name {
	case "accounts.css":
		contentType = "text/css; charset=utf-8"
	case "accounts.js":
		contentType = "text/javascript; charset=utf-8"
	default:
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	content, errRead := accountConsoleAssets.ReadFile("assets/" + name)
	if errRead != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, content)
}
