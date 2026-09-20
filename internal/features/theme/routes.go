package theme

import "github.com/gin-gonic/gin"

// RegisterRoutes binds the theme feature's admin routes under g (typically
// /api/admin). Theme installation goes through the shared chunked-upload
// endpoint; everything else here is a plain REST handler.
func RegisterRoutes(g *gin.RouterGroup) {
	t := g.Group("/theme")
	{
		t.GET("/list", ListThemes)
		t.POST("/delete", DeleteTheme)
		t.GET("/set", SetTheme)
		t.POST("/update", UpdateTheme)
		t.POST("/import", ImportTheme)
		t.POST("/settings", UpdateThemeSettings)
		t.GET("/market/sources", ListThemeMarketSources)
		t.POST("/market/sources", CreateThemeMarketSource)
		t.PUT("/market/sources/:id", UpdateThemeMarketSource)
		t.DELETE("/market/sources/:id", DeleteThemeMarketSource)
		t.GET("/market/catalog", ListThemeMarketCatalog)
		t.POST("/market/install", InstallThemeFromMarket)
	}
}
