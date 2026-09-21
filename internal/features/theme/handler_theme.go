package theme

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/market"
	"github.com/komari-monitor/komari/internal/platform/models"
	"github.com/komari-monitor/komari/internal/platform/public"
	"github.com/komari-monitor/komari/internal/platform/respond"
	"github.com/komari-monitor/komari/pkg/kv"
)

// ListThemes 列出所有主题
func ListThemes(c *gin.Context) {
	dataDir := "./data/theme"

	// 确保主题目录存在
	if _, err := os.Stat(dataDir); errors.Is(err, fs.ErrNotExist) {
		respond.Success(c, []models.Theme{})
		return
	}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "读取主题目录失败: "+err.Error())
		return
	}

	var themes []models.Theme
	defaultTheme, err := public.PublicFS.ReadFile("defaultTheme/komari-theme.json")
	if err == nil {
		dt := models.Theme{}
		err := json.Unmarshal(defaultTheme, &dt)
		if err == nil {
			themes = append(themes, dt)
		}

	}
	for _, entry := range entries {
		if entry.IsDir() {
			themeConfigPath := filepath.Join(dataDir, entry.Name(), "komari-theme.json")
			if themeInfo, err := loadThemeConfig(themeConfigPath); err == nil {
				themes = append(themes, themeInfo)
			}
		}
	}

	respond.Success(c, themes)
}

