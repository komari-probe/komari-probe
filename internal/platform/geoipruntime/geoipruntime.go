package geoipruntime

import (
	"errors"
	"net"
	"time"

	"github.com/komari-monitor/komari/internal/platform/settings"
	provider "github.com/komari-monitor/komari/pkg/geoip"
	"github.com/komari-monitor/komari/pkg/kv"
	"github.com/komari-monitor/komari/pkg/logger"
	"github.com/patrickmn/go-cache"
)

// geoipruntime.go
// Komari 自己的 GeoIP 运行时：根据配置项选择并持有当前生效的 provider、做结果缓存。
// 具体 provider 实现（MaxMind/ip-api/geojs/ipinfo，纯通用逻辑）在 pkg/geoip。

// CurrentProvider is the GeoIP provider currently in effect. It starts as
// EmptyProvider until InitGeoIP selects a real one based on settings.
var CurrentProvider provider.GeoIPService
var geoCache *cache.Cache

func init() {
	CurrentProvider = &provider.EmptyProvider{}
	geoCache = cache.New(48*time.Hour, 1*time.Hour)
}

// InitGeoIP reads the GeoIP settings and installs the configured provider as
// CurrentProvider, falling back to EmptyProvider (and logging) if the
// settings can't be read or the provider fails to initialize. It's called
// both at startup and whenever the provider setting changes, so it must
// never panic: this runs unrecovered inside a goroutine (see internal/app).
func InitGeoIP() {
	conf, err := kv.GetMany(map[string]any{
		settings.GeoIPEnabledKey:  true,
		settings.GeoIPProviderKey: "ipinfo",
	})
	if err != nil {
		logger.Error("geoip", "failed to get configuration for GeoIP; using EmptyProvider", "error", err)
		CurrentProvider = &provider.EmptyProvider{}
		return
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
//
// The success check is on err, not on newProvider being non-nil: the
// constructors return a concrete *T, and a failed constructor returning
// (nil, err) becomes a non-nil provider.GeoIPService interface value here
// (an interface holding a typed nil pointer isn't itself nil), so checking
// newProvider != nil would install a broken provider whose methods panic
// on their nil receiver.
func setGeoIPProvider(name string, construct func() (provider.GeoIPService, error)) {
	newProvider, err := construct()
	if err != nil || newProvider == nil {
		logger.Error("geoip", "failed to initialize "+name+" service; using EmptyProvider", "error", err)
		CurrentProvider = &provider.EmptyProvider{}
		return
	}
	CurrentProvider = newProvider
	logger.Info("geoip", "using GeoIP provider", "provider", name)
}

// Shutdown 关闭当前 GeoIP provider 持有的资源（如 mmdb 文件句柄）。供关闭流程调用。
func Shutdown() error {
	if CurrentProvider == nil {
		return nil
	}
	return CurrentProvider.Close()
}

// GetGeoInfo resolves ip's geographic location using CurrentProvider,
// caching results for 48 hours.
func GetGeoInfo(ip net.IP) (*provider.GeoInfo, error) {
	if CurrentProvider == nil {
		return nil, errors.New("no GeoIP provider is configured")
	}
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

// UpdateDatabase refreshes CurrentProvider's backing data (e.g. re-downloads
// the MaxMind mmdb file) and clears the result cache on success.
func UpdateDatabase() error {
	if CurrentProvider == nil {
		return errors.New("no GeoIP provider is configured")
	}
	err := CurrentProvider.UpdateDatabase()
	if err == nil {
		geoCache.Flush()
		logger.Info("geoip", "cache cleared due to database update")
	}
	return err
}
