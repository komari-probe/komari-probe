package models

import (
	"strings"
)

const (
	ThemeConfigurationManaged  = "managed"
	ThemeConfigurationRaw      = "raw"
	ThemeConfigurationRedirect = "redirect"
)

// Theme represents a komari theme information
type Theme struct {
	Name          any           `json:"name"`          // 主题名称，支持字符串或多语言对象
	Short         string        `json:"short"`         // 短名称，用作文件夹名
	Description   any           `json:"description"`   // 主题描述，支持字符串或多语言对象
	Version       string        `json:"version"`       // 版本号
	Author        any           `json:"author"`        // 作者，支持字符串或多语言对象
	URL           string        `json:"url"`           // 主题URL
	Preview       string        `json:"preview"`       // 预览图片相对路径
	Configuration Configuration `json:"configuration"` // 声明配置项
}

// IsLocalizedText reports whether a manifest text field has a usable value.
func IsLocalizedText(value any) bool {
	switch text := value.(type) {
	case string:
		return strings.TrimSpace(text) != ""
	case map[string]any:
		for _, item := range text {
			if itemText, ok := item.(string); ok && strings.TrimSpace(itemText) != "" {
				return true
			}
		}
	case map[string]string:
		for _, item := range text {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	}
	return false
}

type Configuration struct {
	Type string `json:"type"` // managed raw redirect
	Icon string `json:"icon"` // 图标
	Name any    `json:"name"`
	Data any    `json:"data"` // 配置数据
}

type ManagedThemeConfigurationItem struct {
	Key      string `json:"key"`
	Name     any    `json:"name"`
	Required bool   `json:"required"`
	Type     string `json:"type"` // string number select switch title textbox richtext nodes pingtasks
	Options  string `json:"options"`
	Default  any    `json:"default"`
	Help     any    `json:"help"`
}

type ThemeConfiguration struct {
	Short string `json:"short" gorm:"primaryKey;unique;not null"`
	Data  string `json:"data" gorm:"type:longtext;default:'{}'"`
}
