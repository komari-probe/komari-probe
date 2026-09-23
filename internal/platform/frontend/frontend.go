package frontend

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/platform/settings"
	"github.com/sonar-probe/sonar/pkg/kv"
)

//go:embed defaultTheme/sonar-theme.json defaultTheme/komari-theme.json defaultTheme/preview.png
var PublicFS embed.FS

//go:embed defaultTheme/dist.tar.zst
var embeddedDistArchive []byte

// 常量定义
const (
	DataDir            = "./data"
	ThemesDir          = "theme"
	FaviconFile        = "favicon.ico"
	DefaultTheme       = "default"
	LanguageCookieName = "language"

	// 主题内部结构定义
	DistDir        = "dist"       // 静态资源存放目录
	IndexFile      = "index.html" // 相对于 DistDir
	AdminDir       = "admin"
	AdminIndexFile = "admin.html"
)

func init() {
	var err error
	defaultDistFiles, err = loadEmbeddedDist()
	if err != nil {
		panic("load embedded default frontend: " + err.Error())
	}
}

// Static 注册静态资源和 SPA 路由处理
func Static(r *gin.RouterGroup, noRoute func(handlers ...gin.HandlerFunc)) {
	static(r, noRoute, false)
}

// StaticRestricted serves only the embedded default frontend. Restricted
// startup listeners must not let an installed theme override same-named JS,
// CSS, manifest, or favicon assets used by login and recovery pages.
func StaticRestricted(r *gin.RouterGroup, noRoute func(handlers ...gin.HandlerFunc)) {
	static(r, noRoute, true)
}

