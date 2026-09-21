package theme

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/komari-monitor/komari/internal/platform/models"
)

func themeConfigurationType(t models.Theme) string {
	typ := strings.ToLower(strings.TrimSpace(t.Configuration.Type))
	if typ == "" {
		return models.ThemeConfigurationManaged
	}
	return typ
}

func themeRawHTML(t models.Theme) (string, bool) {
	if themeConfigurationType(t) != models.ThemeConfigurationRaw {
		return "", false
	}
	return configurationDataString(t.Configuration.Data)
}

func themeRedirectTarget(t models.Theme) (string, bool) {
	if themeConfigurationType(t) != models.ThemeConfigurationRedirect {
		return "", false
	}
	return normalizeThemeRedirectTarget(t.Configuration.Data)
}

func validateThemeConfiguration(t models.Theme) error {
	switch themeConfigurationType(t) {
	case models.ThemeConfigurationManaged:
		return nil
	case models.ThemeConfigurationRaw:
		html, ok := themeRawHTML(t)
		if !ok || strings.TrimSpace(html) == "" {
			return fmt.Errorf("raw 类型主题需要在 configuration.data 中提供 HTML 字符串")
		}
		return nil
	case models.ThemeConfigurationRedirect:
		if _, ok := themeRedirectTarget(t); !ok {
			return fmt.Errorf("redirect 类型主题需要在 configuration.data 中提供站内相对路径")
		}
		return nil
	default:
		return fmt.Errorf("不支持的主题类型: %s", t.Configuration.Type)
	}
}

func normalizeThemeRedirectTarget(data any) (string, bool) {
	target, ok := configurationDataString(data)
	if !ok {
		return "", false
	}

	target = strings.TrimSpace(target)
	if target == "" || strings.Contains(target, "\\") || strings.HasPrefix(target, "//") {
		return "", false
	}

	parsed, err := url.Parse(target)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "", false
	}

	cleanInputPath := parsed.Path
	if strings.HasPrefix(cleanInputPath, "/") {
		cleanInputPath = strings.TrimLeft(cleanInputPath, "/")
	} else {
		for strings.HasPrefix(cleanInputPath, "../") {
			cleanInputPath = strings.TrimPrefix(cleanInputPath, "../")
		}
	}

	for _, segment := range strings.Split(cleanInputPath, "/") {
		if segment == ".." {
			return "", false
		}
	}

	cleanPath := cleanInputPath
	if cleanPath == "" {
		cleanPath = "/"
	} else {
		cleanPath = path.Clean(cleanPath)
		if cleanPath == "." {
			cleanPath = "/"
		} else {
			cleanPath = "/" + strings.TrimPrefix(cleanPath, "/")
		}
	}

	normalized := url.URL{
		Path:     cleanPath,
		RawQuery: parsed.RawQuery,
		Fragment: parsed.Fragment,
	}
	return normalized.String(), true
}

func configurationDataString(data any) (string, bool) {
	value, ok := data.(string)
	return value, ok
}
