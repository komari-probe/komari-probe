package router

import (
	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/features/auth"
	"github.com/sonar-probe/sonar/internal/features/backup"
	"github.com/sonar-probe/sonar/internal/features/node"
	"github.com/sonar-probe/sonar/internal/features/plugin"
	"github.com/sonar-probe/sonar/internal/features/theme"
	"github.com/sonar-probe/sonar/internal/platform/frontend"
	"github.com/sonar-probe/sonar/internal/transport/admin"
	jsonRpc "github.com/sonar-probe/sonar/internal/transport/jsonrpc"
)

// Register binds all HTTP, WebSocket, JSON-RPC and static frontend routes.
//
// 设计：JSON 类接口统一经声明式路由桥 jsonRpc.Bind 绑定到对应 RPC2 方法，
// 不再有 per-resource gin handler 层。仅二进制/流/重定向/特殊鉴权类接口保留为 REST handler。
func Register(r *gin.Engine) {
	r.Any("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	registerPublicRoutes(r)
	registerAgentRoutes(r)
	registerAdminRoutes(r)

	frontend.Static(r.Group("/"), func(handlers ...gin.HandlerFunc) {
		r.NoRoute(handlers...)
	})
}

// registerPublicRoutes 公开路由。JSON 读接口经 Bind 绑定到 public: 命名空间方法。
func registerPublicRoutes(r *gin.Engine) {
	// 非 JSON / 特殊流程，保留 REST handler。
	r.POST("/api/login", auth.Login)
	r.GET("/api/logout", auth.Logout)
	r.GET("/api/oauth", auth.OAuth)
	r.GET("/api/oauth_callback", auth.OAuthCallback)
	// 插件公开页面（visibility=public 的 iframe 页面），无需鉴权。
	r.GET("/api/plugin/:short/*filepath", plugin.ServePublicPluginFile)
	// /api/clients 是 WebSocket 端点（客户端发 "get"/"get <uuid>" 拉取在线列表与最新上报），
	// 非 JSON-RPC，保留为 WS handler。
	r.GET("/api/clients", node.GetClients)

	// JSON 接口 -> RPC2。
	r.GET("/api/me", jsonRpc.Bind("public:getMe", jsonRpc.WithRaw()))
	r.GET("/api/nodes", jsonRpc.Bind("public:getNodesInformation"))
	r.GET("/api/public", jsonRpc.Bind("public:getPublicSettings"))
	r.GET("/api/version", jsonRpc.Bind("public:getVersion"))
	r.GET("/api/recent/:uuid", jsonRpc.Bind("public:getClientRecentRecords", jsonRpc.WithPath("uuid")))
	r.GET("/api/records/load", jsonRpc.Bind("public:getRecordsByUUID", jsonRpc.WithQuery("uuid", "load_type", "hours")))
	r.GET("/api/records/ping", jsonRpc.Bind("public:getPingRecords", jsonRpc.WithQuery("uuid", "task_id", "hours")))
	r.GET("/api/task/ping", jsonRpc.Bind("public:getPublicPingTasks"))
	r.GET("/api/task/ping/builtin-presets", jsonRpc.Bind("public:getBuiltinPingPresets"))

	// JSON-RPC 直连入口。
	r.GET("/api/rpc2", jsonRpc.OnRPCRequest)
	r.POST("/api/rpc2", jsonRpc.OnRPCRequest)
}

// registerAgentRoutes agent（客户端）上报与拉取路由。
func registerAgentRoutes(r *gin.Engine) {
	// AutoDiscovery 注册使用独立的 Authorization key 鉴权，保留 REST handler。
	r.POST("/api/clients/register", node.RegisterClient)

	tokenAuthorized := r.Group("/api/clients", auth.RequireRole(auth.RoleAdmin, auth.RoleClient))
	{
		// Agent 上报统一使用 v2 JSON-RPC。
		tokenAuthorized.GET("/v2/rpc", node.WebSocketV2RPC)
		tokenAuthorized.POST("/v2/rpc", node.UploadV2RPC)
	}
}

// registerAdminRoutes 管理员路由。除二进制/流类外全部经 Bind 绑定到 admin: 命名空间方法。
func registerAdminRoutes(r *gin.Engine) {
	g := r.Group("/api/admin", auth.RequireRole(auth.RoleAdmin))
	admin.RegisterPprofRoutes(g)

	// --- 二进制/流/重定向类，保留 REST handler ---
	backup.RegisterRoutes(g)
	uploadHandler := NewArchiveUploadHandler()
	uploadGroup := g.Group("/upload")
	{
		uploadGroup.POST("/init", uploadHandler.Init)
		uploadGroup.POST("/chunk", uploadHandler.Chunk)
		uploadGroup.POST("/merge", uploadHandler.Merge)
		uploadGroup.POST("/cancel", uploadHandler.Cancel)
	}
	g.GET("/test/geoip", jsonRpc.Bind("admin:testGeoip", jsonRpc.WithQuery("ip")))
	g.POST("/test/sendMessage", jsonRpc.Bind("admin:testSendMessage"))
	g.POST("/update/mmdb", admin.UpdateMmdbGeoIP)
	g.POST("/update/user", auth.UpdateUser)
	g.PUT("/update/favicon", admin.UploadFavicon)
	g.POST("/update/favicon", admin.DeleteFavicon)

	// theme 的安装流程通过统一的分片上传接口；其余主题接口保留 REST handler。
	theme.RegisterRoutes(g)

	// 2FA 含二维码 PNG / 敏感操作，保留 REST handler。
	twoFactor := g.Group("/2fa")
	{
		twoFactor.GET("/generate", auth.Generate2FA)
		twoFactor.POST("/enable", auth.Enable2FA)
		twoFactor.POST("/disable", auth.RequireSensitive2FA(), auth.Disable2FA)
	}

	// oauth2 绑定走重定向，保留 REST handler。
	oauth2 := g.Group("/oauth2")
	{
		oauth2.GET("/bind", auth.BindingExternalAccount)
		oauth2.POST("/unbind", auth.UnbindExternalAccount)
	}

	// --- 以下全部 JSON -> RPC2 ---

	// settings
	settings := g.Group("/settings")
	{
		settings.GET("/", jsonRpc.Bind("admin:getSettings"))
		settings.POST("/", jsonRpc.Bind("admin:editSettings"))
		settings.POST("/oidc", jsonRpc.Bind("admin:setOidcProvider"))
		settings.GET("/oidc", jsonRpc.Bind("admin:getOidcProvider", jsonRpc.WithQuery("provider")))
		settings.POST("/message-sender", jsonRpc.Bind("admin:setMessageSenderProvider"))
		settings.GET("/message-sender", jsonRpc.Bind("admin:getMessageSenderProvider", jsonRpc.WithQuery("provider")))
	}

	// database storage inspection and maintenance
	databaseGroup := g.Group("/database")
	{
		databaseGroup.GET("/size", jsonRpc.Bind("admin:getDatabaseSize"))
		databaseGroup.POST("/vacuum", jsonRpc.Bind("admin:vacuumDatabase"))
	}

	// clients
	clientGroup := g.Group("/client")
	{
		clientGroup.POST("/add", jsonRpc.Bind("admin:addClient", jsonRpc.WithFlat()))
		clientGroup.GET("/list", jsonRpc.Bind("admin:listClients", jsonRpc.WithRaw()))
		clientGroup.GET("/:uuid", jsonRpc.Bind("admin:getClient", jsonRpc.WithPath("uuid"), jsonRpc.WithRaw()))
		clientGroup.POST("/:uuid/edit", jsonRpc.Bind("admin:editClient", jsonRpc.WithPath("uuid")))
		clientGroup.POST("/:uuid/remove", jsonRpc.Bind("admin:removeClient", jsonRpc.WithPath("uuid")))
		clientGroup.GET("/:uuid/token", jsonRpc.Bind("admin:getClientToken", jsonRpc.WithPath("uuid"), jsonRpc.WithFlat()))
		clientGroup.POST("/order", jsonRpc.Bind("admin:orderClients"))
	}

	// records
	record := g.Group("/record")
	{
		record.POST("/clear", jsonRpc.Bind("admin:clearRecords"))
		record.POST("/clear/all", jsonRpc.Bind("admin:clearAllRecords"))
	}

	// sessions
	session := g.Group("/session")
	{
		session.GET("/get", jsonRpc.Bind("admin:getSessions", jsonRpc.WithFlat()))
		session.POST("/remove", jsonRpc.Bind("admin:deleteSession"))
		session.POST("/remove/all", jsonRpc.Bind("admin:deleteAllSessions"))
	}

	g.GET("/logs", jsonRpc.Bind("admin:getLogs", jsonRpc.WithQuery("limit", "page")))

	// plugins: 安装流程通过统一的分片上传接口，启停/列表/日志走 RPC2，市场对齐主题市场。
	pluginGroup := g.Group("/plugin")
	{
		pluginGroup.GET("/list", jsonRpc.Bind("admin:listPlugins"))
		pluginGroup.POST("/enabled", jsonRpc.Bind("admin:setPluginEnabled"))
		pluginGroup.GET("/logs", jsonRpc.Bind("admin:getPluginLogs", jsonRpc.WithQuery("short")))
		pluginGroup.GET("/market/sources", plugin.ListPluginMarketSources)
		pluginGroup.POST("/market/sources", plugin.CreatePluginMarketSource)
		pluginGroup.PUT("/market/sources/:id", plugin.UpdatePluginMarketSource)
		pluginGroup.DELETE("/market/sources/:id", plugin.DeletePluginMarketSource)
		pluginGroup.GET("/market/catalog", plugin.ListPluginMarketCatalog)
		pluginGroup.POST("/market/install", plugin.InstallPluginFromMarket)
		pluginGroup.POST("/delete", jsonRpc.Bind("admin:deletePlugin"))
		pluginGroup.GET("/configuration", jsonRpc.Bind("admin:getPluginConfiguration", jsonRpc.WithQuery("short")))
		pluginGroup.POST("/configuration", jsonRpc.Bind("admin:setPluginConfiguration"))
		// 插件注入的管理页面静态文件
		pluginGroup.GET("/:short/*filepath", plugin.ServeAdminPluginFile)
	}

	// notifications
	notificationGroup := g.Group("/notification")
	{
		notificationGroup.GET("/offline", jsonRpc.Bind("admin:listOfflineNotifications"))
		notificationGroup.POST("/offline/edit", jsonRpc.Bind("admin:editOfflineNotification"))
		notificationGroup.POST("/offline/enable", jsonRpc.Bind("admin:enableOfflineNotification"))
		notificationGroup.POST("/offline/disable", jsonRpc.Bind("admin:disableOfflineNotification"))
		loadAlert := notificationGroup.Group("/load")
		{
			loadAlert.GET("/", jsonRpc.Bind("admin:getAllLoadNotifications"))
			loadAlert.POST("/add", jsonRpc.Bind("admin:addLoadNotification"))
			loadAlert.POST("/delete", jsonRpc.Bind("admin:deleteLoadNotification"))
			loadAlert.POST("/edit", jsonRpc.Bind("admin:editLoadNotification"))
		}
	}

	// ping tasks
	pingTask := g.Group("/ping")
	{
		pingTask.GET("/", jsonRpc.Bind("admin:getAllPingTasks"))
		pingTask.POST("/add", jsonRpc.Bind("admin:addPingTask"))
		pingTask.POST("/delete", jsonRpc.Bind("admin:deletePingTask"))
		pingTask.POST("/edit", jsonRpc.Bind("admin:editPingTask"))
		pingTask.POST("/order", jsonRpc.Bind("admin:orderPingTask"))
		pingTask.POST("/sync-client-nodes", jsonRpc.Bind("admin:syncClientPingNodes"))
	}
}
