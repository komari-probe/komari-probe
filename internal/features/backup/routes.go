package backup

import "github.com/gin-gonic/gin"

// RegisterRoutes binds the backup feature's admin routes under g (typically
// /api/admin). Restoring from a backup goes through the shared chunked-upload
// endpoint (see internal/transport/router's upload wiring), not through here.
func RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/download/backup", DownloadBackup)
}
