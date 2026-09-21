package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/platform/api"
)

// handler_oauth_binding.go
// 外部账号绑定/解绑：绑定走 302 重定向，保留为 REST handler（不走 RPC 桥）。
// OIDC provider 配置的 get/set 是 RPC2 方法（见 transport/jsonrpc/admin_provider.go）。

func BindingExternalAccount(c *gin.Context) {
	session, _ := c.Cookie("session_token")
	user, err := GetUserBySession(session)
	if err != nil {
		api.RespondError(c, 500, "No user found: "+err.Error())
		return
	}
	c.SetCookie("binding_external_account", user.UUID, 3600, "/", "", false, true)
	c.Redirect(302, "/api/oauth")
}

func UnbindExternalAccount(c *gin.Context) {
	session, _ := c.Cookie("session_token")
	user, err := GetUserBySession(session)
	if err != nil {
		api.RespondError(c, 500, "No user found: "+err.Error())
		return
	}
	if err := unbindExternalAccount(user.UUID); err != nil {
		api.RespondError(c, 500, "Failed to unbind external account: "+err.Error())
		return
	}
	api.RespondSuccess(c, nil)
}
