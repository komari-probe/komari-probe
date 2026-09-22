package admin

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/platform/auditlog"
	"github.com/sonar-probe/sonar/internal/platform/geoipruntime"
	"github.com/sonar-probe/sonar/internal/platform/respond"
)

// update.go
// 文件/二进制类的更新接口，保留为 REST handler（不走 RPC 桥）。
// 用户名/密码更新（UpdateUser）已迁移到 internal/features/auth。

func UpdateMmdbGeoIP(c *gin.Context) {
	if err := geoipruntime.UpdateDatabase(); err != nil {
		respond.Error(c, 500, "Failed to update GeoIP database "+err.Error())
		return
	}
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "GeoIP database updated", "info")
	respond.Success(c, nil)
}

func UploadFavicon(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20) // 5MB
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			respond.Error(c, http.StatusRequestEntityTooLarge, "File too large. Maximum size is 5MB")
		} else {
			respond.Error(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	if err := os.WriteFile("./data/favicon.ico", data, 0644); err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to save favicon: "+err.Error())
		return
	}
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "Favicon uploaded", "info")
	respond.Success(c, nil)
}

func DeleteFavicon(c *gin.Context) {
	if err := os.Remove("./data/favicon.ico"); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			respond.Error(c, http.StatusNotFound, "Favicon not found")
		} else {
			respond.Error(c, http.StatusInternalServerError, "Failed to delete favicon: "+err.Error())
		}
		return
	}
	uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), uuid.(string), "Favicon deleted", "info")
	respond.Success(c, nil)
}
