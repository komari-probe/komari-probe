package geoip

import (
	"net"
	"time"

	"github.com/komari-monitor/komari/internal/platform/settings"
	provider "github.com/komari-monitor/komari/pkg/geoip"
	"github.com/komari-monitor/komari/pkg/kv"
	"github.com/komari-monitor/komari/pkg/logger"
	"github.com/patrickmn/go-cache"
)

// geoip.go
// Komari 自己的 GeoIP 运行时：根据配置项选择并持有当前生效的 provider、做结果缓存。
// 具体 provider 实现（MaxMind/ip-api/geojs/ipinfo，纯通用逻辑）在 pkg/geoip。

var CurrentProvider provider.GeoIPService
var geoCache *cache.Cache

func init() {
	CurrentProvider = &provider.EmptyProvider{}
	geoCache = cache.New(48*time.Hour, 1*time.Hour)
}

func InitGeoIP() {
	conf, err := kv.GetMany(map[string]any{
		settings.GeoIPEnabledKey:  true,
		settings.GeoIPProviderKey: "ipinfo",
	})
	if err != nil {
		panic("Failed to get configuration for GeoIP: " + err.Error())
	}
	if !conf[settings.GeoIPEnabledKey].(bool) {
		return
	}
	switch conf[settings.GeoIPProviderKey].(string) {
	case "mmdb":
		setGeoIPProvider("MaxMind", func() (provider.GeoIPService, error) { return provider.NewMaxMindGeoIPService() })
	case "ip-api":
		setGeoIPProvider("ip-api.com", func() (provider.GeoIPService, error) { return provider.NewIPAPIService() })
	case "geojs":
		setGeoIPProvider("geojs.io", func() (provider.GeoIPService, error) { return provider.NewGeoJSService() })
	case "ipinfo":
		setGeoIPProvider("ipinfo.io", func() (provider.GeoIPService, error) { return provider.NewIPInfoService() })
	default:
		CurrentProvider = &provider.EmptyProvider{}
	}
}

// setGeoIPProvider constructs a provider by name and installs it as
// CurrentProvider, falling back to EmptyProvider (and logging) on failure.
func setGeoIPProvider(name string, construct func() (provider.GeoIPService, error)) {
	newProvider, err := construct()
	if err != nil {
		logger.Error("geoip", "failed to initialize "+name+" service", "error", err)
	}
	if newProvider != nil {
		CurrentProvider = newProvider
		logger.Info("geoip", "using GeoIP provider", "provider", name)
		return
	}
	CurrentProvider = &provider.EmptyProvider{}
	logger.Warn("geoip", "failed to initialize "+name+" service; using EmptyProvider")
}

// Shutdown 关闭当前 GeoIP provider 持有的资源（如 mmdb 文件句柄）。供关闭流程调用。
func Shutdown() error {
	if CurrentProvider == nil {
		return nil
	}
	return CurrentProvider.Close()
}

func GetGeoInfo(ip net.IP) (*provider.GeoInfo, error) {
	providerName := CurrentProvider.Name()
	cacheKey := providerName + ":" + ip.String()

	if cachedInfo, found := geoCache.Get(cacheKey); found {
		return cachedInfo.(*provider.GeoInfo), nil
	}

	info, err := CurrentProvider.GetGeoInfo(ip)
	if err == nil && info != nil {
		geoCache.Set(cacheKey, info, cache.DefaultExpiration)
	}
	return info, err
}

func UpdateDatabase() error {
	err := CurrentProvider.UpdateDatabase()
	if err == nil {
		geoCache.Flush()
		logger.Info("geoip", "cache cleared due to database update")
	}
	return err
}
