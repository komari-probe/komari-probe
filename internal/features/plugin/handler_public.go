package plugin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ServePublicPluginFile serves a public iframe page declared in a plugin manifest
// (visibility=public). It is reachable without authentication; the plugin
// must be enabled and the requested file must be listed as a public page.
// 与 ServeAdminPluginFile（管理页面静态文件）同名会冲突，故以 Admin/Public 区分。
func ServePublicPluginFile(c *gin.Context) {
	name := strings.TrimPrefix(c.Param("filepath"), "/")
	full, err := ResolvePublicFile(c.Param("short"), name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(full)
}
