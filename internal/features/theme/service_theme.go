package theme

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/komari-monitor/komari/internal/platform/download"
	"github.com/komari-monitor/komari/internal/platform/market"
	"github.com/komari-monitor/komari/internal/platform/models"
)

const (
	maxThemeArchiveFiles  = 10000
	maxThemeFileSize      = 128 << 20
	maxThemeExtractedSize = 512 << 20
	maxThemeManifestSize  = 1 << 20
)

// InstallZip 解压并验证主题
func InstallZip(zipPath string) (models.Theme, error) {
	var themeInfo models.Theme

	// 打开ZIP文件
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return themeInfo, fmt.Errorf("无法打开ZIP文件: %v", err)
	}
	defer r.Close()

	if err := validateThemeArchive(r.File); err != nil {
		return themeInfo, err
	}

	// 查找komari-theme.json文件
	var themeConfigFile *zip.File
	for _, f := range r.File {
		if f.Name == "komari-theme.json" {
			themeConfigFile = f
			break
		}
	}

	if themeConfigFile == nil {
		return themeInfo, fmt.Errorf("主题配置文件 komari-theme.json 不存在")
	}

	// 读取主题配置
	rc, err := themeConfigFile.Open()
	if err != nil {
		return themeInfo, fmt.Errorf("无法读取主题配置文件: %v", err)
	}
	defer rc.Close()

	configData, err := io.ReadAll(io.LimitReader(rc, maxThemeManifestSize+1))
	if err != nil {
		return themeInfo, fmt.Errorf("读取主题配置失败: %v", err)
	}
	if len(configData) > maxThemeManifestSize {
		return themeInfo, fmt.Errorf("主题配置文件超过 %d 字节限制", maxThemeManifestSize)
	}

	if err := json.Unmarshal(configData, &themeInfo); err != nil {
		return themeInfo, fmt.Errorf("主题配置格式错误: %v", err)
	}

	if err := validateThemeManifest(themeInfo); err != nil {
		return themeInfo, err
	}

	// 创建主题目录
	themeDir := filepath.Join("./data/theme", themeInfo.Short)

	// 如果目录已存在，先删除
	if _, err := os.Stat(themeDir); err == nil {
		if err := os.RemoveAll(themeDir); err != nil {
			return themeInfo, fmt.Errorf("删除原有主题失败: %v", err)
		}
	}

	if err := os.MkdirAll(themeDir, 0755); err != nil {
		return themeInfo, fmt.Errorf("创建主题目录失败: %v", err)
	}

	// 解压文件到主题目录
	for _, f := range r.File {
		path := filepath.Join(themeDir, f.Name)

		// 安全检查，防止路径遍历攻击
		if !strings.HasPrefix(path, filepath.Clean(themeDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.FileInfo().Mode())
			continue
		}

		// 创建目录
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return themeInfo, fmt.Errorf("创建目录失败: %v", err)
		}

		// 解压文件
		rc, err := f.Open()
		if err != nil {
			return themeInfo, fmt.Errorf("打开压缩文件失败: %v", err)
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.FileInfo().Mode())
		if err != nil {
			rc.Close()
			return themeInfo, fmt.Errorf("创建文件失败: %v", err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return themeInfo, fmt.Errorf("解压文件失败: %v", err)
		}
	}

	return themeInfo, nil
}

func validateThemeArchive(files []*zip.File) error {
	if len(files) > maxThemeArchiveFiles {
		return fmt.Errorf("主题压缩包文件数量超过 %d 个限制", maxThemeArchiveFiles)
	}
	var total uint64
	for _, file := range files {
		if file.FileInfo().IsDir() {
			continue
		}
		if file.UncompressedSize64 > maxThemeFileSize {
			return fmt.Errorf("主题文件 %s 超过 %d 字节限制", file.Name, maxThemeFileSize)
		}
		total += file.UncompressedSize64
		if total > maxThemeExtractedSize {
			return fmt.Errorf("主题解压后总大小超过 %d 字节限制", maxThemeExtractedSize)
		}
	}
	return nil
}

// loadThemeConfig 加载主题配置
func loadThemeConfig(configPath string) (models.Theme, error) {
	var themeInfo models.Theme

	data, err := os.ReadFile(configPath)
	if err != nil {
		return themeInfo, err
	}

	if err := json.Unmarshal(data, &themeInfo); err != nil {
		return themeInfo, err
	}

	return themeInfo, nil
}

func validateThemeManifest(themeInfo models.Theme) error {
	if !models.IsLocalizedText(themeInfo.Name) || themeInfo.Short == "" {
		return fmt.Errorf("主题配置缺少必填字段（name、short）")
	}
	if !market.IsValidShort(themeInfo.Short) {
		return fmt.Errorf("主题short字段格式无效，只允许字母、数字、下划线和连字符")
	}
	return themeInfo.ValidateConfiguration()
}

