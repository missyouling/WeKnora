package router

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/dig"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/handler/session"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/mcpserver"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types/interfaces"

	_ "github.com/Tencent/WeKnora/docs" // swagger docs
)

// 路由依赖原本是 68 个平铺字段的 God-struct，现按域拆分为若干子参数集。
// 每个子结构体都是独立的 dig 参数对象，RouterParams 嵌套它们：
//   - 各路由组只需关心自己那个依赖集（每组 ≤10 个字段）；
//   - 新增路由时改对应的子结构体，不必面对 68 个平铺字段。
// 访问方式相应变为 params.<组>.<字段>，例如 params.Knowledge.KnowledgeHandler。

// CoreRouteDeps 基础能力：配置、文件、存储、缓存、向量库
type CoreRouteDeps struct {
	dig.In

	Config                 *config.Config
	FileService            interfaces.FileService
	RedisClient            *redis.Client
	StorageBackendResolver interfaces.StorageBackendResolver
	ResourceCatalog        interfaces.ResourceCatalog
	StorageBackendHandler  *handler.StorageBackendHandler
	VectorStoreHandler     *handler.VectorStoreHandler
}

// AuthTenantRouteDeps 认证、用户与租户
type AuthTenantRouteDeps struct {
	dig.In

	UserService             interfaces.UserService
	TenantService           interfaces.TenantService
	TenantAPIKeyService     interfaces.TenantAPIKeyService
	TenantMemberService     interfaces.TenantMemberService
	TenantHandler           *handler.TenantHandler
	TenantMemberHandler     *handler.TenantMemberHandler
	TenantInvitationHandler *handler.TenantInvitationHandler
	AuthHandler             *handler.AuthHandler
	InitializationHandler   *handler.InitializationHandler
	SystemHandler           *handler.SystemHandler
}

// KnowledgeRouteDeps 知识库、知识、分块、标签、FAQ、Wiki
type KnowledgeRouteDeps struct {
	dig.In

	KBService        interfaces.KnowledgeBaseService
	KnowledgeService interfaces.KnowledgeService
	ChunkService     interfaces.ChunkService
	KBShareService   interfaces.KBShareService
	KBHandler        *handler.KnowledgeBaseHandler
	KnowledgeHandler *handler.KnowledgeHandler
	ChunkHandler     *handler.ChunkHandler
	FAQHandler       *handler.FAQHandler
	TagHandler       *handler.TagHandler
	WikiPageHandler  *handler.WikiPageHandler
}

// SessionRouteDeps 会话与消息
type SessionRouteDeps struct {
	dig.In

	SessionService           interfaces.SessionService
	MessageService           interfaces.MessageService
	SessionHandler           *session.Handler
	MessageHandler           *handler.MessageHandler
	MessageSuggestionHandler *handler.MessageSuggestionHandler
}

// ModelRouteDeps 模型、凭据与评估
type ModelRouteDeps struct {
	dig.In

	ModelService            interfaces.ModelService
	EvaluationService       interfaces.EvaluationService
	ModelHandler            *handler.ModelHandler
	ModelCredentialsHandler *handler.ModelCredentialsHandler
	EvaluationHandler       *handler.EvaluationHandler
}

// AgentRouteDeps 智能体、技能、组织、收藏、记忆
type AgentRouteDeps struct {
	dig.In

	AgentShareService   interfaces.AgentShareService
	CustomAgentHandler  *handler.CustomAgentHandler
	SkillHandler        *handler.SkillHandler
	OrganizationHandler *handler.OrganizationHandler
	UserFavoriteHandler *handler.UserResourceFavoriteHandler
	MemoryHandler       *handler.MemoryHandler
}

// SandboxRouteDeps 沙箱配置与宿主沙箱
type SandboxRouteDeps struct {
	dig.In

	SandboxConfigHandler *handler.SandboxConfigHandler
	SandboxSkillHandler  *handler.SandboxSkillHandler
	MeEnvVarHandler      *handler.MeEnvVarHandler
	HostSandbox          service.HostSandboxManager
}

// MCPRouteDeps MCP 服务、凭据、OAuth 与端点
type MCPRouteDeps struct {
	dig.In

	MCPServiceHandler     *handler.MCPServiceHandler
	MCPCredentialsHandler *handler.MCPCredentialsHandler
	MCPOAuthHandler       *handler.MCPOAuthHandler
	MCPEndpointHandler    *handler.MCPEndpointHandler
	MCPEndpointService    interfaces.MCPEndpointService
	MCPServer             *mcpserver.Server
}