// DeleteTheme 删除主题
func DeleteTheme(c *gin.Context) {
	var req struct {
		Short string `json:"short" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	if req.Short == "default" {
		respond.Error(c, http.StatusBadRequest, "默认主题不能删除")
		return
	}

	// 校验主题短名称，防止路径穿越（如 ../）导致删除工作目录外的任意文件
	if !market.IsValidShort(req.Short) {
		respond.Error(c, http.StatusBadRequest, "无效的主题名称")
		return
	}

	themeDir := filepath.Join("./data/theme", req.Short)

	// 检查主题是否存在
	if _, err := os.Stat(themeDir); errors.Is(err, fs.ErrNotExist) {
		respond.Error(c, http.StatusNotFound, "主题不存在")
		return
	}

	// 删除主题目录
	if err := os.RemoveAll(themeDir); err != nil {
		respond.Error(c, http.StatusInternalServerError, "删除主题失败: "+err.Error())
		return
	}

	respond.SuccessMessage(c, "主题删除成功", nil)
}

// SetTheme 设置主题
func SetTheme(c *gin.Context) {
	themeName := c.Query("theme")
	if themeName == "" {
		respond.Error(c, http.StatusBadRequest, "主题名称不能为空")
		return
	}

	// 如果不是default主题，检查主题是否存在
	if themeName != "default" {
		// 校验主题名称，防止路径穿越（如 ../）访问工作目录外的文件
		if !market.IsValidShort(themeName) {
			respond.Error(c, http.StatusBadRequest, "无效的主题名称")
			return
		}
		themeDir := filepath.Join("./data/theme", themeName)
		themeConfigPath := filepath.Join(themeDir, "komari-theme.json")

		if _, err := os.Stat(themeConfigPath); errors.Is(err, fs.ErrNotExist) {
			respond.Error(c, http.StatusNotFound, "主题不存在")
			return
		}
	}

	if err := kv.Set("theme", themeName); err != nil {
		respond.Error(c, http.StatusInternalServerError, "更新主题设置失败: "+err.Error())
		return
	}

	respond.SuccessMessage(c, "主题设置成功", gin.H{"theme": themeName})
}

// UpdateTheme 更新主题
// 支持四种更新方式：
// 1. 使用主题原有URL下载更新
// 2. 提供新的直接下载URL进行更新
// 3. 提供GitHub仓库信息，从最新release下载更新
// 4. 如果主题URL是GitHub仓库地址，自动获取最新release
func UpdateTheme(c *gin.Context) {
	var req struct {
		Short    string `json:"short" binding:"required"` // 主题短名称
		URL      string `json:"url"`                      // 新的URL地址（可选）
		GitOwner string `json:"git_owner"`                // GitHub仓库所有者（可选）
		GitRepo  string `json:"git_repo"`                 // GitHub仓库名称（可选）
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	// 校验主题短名称，防止路径穿越（如 ../）访问工作目录外的文件
	if !market.IsValidShort(req.Short) {
		respond.Error(c, http.StatusBadRequest, "无效的主题名称")
		return
	}

	// 检查主题是否存在
	themeDir := filepath.Join("./data/theme", req.Short)
	themeConfigPath := filepath.Join(themeDir, "komari-theme.json")

	if _, err := os.Stat(themeConfigPath); errors.Is(err, fs.ErrNotExist) {
		respond.Error(c, http.StatusNotFound, "主题不存在")
		return
	}

	// 加载现有主题配置
	themeInfo, err := loadThemeConfig(themeConfigPath)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "读取主题配置失败: "+err.Error())
		return
	}

	// 方式1和方式4: 尝试从原始URL下载主题
	// 如果原始URL是GitHub仓库地址，则自动获取最新release
	// 不保存下载链接，更新后由主题覆盖
	var themeData []byte

	if themeInfo.URL != "" {
		// 检查原始URL是否是GitHub仓库地址
		// 例如: https://github.com/owner/repo
		isGitHub, owner, repo := isGitHubRepoURL(themeInfo.URL)
		if isGitHub {
			// 方式4: 如果原始URL是GitHub仓库地址，自动获取最新release
			// 这是本次需求的核心功能：当主题文件中现有的url地址如果是github仓库的路径，则直接引用该url地址去下载最新的release
			gitHubURL, err := getGitHubReleaseDownloadURL(owner, repo)
			if err == nil {
				// 使用获取到的GitHub release下载链接下载主题
				themeData, _ = downloadThemeFromURL(gitHubURL)
			}
		} else {
			// 原始URL不是GitHub仓库地址，直接尝试下载（方式1）
			themeData, _ = downloadThemeFromURL(themeInfo.URL)
		}
	}

	// 如果原始URL下载失败，尝试其他方式下载
	if themeData == nil || len(themeData) == 0 {
		// 方式3: 如果提供了GitHub仓库信息，尝试从GitHub最新release下载
		// 这种方式允许用户只需提供owner和repo信息，系统会自动获取最新release的下载链接
		if req.GitOwner != "" && req.GitRepo != "" {
			// 从GitHub API获取下载链接
			// 相当于: DOWNLOAD_URL=$(curl -s https://respond.github.com/repos/owner/repo/releases/latest | jq -r ".assets[0].browser_download_url")
			gitHubURL, err := getGitHubReleaseDownloadURL(req.GitOwner, req.GitRepo)
			if err != nil {
				respond.Error(c, http.StatusBadRequest, "从GitHub获取下载链接失败: "+err.Error())
				return
			}

			// 使用获取到的链接下载主题
			themeData, err = downloadThemeFromURL(gitHubURL)
			if err != nil {
				respond.Error(c, http.StatusBadRequest, "从GitHub下载主题失败: "+err.Error())
				return
			}
		} else if req.URL != "" {
			// 方式2: 如果提供了新URL，尝试从新URL下载
			// 检查新URL是否是GitHub仓库地址
			isGitHub, owner, repo := isGitHubRepoURL(req.URL)
			if isGitHub {
				// 如果新URL是GitHub仓库地址，获取最新release
				// 这里也应用了自动检测GitHub仓库并下载最新release的功能
				gitHubURL, err := getGitHubReleaseDownloadURL(owner, repo)
				if err != nil {
					respond.Error(c, http.StatusBadRequest, "从GitHub获取下载链接失败: "+err.Error())
					return
				}

				// 使用获取到的链接下载主题
				themeData, err = downloadThemeFromURL(gitHubURL)
				if err != nil {
					respond.Error(c, http.StatusBadRequest, "从GitHub下载主题失败: "+err.Error())
					return
				}
			} else {
				// 新URL不是GitHub仓库地址，直接尝试下载
				themeData, err = downloadThemeFromURL(req.URL)
				if err != nil {
					respond.Error(c, http.StatusBadRequest, "从新URL下载主题失败: "+err.Error())
					return
				}
			}
		}
	}

	// 如果没有成功下载主题数据
	if themeData == nil || len(themeData) == 0 {
		respond.Error(c, http.StatusBadRequest, "无法下载主题，请提供有效的URL或GitHub仓库信息")
		return
	}

	// 到这里，我们已经成功获取了主题数据，可能是通过以下四种方式之一：
	// 1. 原始URL直接下载
	// 2. 原始URL是GitHub仓库，自动获取最新release下载
	// 3. 用户提供的新URL下载
	// 4. 用户提供的GitHub仓库信息，获取最新release下载

	// 临时文件名
	tempFile := filepath.Join(os.TempDir(), "downloaded_theme.zip")
	if err := os.WriteFile(tempFile, themeData, 0644); err != nil {
		respond.Error(c, http.StatusInternalServerError, "保存文件失败: "+err.Error())
		return
	}
	defer os.Remove(tempFile)

	// 解压ZIP文件并验证
	updatedThemeInfo, err := InstallZip(tempFile)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	respond.SuccessMessage(c, "主题更新成功", updatedThemeInfo)
}

// ImportTheme 导入远程主题
// 支持preview查询参数：preview=true时仅返回主题信息，否则下载安装
// 请求body: {"url": "https://..."}
// URL支持GitHub仓库地址（自动取latest release）和直接ZIP下载链接
func ImportTheme(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	// 解析下载链接
	downloadURL := req.URL
	isGitHub, owner, repo := isGitHubRepoURL(req.URL)
	if isGitHub {
		gitHubURL, err := getGitHubReleaseDownloadURL(owner, repo)
		if err != nil {
			respond.Error(c, http.StatusBadRequest, "从GitHub获取下载链接失败: "+err.Error())
			return
		}
		downloadURL = gitHubURL
	}

	// 下载主题ZIP
	themeData, err := downloadThemeFromURL(downloadURL)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "下载主题失败: "+err.Error())
		return
	}

	// 保存到临时文件
	tempFile := filepath.Join(os.TempDir(), "import_theme.zip")
	if err := os.WriteFile(tempFile, themeData, 0644); err != nil {
		respond.Error(c, http.StatusInternalServerError, "保存文件失败: "+err.Error())
		return
	}
	defer os.Remove(tempFile)

	// preview模式：仅解析并返回主题信息
	preview := c.Query("preview")
	if preview == "true" {
		themeInfo, err := peekThemeFromZip(tempFile)
		if err != nil {
			respond.Error(c, http.StatusBadRequest, err.Error())
			return
		}

		// 检查是否已存在同名主题
		exists := false
		themeDir := filepath.Join("./data/theme", themeInfo.Short)
		if _, err := os.Stat(themeDir); err == nil {
			exists = true
		}

		respond.Success(c, gin.H{
			"theme":  themeInfo,
			"exists": exists,
		})
		return
	}

	// 安装模式：检查是否存在同名主题
	// 先peek一下获取short名称用于检测冲突
	themeInfo, err := peekThemeFromZip(tempFile)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	overwritten := false
	themeDir := filepath.Join("./data/theme", themeInfo.Short)
	if _, err := os.Stat(themeDir); err == nil {
		overwritten = true
	}

	// 解压安装
	installedTheme, err := InstallZip(tempFile)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	msg := "主题导入成功"
	if overwritten {
		msg = "主题导入成功（已覆盖同名主题）"
	}

	respond.SuccessMessage(c, msg, installedTheme)
}

func UpdateThemeSettings(c *gin.Context) {
	theme := c.Query("theme")
	if theme == "" {
		respond.Error(c, http.StatusBadRequest, "主题名称不能为空")
		return
	}
	var req map[string]any

	err := c.ShouldBindJSON(&req)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	db := dbcore.GetDBInstance()

	data, err := json.Marshal(&req)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "生成主题配置失败: "+err.Error())
		return
	}

	var themeCfg models.ThemeConfiguration
	if err := db.Where("short = ?", theme).
		Assign(models.ThemeConfiguration{Short: theme, Data: string(data)}).
		FirstOrCreate(&themeCfg).Error; err != nil {
		respond.Error(c, http.StatusInternalServerError, "保存主题配置失败: "+err.Error())
		return
	}
	respond.Success(c, nil)
}
