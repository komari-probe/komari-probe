package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/platform/api"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
)

// handler_update_user.go
// 更新用户名/密码/SSO 类型，从原 web/api/admin/update.go 中拆出（该文件其余的
// GeoIP/favicon 更新接口与 auth 无关，留在原处）。

func UpdateUser(c *gin.Context) {
	var req struct {
		Uuid     string  `json:"uuid" binding:"required"`
		Name     *string `json:"username"`
		Password *string `json:"password"`
		SsoType  *string `json:"sso_type"`
		TwoFa    string  `json:"2fa_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, 400, "Invalid or missing request body: "+err.Error())
		return
	}
	if req.Password == nil && req.Name == nil {
		api.RespondError(c, 400, "At least one field (username or password) must be provided")
		return
	}
	if req.Name != nil && len(*req.Name) < 3 {
		api.RespondError(c, 400, "Username must be at least 3 characters long")
		return
	}
	if req.Password != nil && len(*req.Password) < 6 {
		api.RespondError(c, 400, "Password must be at least 6 characters long")
		return
	}
	if req.Password != nil {
		c.Set("2fa_code", req.TwoFa)
		if err := VerifySensitive2FA(c); err != nil {
			api.RespondError(c, 401, err.Error())
			return
		}
	}
	if err := updateUserRecord(req.Uuid, req.Name, req.Password, req.SsoType); err != nil {
		api.RespondError(c, 500, "Failed to update user: "+err.Error())
		return
	}
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "User updated", "warn")
	api.RespondSuccess(c, gin.H{"uuid": req.Uuid})
}
