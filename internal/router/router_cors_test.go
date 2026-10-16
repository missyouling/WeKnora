package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCorsOriginsDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origins := corsOrigins()
	require.NotEmpty(t, origins)

	// 本地开发必须默认放行，避免全新部署前端跨域报错。
	for _, want := range []string{
		"http://localhost:3000",
		"http://localhost:5173",
		"http://localhost:4173",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"https://daily.mozuiapp.com",
		"https://*.mozuiapp.com",
	} {
		require.Contains(t, origins, want, "default allowlist must contain %s", want)
	}

	// 默认清单不允许全放开。
	require.NotContains(t, origins, "*")
}

func TestCorsOriginsFromEnv(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("自定义清单", func(t *testing.T) {
		t.Setenv("WEKNORA_CORS_ORIGINS", "https://a.example.com, https://b.example.com ,,")
		require.Equal(t, []string{"https://a.example.com", "https://b.example.com"}, corsOrigins())
	})

	t.Run("星号恢复全放开", func(t *testing.T) {
		t.Setenv("WEKNORA_CORS_ORIGINS", "*")
		require.Equal(t, []string{"*"}, corsOrigins())
	})

	t.Run("空串视为空清单", func(t *testing.T) {
		t.Setenv("WEKNORA_CORS_ORIGINS", "  ")
		require.Empty(t, corsOrigins())
	})
}

func TestCorsSelfAuthenticatedSurfaceExempt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	allowed := []string{
		"/api/v1/embed/abc/exchange",
		"/api/v1/embed/abc/sessions",
		"/api/v1/embed/abc/files",
		"/api/v1/embed-frame-policy",
		"/mcp/ep-1",
		"/r/some-capability-token",
	}
	for _, p := range allowed {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, p, nil)
		require.True(t, corsSelfAuthenticatedSurfaceExempt(c, "https://third-party.example"),
			"self-authenticated path %s must be exempt so its own auth can police it", p)
	}

	// 非豁免路径一律不收窄，由白名单收敛。
	for _, p := range []string{
		"/api/v1/knowledge-bases",
		"/api/v1/auth/login",
		"/health",
		"/api/v1/embedframe-policy",
		"/api/v1/embed",
		"/mcpfoo/ep-1",
		"/rfoo/token",
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, p, nil)
		require.False(t, corsSelfAuthenticatedSurfaceExempt(c, "https://evil.example"),
			"path %s must not be exempt", p)
	}
}

// buildCorsEngine 复刻 NewRouter 中与 CORS 相关的装配，便于端到端验证状态码。
func buildCorsEngine(t *testing.T) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.Use(cors.New(cors.Config{
		AllowOrigins:               corsOrigins(),
		AllowWildcard:              true,
		AllowOriginWithContextFunc: corsSelfAuthenticatedSurfaceExempt,
		AllowMethods:               []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:               []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials:           true,
		MaxAge:                     time.Hour,
	}))
	r.GET("/api/v1/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	r.POST("/api/v1/embed/:channel_id/exchange", func(c *gin.Context) { c.String(http.StatusOK, "embed") })
	r.GET("/api/v1/embed-frame-policy", func(c *gin.Context) { c.String(http.StatusOK, "policy") })
	r.Any("/mcp/:endpoint_id", func(c *gin.Context) { c.String(http.StatusOK, "mcp") })
	r.GET("/r/:token", func(c *gin.Context) { c.String(http.StatusOK, "grant") })
	return r
}

func doCORS(r *gin.Engine, method, path, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestCORSAllowsWhitelistedOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := buildCorsEngine(t)

	for _, origin := range []string{
		"http://localhost:5173",
		"http://127.0.0.1:8080",
		"https://daily.mozuiapp.com",
		"https://tenant.mozuiapp.com",
		"https://anything.trycloudflare.com",
	} {
		rec := doCORS(r, http.MethodGet, "/api/v1/ping", origin)
		require.Equal(t, http.StatusOK, rec.Code, "origin %s should be allowed", origin)
		require.Equal(t, origin, rec.Header().Get("Access-Control-Allow-Origin"),
			"allowed origin %s should be echoed back", origin)
	}
}

func TestCORSRejectsUnknownOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := buildCorsEngine(t)

	// 不在白名单的 Origin 必须被拒绝（这是本次收敛的核心行为）。
	for _, origin := range []string{
		"https://evil.example",
		"http://localhost:9999",
		"https://mozuiapp.com.evil.com",
	} {
		rec := doCORS(r, http.MethodGet, "/api/v1/ping", origin)
		require.Equal(t, http.StatusForbidden, rec.Code, "origin %s must be rejected", origin)
	}
}

func TestCORSWildcardEnvRestoresAllowAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("WEKNORA_CORS_ORIGINS", "*")
	r := buildCorsEngine(t)

	rec := doCORS(r, http.MethodGet, "/api/v1/ping", "https://anything.example")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSSelfAuthenticatedSurfacesBypassGlobalAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := buildCorsEngine(t)

	// 第三方站点调用自鉴权接口：全局 CORS 层放行，交由各自中间件校验。
	// 若此处变成 403，说明豁免失效，会直接打断生产 embed / MCP / 能力 URL。
	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/v1/embed/chan123/exchange"},
		{http.MethodGet, "/api/v1/embed-frame-policy"},
		{http.MethodPost, "/mcp/ep-1"},
		{http.MethodGet, "/r/capability-token"},
	}
	for _, tc := range cases {
		rec := doCORS(r, tc.method, tc.path, "https://third-party.example")
		require.Equal(t, http.StatusOK, rec.Code,
			"%s %s must not be 403'd by global CORS", tc.method, tc.path)
		require.Equal(t, "https://third-party.example", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSNoOriginHeaderIsNotBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := buildCorsEngine(t)

	// 非浏览器客户端（CLI / 桌面端 / 服务端）不发 Origin，不应受 CORS 影响。
	rec := doCORS(r, http.MethodGet, "/api/v1/ping", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}
