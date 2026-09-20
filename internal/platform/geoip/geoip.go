package geoip

import (
	"net"
	"time"

	"github.com/komari-monitor/komari/internal/platform/settings"
	provider "github.com/komari-monitor/komari/pkg/geoip"
	config "github.com/komari-monitor/komari/pkg/kv"
	logger "github.com/komari-monitor/komari/pkg/log"
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

func InitGeoIp() {
	conf, err := config.GetMany(map[string]any{
		settings.GeoIpEnabledKey:  true,
		settings.GeoIpProviderKey: "ipinfo",
	})
	if err != nil {
		panic("Failed to get configuration for GeoIP: " + err.Error())
	}
	if !conf[settings.GeoIpEnabledKey].(bool) {
		return
	}
	switch conf[settings.GeoIpProviderKey].(string) {
	case "mmdb":
		NewCurrentProvider, err := provider.NewMaxMindGeoIPService()
		if err != nil {
			logger.Error("geoip", "failed to initialize MaxMind GeoIP service", "error", err)
		}
		if NewCurrentProvider != nil {
			CurrentProvider = NewCurrentProvider
		} else {
			CurrentProvider = &provider.EmptyProvider{}
			logger.Error("geoip", "failed to initialize MaxMind GeoIP service; using EmptyProvider")
		}
	case "ip-api":
		NewCurrentProvider, err := provider.NewIPAPIService()
		if err != nil {
			logger.Error("geoip", "failed to initialize ip-api service", "error", err)
		}
		if NewCurrentProvider != nil {
			CurrentProvider = NewCurrentProvider
			logger.Info("geoip", "using GeoIP provider", "provider", "ip-api.com")
		} else {
			CurrentProvider = &provider.EmptyProvider{}
			logger.Warn("geoip", "failed to initialize ip-api service; using EmptyProvider")
		}
	case "geojs":
		NewCurrentProvider, err := provider.NewGeoJSService()
		if err != nil {
			logger.Error("geoip", "failed to initialize GeoJS service", "error", err)
		}
		if NewCurrentProvider != nil {
			CurrentProvider = NewCurrentProvider
			logger.Info("geoip", "using GeoIP provider", "provider", "geojs.io")
		} else {
			CurrentProvider = &provider.EmptyProvider{}
			logger.Warn("geoip", "failed to initialize GeoJS service; using EmptyProvider")
		}
	case "ipinfo":
		NewCurrentProvider, err := provider.NewIPInfoService()
		if err != nil {
			logger.Error("geoip", "failed to initialize IPInfo service", "error", err)
		}
		if NewCurrentProvider != nil {
			CurrentProvider = NewCurrentProvider
			logger.Info("geoip", "using GeoIP provider", "provider", "ipinfo.io")
		} else {
			CurrentProvider = &provider.EmptyProvider{}
			logger.Warn("geoip", "failed to initialize IPInfo service; using EmptyProvider")
		}
	default:
		CurrentProvider = &provider.EmptyProvider{}
	}
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
