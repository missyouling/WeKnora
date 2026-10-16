package router

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// corsOrigins returns the cross-origin allowlist.
//
// Source of truth is WEKNORA_CORS_ORIGINS (comma-separated). "*" restores the
// legacy allow-all behaviour, which is only meant for local experiments and
// emergency rollback. When the variable is unset the defaults below apply, so
// a fresh deployment works out of the box for local dev and production.
//
// Entries may contain a single "*" (gin-contrib/cors requires AllowWildcard),
// e.g. "https://*.mozuiapp.com" matches any subdomain over https.
func corsOrigins() []string {
	raw, ok := os.LookupEnv("WEKNORA_CORS_ORIGINS")
	if !ok {
		return []string{
			// 本地开发：vite dev / vite preview / 直接访问后端
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:4173",
			"http://127.0.0.1:4173",
			"http://localhost:8080",
			"http://127.0.0.1:8080",
			// 生产域名（显式 + 通配符子域，两者等价保留以便阅读）
			"https://daily.mozuiapp.com",
			"https://*.mozuiapp.com",
			// Cloudflare 隧道（vite.config.ts 的 allowedHosts 同源）
			"https://*.trycloudflare.com",
		}
	}
	origins := make([]string, 0)
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// corsSelfAuthenticatedSurfaceExempt lets surfaces that carry their own
// credential through the global CORS layer. These are the browser-facing
// routes whose auth does not rely on ambient credentials, so the global
// allowlist cannot know their callers' origins in advance:
//
//   - /api/v1/embed/** and /api/v1/embed-frame-policy — per-channel allowlist
//     enforced by middleware.EmbedAuth
//   - /mcp/:endpoint_id — per-endpoint bearer token (browser MCP clients such
//     as Cursor / Claude connectors)
//   - /r/:token — short-lived capability URL for clients that cannot attach a
//     WeKnora bearer token
//
// Rejecting these here would pre-empt their own auth with a 403 and break live
// third-party integrations, so the global layer defers to them. Everything
// else stays on the WEKNORA_CORS_ORIGINS allowlist.
func corsSelfAuthenticatedSurfaceExempt(c *gin.Context, _ string) bool {
	p := c.Request.URL.Path
	switch {
	case p == "/api/v1/embed-frame-policy":
		return true
	case strings.HasPrefix(p, "/api/v1/embed/"):
		return true
	case strings.HasPrefix(p, "/mcp/"):
		return true
	case strings.HasPrefix(p, "/r/"):
		return true
	}
	return false
}
