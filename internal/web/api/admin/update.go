package admin

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/platform/api"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/internal/platform/geoip"
)

// update.go
// 文件/二进制类的更新接口，保留为 REST handler（不走 RPC 桥）。
// 用户名/密码更新（UpdateUser）已迁移到 internal/features/auth。

func UpdateMmdbGeoIP(c *gin.Context) {
	if err := geoip.UpdateDatabase(); err != nil {
		api.RespondError(c, 500, "Failed to update GeoIP database "+err.Error())
		return
	}
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "GeoIP database updated", "info")
	api.RespondSuccess(c, nil)
}

func UploadFavicon(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20) // 5MB
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			api.RespondError(c, http.StatusRequestEntityTooLarge, "File too large. Maximum size is 5MB")
		} else {
			api.RespondError(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	if err := os.WriteFile("./data/favicon.ico", data, 0644); err != nil {
		api.RespondError(c, http.StatusInternalServerError, "Failed to save favicon: "+err.Error())
		return
	}
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "Favicon uploaded", "info")
	api.RespondSuccess(c, nil)
}

func DeleteFavicon(c *gin.Context) {
	if err := os.Remove("./data/favicon.ico"); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			api.RespondError(c, http.StatusNotFound, "Favicon not found")
		} else {
			api.RespondError(c, http.StatusInternalServerError, "Failed to delete favicon: "+err.Error())
		}
		return
	}
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "Favicon deleted", "info")
	api.RespondSuccess(c, nil)
}