// WebSearchRouteDeps 网络搜索
type WebSearchRouteDeps struct {
	dig.In

	WebSearchHandler            *handler.WebSearchHandler
	WebSearchProviderHandler    *handler.WebSearchProviderHandler
	WebSearchCredentialsHandler *handler.WebSearchProviderCredentialsHandler
}

// DataSourceRouteDeps 数据源接入
type DataSourceRouteDeps struct {
	dig.In

	DataSourceHandler            *handler.DataSourceHandler
	DataSourceCredentialsHandler *handler.DataSourceCredentialsHandler
}

// BusinessRouteDeps 二开业务：日常事务、车队、水电、账单、审计
type BusinessRouteDeps struct {
	dig.In

	BusinessExtractHandler *handler.BusinessExtractHandler
	UtilityHandler         *handler.UtilityHandler
	BillingHandler         *handler.BillingHandler
	FleetHandler           *handler.FleetHandler
	WeKnoraCloudHandler    *handler.WeKnoraCloudHandler
	AuditLogHandler        *handler.AuditLogHandler
	AuditLogService        interfaces.AuditLogService
}

// IntegrationRouteDeps 外部集成：IM 机器人与嵌入渠道
type IntegrationRouteDeps struct {
	dig.In

	IMHandler           *handler.IMHandler
	EmbedChannelHandler *handler.EmbedChannelHandler
	EmbedChannelService interfaces.EmbedChannelService
}

// RouterParams 路由参数（按域分组后的聚合入口）
type RouterParams struct {
	dig.In

	Core        CoreRouteDeps
	AuthTenant  AuthTenantRouteDeps
	Knowledge   KnowledgeRouteDeps
	Session     SessionRouteDeps
	Model       ModelRouteDeps
	Agent       AgentRouteDeps
	Sandbox     SandboxRouteDeps
	MCP         MCPRouteDeps
	WebSearch   WebSearchRouteDeps
	DataSource  DataSourceRouteDeps
	Business    BusinessRouteDeps
	Integration IntegrationRouteDeps
}