func downloadThemeFromURL(rawURL string) ([]byte, error) {
	return download.DownloadMarketURL(rawURL, download.MaxBytes)
}

// getGitHubReleaseDownloadURL 从GitHub API获取最新release的下载链接
// 该函数通过GitHub API获取指定仓库最新release的资源下载链接
// 参考API: https://api.github.com/repos/{owner}/{repo}/releases/latest
// 参数:
//   - owner: GitHub仓库所有者
//   - repo: GitHub仓库名称
//
// 返回:
//   - 最新release的第一个资源的下载链接
//   - 错误信息（如果有）
func getGitHubReleaseDownloadURL(owner, repo string) (string, error) {
	if owner == "" || repo == "" {
		return "", errors.New("GitHub仓库所有者和仓库名称不能为空")
	}

	// 构建GitHub API URL
	// 使用GitHub API获取最新release信息
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	// 发送HTTP GET请求
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("获取GitHub release信息失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取GitHub release信息失败，HTTP状态码: %d", resp.StatusCode)
	}

	// 解析JSON响应
	// GitHub API返回的JSON包含assets数组，每个asset包含browser_download_url字段
	var releaseInfo struct {
		Assets []struct {
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		return "", fmt.Errorf("解析GitHub API响应失败: %v", err)
	}

	// 检查是否有可下载的资源
	if len(releaseInfo.Assets) == 0 {
		return "", errors.New("GitHub release中没有可下载的资源")
	}

	// 返回第一个资源的下载链接
	// 相当于shell命令: curl -s https://api.github.com/repos/owner/repo/releases/latest | jq -r ".assets[0].browser_download_url"
	return releaseInfo.Assets[0].BrowserDownloadURL, nil
}

// isGitHubRepoURL 检查URL是否是GitHub仓库地址
// 支持的格式:
// - https://github.com/owner/repo
// - https://github.com/owner/repo.git
// - https://www.github.com/owner/repo
// - http://github.com/owner/repo
// 返回:
//   - 是否是GitHub仓库URL
//   - 仓库所有者
//   - 仓库名称
func isGitHubRepoURL(urlStr string) (bool, string, string) {
	if urlStr == "" {
		return false, "", ""
	}

	// 检查URL是否包含github.com
	if !strings.Contains(strings.ToLower(urlStr), "github.com") {
		return false, "", ""
	}

	// 解析URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false, "", ""
	}

	// 检查主机名是否是github.com或www.github.com
	hostname := strings.ToLower(parsedURL.Host)
	if hostname != "github.com" && hostname != "www.github.com" {
		return false, "", ""
	}

	// 解析路径部分，提取owner和repo
	// 路径格式应该是 /owner/repo 或 /owner/repo.git
	path := strings.TrimPrefix(parsedURL.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		return false, "", ""
	}

	owner := parts[0]
	repo := parts[1]

	// 如果repo以.git结尾，去掉这个后缀
	repo = strings.TrimSuffix(repo, ".git")

	return true, owner, repo
}

// peekThemeFromZip 仅从ZIP文件中读取komari-theme.json并解析主题信息
// 不执行解压安装，用于preview模式
func peekThemeFromZip(zipPath string) (models.Theme, error) {
	var themeInfo models.Theme

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return themeInfo, fmt.Errorf("无法打开ZIP文件: %v", err)
	}
	defer r.Close()

	if err := validateThemeArchive(r.File); err != nil {
		return themeInfo, err
	}

	var themeConfigFile *zip.File
	for _, f := range r.File {
		if f.Name == "komari-theme.json" {
			themeConfigFile = f
			break
		}
	}

	if themeConfigFile == nil {
		return themeInfo, fmt.Errorf("主题配置文件 komari-theme.json 不存在，不是合法的主题包")
	}

	rc, err := themeConfigFile.Open()
	if err != nil {
		return themeInfo, fmt.Errorf("无法读取主题配置文件: %v", err)
	}
	defer rc.Close()

	configData, err := io.ReadAll(io.LimitReader(rc, maxThemeManifestSize+1))
	if err != nil {
		return themeInfo, fmt.Errorf("读取主题配置失败: %v", err)
	}
	if len(configData) > maxThemeManifestSize {
		return themeInfo, fmt.Errorf("主题配置文件超过 %d 字节限制", maxThemeManifestSize)
	}

	if err := json.Unmarshal(configData, &themeInfo); err != nil {
		return themeInfo, fmt.Errorf("主题配置格式错误: %v", err)
	}

	if err := validateThemeManifest(themeInfo); err != nil {
		return themeInfo, err
	}

	return themeInfo, nil
}