func static(r *gin.RouterGroup, noRoute func(handlers ...gin.HandlerFunc), forceDefaultTheme bool) {
	// 初始化嵌入式文件系统，指向 defaultTheme 根目录。
	defaultThemeFS, err := fs.Sub(PublicFS, "defaultTheme")
	if err != nil {
		panic("embedded default theme metadata is unavailable: " + err.Error())
	}

	getConfig := func() map[string]any {
		cfg, _ := kv.GetMany(map[string]any{
			settings.DescriptionKey: "A simple server monitor tool.",
			settings.CustomHeadKey:  "",
			settings.CustomBodyKey:  "",
			settings.SitenameKey:    "Sonar",
			settings.ThemeKey:       DefaultTheme,
		})
		return cfg
	}

	// 核心逻辑：获取文件内容
	// filePath: 相对于主题根目录的路径 (例如 "theme.json" 或 "dist/assets/a.js")
	// 返回: content, contentType, exists
	getFileContent := func(themeID string, relativePath string) ([]byte, string, bool) {
		cleanPath := strings.TrimPrefix(relativePath, "/")

		cleanPath = filepath.Clean(cleanPath)

		if themeID != DefaultTheme {
			if strings.Contains(themeID, "..") || strings.Contains(themeID, "/") || strings.Contains(themeID, "\\") {
				return nil, "", false
			}

			themeBasePath := filepath.Join(DataDir, ThemesDir, themeID)

			if !isSafePath(themeBasePath, cleanPath) {
				return nil, "", false
			}

			localPath := filepath.Join(themeBasePath, cleanPath)
			// 检查文件是否存在且不是目录
			if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
				content, err := os.ReadFile(localPath)
				if err == nil {
					return content, mime.TypeByExtension(filepath.Ext(localPath)), true
				}
			}
			// 本地文件不存在，或读取失败 -> 继续向下回退
		}

		// 2. 尝试从嵌入式 defaultTheme/{cleanPath} 读取
		// fs.ReadFile 处理 embed 路径时使用 "/"
		embedPath := filepath.ToSlash(cleanPath)

		if strings.Contains(embedPath, "..") {
			return nil, "", false
		}

		if strings.HasPrefix(embedPath, DistDir+"/") {
			if content, ok := defaultDistFiles[strings.TrimPrefix(embedPath, DistDir+"/")]; ok {
				return content, mime.TypeByExtension(filepath.Ext(embedPath)), true
			}
		} else if content, err := fs.ReadFile(defaultThemeFS, embedPath); err == nil {
			return content, mime.TypeByExtension(filepath.Ext(embedPath)), true
		}

		return nil, "", false
	}

	// 核心逻辑：渲染 Index.html
	serveIndex := func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		cfg := getConfig()

		currentTheme := cfg[settings.ThemeKey].(string)
		shouldReplace := true

		// 内置管理应用不属于可替换主题。它与默认主题一起打包，
		// 但使用自己的入口与 assets，避免第三方主题影响控制台。
		isAdminApp := strings.HasPrefix(reqPath, "/admin") ||
			strings.HasPrefix(reqPath, "/manage") ||
			reqPath == "/install" ||
			strings.HasPrefix(reqPath, "/database-recovery")
		if forceDefaultTheme || isAdminApp {
			currentTheme = DefaultTheme
			shouldReplace = false
		}

		targetFile := path.Join(DistDir, IndexFile)
		if isAdminApp {
			targetFile = path.Join(DistDir, AdminDir, AdminIndexFile)
		}
		content, _, exists := getFileContent(currentTheme, targetFile)
		// A source checkout can contain an older embedded archive until the
		// frontend packaging action has run. Keep that checkout usable during
		// the migration; release builds assert and package the admin entry.
		if !exists && isAdminApp {
			targetFile = path.Join(DistDir, IndexFile)
			content, _, exists = getFileContent(DefaultTheme, targetFile)
		}

		if !exists {
			c.String(http.StatusNotFound, "Index file missing (checked %s/%s and default).", currentTheme, targetFile)
			return
		}

		htmlStr := string(content)
		if forceDefaultTheme {
			htmlStr = stripServiceWorkerRegistration(htmlStr)
		}
		if language, err := c.Cookie(LanguageCookieName); err == nil {
			htmlStr = replaceHTMLLanguage(htmlStr, language)
		}

		// 如果不替换，保留系统内置页面内容，仅同步 html lang。
		if !shouldReplace {
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlStr))
			return
		}

		// 执行 HTML 内容替换
		replacer := strings.NewReplacer(
			"<title>Sonar</title>", "<title>"+cfg[settings.SitenameKey].(string)+"</title>",
			"<title>Komari Monitor</title>", "<title>"+cfg[settings.SitenameKey].(string)+"</title>",
			"A simple server monitor tool.", cfg[settings.DescriptionKey].(string),
			"</head>", cfg[settings.CustomHeadKey].(string)+"</head>",
			"</body>", cfg[settings.CustomBodyKey].(string)+"</body>",
		)

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(replacer.Replace(htmlStr)))
	}

	// ================= 路由定义 =================
	// 1. Favicon 优先策略
	r.GET("/favicon.ico", func(c *gin.Context) {
		// 优先：./data/favicon.ico
		localFavicon := filepath.Join(DataDir, FaviconFile)
		if !forceDefaultTheme {
			if _, err := os.Stat(localFavicon); err == nil {
				c.File(localFavicon)
				return
			}
		}

		// 其次：当前主题的 dist/favicon.ico 或 theme_root/favicon.ico ?
		// 通常构建后的资源在 dist 中，这里假设优先找 dist 内的，如果你的 favicon 在根目录，去掉 DistDir 拼接即可
		cfg := getConfig()
		themeFaviconPath := path.Join(DistDir, FaviconFile)
		currentTheme := cfg[settings.ThemeKey].(string)
		if forceDefaultTheme {
			currentTheme = DefaultTheme
		}
		content, mimeType, exists := getFileContent(currentTheme, themeFaviconPath)
		if exists {
			c.Data(http.StatusOK, mimeType, content)
			return
		}

		c.Status(http.StatusNotFound)
	})

	// 2. 静态资源路由 /themes/:id/*path
	// 允许访问 /themes/MyTheme/theme.json 和 /themes/MyTheme/dist/assets/a.js
	r.GET("/themes/:id/*path", func(c *gin.Context) {
		themeID := c.Param("id")
		if forceDefaultTheme && themeID != DefaultTheme {
			c.Status(http.StatusNotFound)
			return
		}
		if forceDefaultTheme {
			themeID = DefaultTheme
		}
		// c.Param("path") 包含了开头的 /，getFileContent 会处理
		filePath := c.Param("path")

		content, mimeType, exists := getFileContent(themeID, filePath)
		if exists {
			c.Data(http.StatusOK, mimeType, content)
			return
		}
		c.Status(http.StatusNotFound)
	})

	// 3. SPA 路由 (noRoute)
	noRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.Status(http.StatusNotFound)
			return
		}
		//
		func() {
			tempKey := c.Query("temp_key")
			if tempKey == "" {
				return
			}

			tempKeyExpireTime, err := kv.GetAs[int64]("tempory_share_token_expire_at", 0)
			if err != nil {
				return
			}
			allowTempKey, err := kv.GetAs[string]("tempory_share_token", "")
			if err != nil {
				return
			}

			if allowTempKey == "" || tempKey != allowTempKey {
				return
			}
			now := time.Now().Unix()
			if tempKeyExpireTime < now {
				return
			}
			expireSeconds := int(tempKeyExpireTime - now)
			if expireSeconds > 0 {
				c.SetCookie(
					"temp_key",    // key
					tempKey,       // value
					expireSeconds, // maxAge（秒）
					"/",           // path
					"",            // domain
					false,         // secure
					false,         // httpOnly
				)
			}
		}()
		reqPath := c.Request.URL.Path
		cfg := getConfig()
		currentTheme := cfg[settings.ThemeKey].(string)
		if forceDefaultTheme {
			currentTheme = DefaultTheme
		}

		// SPA 静态资源回退
		distPath := path.Join(DistDir, reqPath)

		content, mimeType, exists := getFileContent(currentTheme, distPath)
		if exists {
			c.Data(http.StatusOK, mimeType, content)
			return
		}

		// 路由 (如 /dashboard, /settings) -> 返回 index.html
		serveIndex(c)
	})
}