// NewRouter 创建新的路由
func NewRouter(params RouterParams) *gin.Engine {
	r := gin.New()
	r.ContextWithFallback = true
	// 清理 FormFile/MultipartForm 解析产生的 multipart 临时文件，避免容器 /tmp 持续增长。
	r.Use(middleware.MultipartFormCleanup())

	// Trusted proxies: gin defaults to trusting ALL proxies, which makes
	// c.ClientIP() honor a client-supplied X-Forwarded-For. Public, unauthed
	// embed endpoints rate-limit per (channel, ClientIP), so a spoofed XFF would
	// trivially bypass the limiter. Restrict to the fronting proxy network so
	// only the real client IP (appended by nginx) is returned. Configurable via
	// WEKNORA_TRUSTED_PROXIES (comma-separated CIDRs/IPs).
	if err := r.SetTrustedProxies(trustedProxies()); err != nil {
		logger.Errorf(context.Background(), "[Router] failed to set trusted proxies: %v", err)
	}

	// CORS 中间件应放在最前面。
	//
	// 通配符 "*" 的历史问题：① 任何站点都能调用 API；② 与 credentials 组合
	// 时浏览器直接拒绝，将来若引入 cookie 认证会静默失效。现收敛为受控清单，
	// 来源为 WEKNORA_CORS_ORIGINS（逗号分隔；设为 "*" 可临时恢复全放开，
	// 便于本地实验与紧急回滚）。
	//
	// 默认清单覆盖本地开发（vite 5173 / preview 4173 / 后端 8080）与生产域名
	// （*.mozuiapp.com），全新部署不会因跨域打断前端。
	//
	// embed 例外：/api/v1/embed/**、/mcp/**、/r/** 面向第三方浏览器客户端
	// （挂件 iframe / MCP 连接器 / 能力 URL），其凭据由各自的中间件校验
	// （EmbedAuth per-channel、MCPEndpointAuth per-endpoint、grant token）。
	// 若在此层先 403，会抢先于它们打断生产集成，故这些路径在本层放行、把校验
	// 交回对应中间件。见 corsSelfAuthenticatedSurfaceExempt。
	r.Use(cors.New(cors.Config{
		AllowOrigins:               corsOrigins(),
		AllowWildcard:              true,
		AllowOriginWithContextFunc: corsSelfAuthenticatedSurfaceExempt,
		AllowMethods:               []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key", "X-Request-ID", "X-Tenant-ID",
			"X-Embed-Session", "X-External-User-ID", "X-External-User-Token", "X-WeKnora-Desktop-Token",
			// Streamable HTTP MCP clients running in a browser send these on
			// the /mcp/:endpoint_id surface.
			"MCP-Protocol-Version", "Mcp-Method", "Mcp-Name", "Mcp-Session-Id", "Last-Event-ID",
		},
		ExposeHeaders:    []string{"Content-Length", "Access-Control-Allow-Origin", "Mcp-Session-Id"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 基础中间件（不需要认证）
	r.Use(middleware.RequestID())
	r.Use(middleware.Language())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.ErrorHandler())

	// 健康检查（不需要认证）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Swagger API 文档（仅在非生产环境下启用）
	// 通过 GIN_MODE 环境变量判断：release 模式下禁用 Swagger
	if gin.Mode() != gin.ReleaseMode {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler,
			ginSwagger.DefaultModelsExpandDepth(-1), // 默认折叠 Models
			ginSwagger.DocExpansion("list"),         // 展开模式: "list"(展开标签), "full"(全部展开), "none"(全部折叠)
			ginSwagger.DeepLinking(true),            // 启用深度链接
			ginSwagger.PersistAuthorization(true),   // 持久化认证信息
		))
	}

	// Embed page framing policy: emit a per-channel `frame-ancestors` CSP so the
	// embed SPA page (/embed/:channelId) can only be iframed by the channel's
	// allowed origins. This is the page-level counterpart to the API Origin
	// allowlist enforced in EmbedAuth. Registered before the static handler so
	// it runs for the embed HTML response.
	if params.Integration.EmbedChannelService != nil {
		r.Use(embedFrameAncestorsMiddleware(params.Integration.EmbedChannelService))
	}

	// 前端静态文件（内嵌前端；web 目录不存在时 serveFrontendStatic 内部自动跳过）
	serveFrontendStatic(r)

	// IM 回调路由（在认证中间件之前注册，使用各平台自身的签名验证）
	RegisterIMRoutes(r, params.Integration.IMHandler)

	// Web embed 公开路由（使用 publish token 鉴权，不走全局 Auth）
	RegisterEmbedPublicRoutes(
		r,
		params.Integration.EmbedChannelHandler,
		params.Integration.EmbedChannelService,
		params.AuthTenant.TenantService,
		params.Core.RedisClient,
		params.Core.FileService,
		params.Core.StorageBackendResolver,
		params.Core.ResourceCatalog,
	)

	// Workspace MCP server surface (/mcp/:endpoint_id): bearer-token auth per
	// endpoint, so it must precede the global Auth middleware.
	RegisterMCPServerRoutes(r, params.MCP.MCPServer, params.MCP.MCPEndpointService, params.AuthTenant.TenantService)

	// Short-lived capability URLs for IM and other clients that cannot attach
	// WeKnora authentication headers.
	serveResourceGrants(r, params.Core.ResourceCatalog, params.AuthTenant.TenantService, params.Core.FileService, params.Core.StorageBackendResolver)

	// Sandbox terminal WebSocket (self-authenticated via a short-lived
	// query ticket — see RegisterSandboxTerminalRoutes; browsers cannot set
	// auth headers on the WS handshake, so this must precede the global Auth
	// middleware). The ticket is minted by an authenticated POST.
	RegisterSandboxTerminalRoutes(r, params.Session.SessionHandler)
	RegisterSandboxDesktopRoutes(r, params.Session.SessionHandler)
	r.GET("/api/v1/local-browser/extension", params.Session.SessionHandler.BrowserSkillExtension)
	r.POST("/api/v1/local-browser/extension/authorize", params.Session.SessionHandler.BrowserSkillAuthorize)
	r.POST("/api/v1/local-browser/internal", params.Session.SessionHandler.BrowserSkillInternal)

	// 认证中间件
	r.Use(middleware.Auth(params.AuthTenant.TenantService, params.AuthTenant.UserService, params.AuthTenant.TenantMemberService, params.AuthTenant.TenantAPIKeyService, params.Core.Config))

	// 文件服务：统一代理本地/MinIO/COS/TOS存储后端（需要认证）
	serveFilesWithResources(r, params.Core.FileService, params.Core.StorageBackendResolver, params.Core.ResourceCatalog)

	// Presigned file access: no auth required, signature-verified.
	servePresignedFiles(r, params.AuthTenant.TenantService, params.Core.StorageBackendResolver)

	// Diagnostic preview of presigned URLs (Admin only, behind auth middleware).
	servePresignedPreview(r, params.Core.Config, params.Core.StorageBackendResolver, params.Core.ResourceCatalog)

	// Langfuse observability — only active when LANGFUSE_* env vars are set.
	// The middleware is registered unconditionally; when disabled it's a no-op.
	r.Use(langfuse.GinMiddleware())

	// Audit log injection — middleware/rbac.go's reject paths and the
	// admin-only /tenants/:id/audit-log endpoint pull the service out
	// of the gin context. Provider is a no-op when AuditLogService is
	// nil (e.g. lite mode without DB), so the rbac path degrades to
	// "log to stderr only" instead of crashing.
	r.Use(middleware.AuditServiceProvider(params.Business.AuditLogService))

	// 需要认证的API路由
	v1 := r.Group("/api/v1")
	{
		// rbacGuards bundles the role-gating middleware factories so each
		// Register* function below can attach the right guard without
		// taking a *config.Config dependency directly. The guards honour
		// cfg.Tenant.EnableRBAC: when false, they log but pass through,
		// preserving today's behaviour during the rollout window.
		rbacGuards := newRBACGuards(
			params.Core.Config,
			params.Knowledge.KBHandler,
			params.Agent.CustomAgentHandler,
			params.Knowledge.KnowledgeHandler,
			params.Knowledge.ChunkHandler,
			params.Knowledge.WikiPageHandler,
			params.Knowledge.KBService,
			params.Knowledge.KnowledgeService,
			params.Knowledge.ChunkService,
			params.Knowledge.KBShareService,
			params.Agent.AgentShareService,
		)

		// API-key gate: single authority for X-API-Key principals. Runs
		// first on every /api/v1 route (JWT sessions pass straight
		// through) and denies any route not explicitly declared via the
		// apiKeyGroup helpers. Must be attached BEFORE the Register* calls
		// so that sub-groups inherit it.
		v1.Use(rbacGuards.apiKeyAuthorizer.Middleware())

		RegisterAuthRoutes(v1, params.AuthTenant.AuthHandler, rbacGuards)
		RegisterTenantRoutes(v1, params.AuthTenant.TenantHandler, params.AuthTenant.TenantMemberHandler, params.AuthTenant.TenantInvitationHandler, params.Business.AuditLogHandler, rbacGuards)
		RegisterMyInvitationRoutes(v1, params.AuthTenant.TenantInvitationHandler)
		RegisterKnowledgeBaseRoutes(v1, params.Knowledge.KBHandler, rbacGuards)
		RegisterImageAttrRoutes(v1, params.Knowledge.KBHandler, rbacGuards)
		RegisterKnowledgeBaseActivityRoutes(v1, params.Business.AuditLogHandler, rbacGuards)
		// KB-scoped image proxy: lets tenants render images embedded in
		// org-shared / agent-visible KB content, which the tenant-scoped
		// /files route cannot serve because it enforces same-tenant paths.
		serveKBScopedFiles(
			v1,
			rbacGuards,
			params.AuthTenant.TenantService,
			params.Core.FileService,
			params.Core.StorageBackendResolver,
			params.Core.ResourceCatalog,
		)
		// Message-scoped image proxy: shared-agent replies belong to the
		// caller's session but may reference resources stored in the agent's
		// source workspace. Authorization is derived from the persisted message,
		// never from a client-provided workspace ID. Replies produced by the
		// caller's own agent over an org-shared KB fall back to the KB share
		// relation instead (#3022).
		serveMessageScopedFiles(
			v1,
			rbacGuards,
			params.Session.MessageService,
			params.Agent.AgentShareService,
			params.AuthTenant.TenantService,
			params.Core.FileService,
			params.Core.StorageBackendResolver,
			params.Core.ResourceCatalog,
			params.Knowledge.KBShareService,
			params.Knowledge.KBService,
			params.Knowledge.KnowledgeService,
		)
		RegisterKnowledgeTagRoutes(v1, params.Knowledge.TagHandler, rbacGuards)
		RegisterKnowledgeRoutes(v1, params.Knowledge.KnowledgeHandler, params.Business.BusinessExtractHandler, rbacGuards)
		RegisterUtilityRoutes(v1, params.Business.UtilityHandler, rbacGuards)
		RegisterBillingRoutes(v1, params.Business.BillingHandler, rbacGuards)
		RegisterFleetRoutes(v1, params.Business.FleetHandler, rbacGuards)
		RegisterFAQRoutes(v1, params.Knowledge.FAQHandler, rbacGuards)
		RegisterChunkRoutes(v1, params.Knowledge.ChunkHandler, rbacGuards)
		RegisterSessionRoutes(v1, params.Session.SessionHandler, params.Session.MessageSuggestionHandler, rbacGuards)
		RegisterChatRoutes(v1, params.Session.SessionHandler, rbacGuards)
		RegisterMessageRoutes(v1, params.Session.MessageHandler, rbacGuards)
		RegisterModelRoutes(v1, params.Model.ModelHandler, params.Model.ModelCredentialsHandler, rbacGuards)
		RegisterSandboxConfigRoutes(v1, params.Sandbox.SandboxConfigHandler, params.Sandbox.SandboxSkillHandler, rbacGuards)
		RegisterMyEnvVarRoutes(v1, params.Sandbox.MeEnvVarHandler)
		v1.GET("/me/browser", params.Session.SessionHandler.BrowserSkillAccount)
		v1.GET("/me/browser/extension", params.Session.SessionHandler.BrowserSkillDownload)
		v1.POST("/me/browser", params.Session.SessionHandler.BrowserSkillAccount)
		RegisterEvaluationRoutes(v1, params.Model.EvaluationHandler, rbacGuards)
		RegisterInitializationRoutes(v1, params.AuthTenant.InitializationHandler, rbacGuards)
		params.AuthTenant.SystemHandler.BindDeploymentCapabilities(deploymentCapabilitiesFromRouter(params))
		RegisterSystemRoutes(v1, params.AuthTenant.SystemHandler, rbacGuards)
		RegisterSystemAdminRoutes(v1, params.AuthTenant.SystemHandler, params.Business.AuditLogHandler, rbacGuards)
		RegisterMCPServiceRoutes(v1, params.MCP.MCPServiceHandler, params.MCP.MCPCredentialsHandler, params.MCP.MCPOAuthHandler, rbacGuards)
		RegisterWebSearchRoutes(v1, params.WebSearch.WebSearchHandler, rbacGuards)
		RegisterWebSearchProviderRoutes(v1, params.WebSearch.WebSearchProviderHandler, params.WebSearch.WebSearchCredentialsHandler, rbacGuards)
		RegisterVectorStoreRoutes(v1, params.Core.VectorStoreHandler, rbacGuards)
		RegisterStorageBackendRoutes(v1, params.Core.StorageBackendHandler, rbacGuards)
		RegisterCustomAgentRoutes(v1, params.Agent.CustomAgentHandler, rbacGuards)
		RegisterUserFavoriteRoutes(v1, params.Agent.UserFavoriteHandler, rbacGuards)
		RegisterSkillRoutes(v1, params.Agent.SkillHandler, rbacGuards)
		RegisterOrganizationRoutes(v1, params.Agent.OrganizationHandler, rbacGuards)
		RegisterIMChannelRoutes(v1, params.Integration.IMHandler, rbacGuards)
		RegisterEmbedChannelRoutes(v1, params.Integration.EmbedChannelHandler, rbacGuards)
		RegisterMCPEndpointRoutes(v1, params.MCP.MCPEndpointHandler, rbacGuards)
		RegisterDataSourceRoutes(v1, params.DataSource.DataSourceHandler, params.DataSource.DataSourceCredentialsHandler, rbacGuards)
		RegisterWeKnoraCloudRoutes(v1, params.Business.WeKnoraCloudHandler, rbacGuards)
		RegisterWikiPageRoutes(v1, params.Knowledge.WikiPageHandler, rbacGuards)
		RegisterMemoryRoutes(v1, params.Agent.MemoryHandler, rbacGuards)
		RegisterChunkerDebugRoutes(v1, rbacGuards)

		// Fail fast if any declared API-key policy points at a route
		// template that does not actually exist (typo / path drift). A
		// stale template would silently 403 every API key on that route,
		// so we panic at startup instead of shipping a dead policy.
		rbacGuards.assertAPIKeyPoliciesMatchRoutes(r)
	}

	return r
}

// trustedProxies returns the proxy CIDRs/IPs whose X-Forwarded-For headers
// gin should trust when resolving the client IP. Defaults to loopback and
// private ranges (covers the bundled nginx in a container network); override
// with WEKNORA_TRUSTED_PROXIES (comma-separated). An explicit empty value
// disables proxy trust entirely so ClientIP() returns the direct peer.
func trustedProxies() []string {
	raw, ok := os.LookupEnv("WEKNORA_TRUSTED_PROXIES")
	if !ok {
		return []string{
			"127.0.0.0/8",
			"::1/128",
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"fc00::/7",
		}
	}
	proxies := make([]string, 0)
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			proxies = append(proxies, p)
		}
	}
	return proxies
}

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
