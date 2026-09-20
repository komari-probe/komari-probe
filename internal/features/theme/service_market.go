package theme

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/internal/platform/download"
	"github.com/komari-monitor/komari/internal/platform/market"
)

const defaultThemeMarketURL = "https://raw.githubusercontent.com/komari-monitor/theme-market/main/v1.json"

type ThemeMarketSource struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

type ThemeMarketTheme struct {
	Name        any    `json:"name"`
	Short       string `json:"short"`
	Description any    `json:"description"`
	Version     string `json:"version"`
	Author      any    `json:"author"`
	URL         string `json:"url"`
	Preview     string `json:"preview"`
	Download    string `json:"download"`
	SHA256      string `json:"sha256"`
	Installable bool   `json:"installable"`
	SourceID    string `json:"source_id,omitempty"`
	SourceName  string `json:"source_name,omitempty"`
}

type themeMarketCatalog struct {
	Schema int                `json:"schema"`
	Themes []ThemeMarketTheme `json:"themes"`
}

type themeMarketSourceStatus struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Count int    `json:"count"`
	Error string `json:"error,omitempty"`
}

type cachedThemeMarketCatalog struct {
	Themes    []ThemeMarketTheme
	ExpiresAt time.Time
}

var themeMarketCache = struct {
	sync.RWMutex
	items map[string]cachedThemeMarketCatalog
}{items: make(map[string]cachedThemeMarketCatalog)}

func defaultThemeMarketSources() []ThemeMarketSource {
	return []ThemeMarketSource{{
		ID:      "official",
		Name:    "Komari Official",
		URL:     defaultThemeMarketURL,
		Enabled: true,
	}}
}

func getThemeMarketSources() ([]ThemeMarketSource, error) {
	return config.GetAs[[]ThemeMarketSource](config.ThemeMarketSourcesKey, defaultThemeMarketSources())
}

func saveThemeMarketSources(sources []ThemeMarketSource) error {
	return config.Set(config.ThemeMarketSourcesKey, sources)
}

func normalizeThemeMarketSource(source ThemeMarketSource) (ThemeMarketSource, error) {
	source.ID = strings.TrimSpace(source.ID)
	source.Name = strings.TrimSpace(source.Name)
	source.URL = strings.TrimSpace(source.URL)
	if source.Name == "" {
		return source, errors.New("source name is required")
	}
	parsed, err := url.Parse(source.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return source, errors.New("source URL must be a valid HTTP or HTTPS URL")
	}
	return source, nil
}

func fetchThemeMarketCatalog(source ThemeMarketSource, force bool) ([]ThemeMarketTheme, error) {
	if !force {
		themeMarketCache.RLock()
		cached, ok := themeMarketCache.items[source.URL]
		themeMarketCache.RUnlock()
		if ok && time.Now().Before(cached.ExpiresAt) {
			return append([]ThemeMarketTheme(nil), cached.Themes...), nil
		}
	}
	data, err := download.DownloadMarketURL(source.URL, market.CatalogMaxSize)
	if err != nil {
		return nil, err
	}
	themes, err := parseThemeMarketCatalog(data)
	if err != nil {
		return nil, err
	}
	for i := range themes {
		if err := validateThemeMarketTheme(themes[i]); err != nil {
			return nil, fmt.Errorf("theme %q: %w", themes[i].Short, err)
		}
		themes[i].SHA256 = strings.TrimPrefix(strings.ToLower(themes[i].SHA256), "sha256:")
		themes[i].Installable = themes[i].Download != "" && themes[i].SHA256 != ""
		themes[i].SourceID = source.ID
		themes[i].SourceName = source.Name
	}
	themeMarketCache.Lock()
	themeMarketCache.items[source.URL] = cachedThemeMarketCatalog{Themes: themes, ExpiresAt: time.Now().Add(market.CacheTTL)}
	themeMarketCache.Unlock()
	return append([]ThemeMarketTheme(nil), themes...), nil
}

func parseThemeMarketCatalog(data []byte) ([]ThemeMarketTheme, error) {
	var catalog themeMarketCatalog
	if err := json.Unmarshal(data, &catalog); err == nil && catalog.Themes != nil {
		return catalog.Themes, nil
	}
	var themes []ThemeMarketTheme
	if err := json.Unmarshal(data, &themes); err == nil && themes != nil {
		return themes, nil
	}
	var theme ThemeMarketTheme
	if err := json.Unmarshal(data, &theme); err != nil {
		return nil, fmt.Errorf("invalid market catalog JSON: %w", err)
	}
	if theme.Short == "" {
		return nil, errors.New("market catalog must contain a themes array or a theme object")
	}
	return []ThemeMarketTheme{theme}, nil
}

func validateThemeMarketTheme(theme ThemeMarketTheme) error {
	if !market.IsText(theme.Name) || theme.Short == "" || theme.Version == "" || !market.IsText(theme.Author) {
		return errors.New("name, short, version and author are required")
	}
	if !market.IsValidShort(theme.Short) {
		return errors.New("short contains invalid characters")
	}
	if (theme.Download == "") != (theme.SHA256 == "") {
		return errors.New("download and sha256 must be provided together")
	}
	urls := []struct {
		field string
		value string
	}{{"url", theme.URL}, {"preview", theme.Preview}, {"download", theme.Download}}
	for _, item := range urls {
		field, value := item.field, item.value
		if value == "" && (field == "preview" || field == "download") {
			continue
		}
		if err := market.ValidateURLSyntax(value); err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
	}
	if theme.SHA256 == "" {
		return nil
	}
	sha := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(theme.SHA256)), "sha256:")
	if len(sha) != sha256.Size*2 {
		return errors.New("sha256 must contain 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(sha); err != nil {
		return errors.New("sha256 must contain 64 hexadecimal characters")
	}
	return nil
}

func invalidateThemeMarketCache(rawURL string) {
	themeMarketCache.Lock()
	delete(themeMarketCache.items, rawURL)
	themeMarketCache.Unlock()
}
