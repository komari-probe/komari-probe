package node

import (
	"net"

	"github.com/komari-monitor/komari/internal/platform/clients"
	"github.com/komari-monitor/komari/internal/platform/geoipruntime"
	"github.com/komari-monitor/komari/internal/platform/settings"
	geoipprovider "github.com/komari-monitor/komari/pkg/geoip"
	"github.com/komari-monitor/komari/pkg/kv"
)

func getClientIPType(ip net.IP) int {
	// 0:ipv4 1:ipv6 -1:错误的输入
	if ip == nil {
		return -1
	}
	if ip.To4() == nil {
		return 1
	}
	return 0
}

func saveClientBasicInfo(info map[string]any, uuid string, fallbackIP string) error {
	info["uuid"] = uuid
	applyFallbackClientIP(info, fallbackIP)
	appendClientRegionFromGeoIP(info)
	return clients.SaveClientInfo(info)
}

func applyFallbackClientIP(info map[string]any, fallbackIP string) {
	if hasClientIP(info) {
		return
	}
	ip := net.ParseIP(fallbackIP)

	switch getClientIPType(ip) {
	case 0:
		info["ipv4"] = fallbackIP
	case 1:
		info["ipv6"] = fallbackIP
	}
}

func hasClientIP(info map[string]any) bool {
	if ipv4, ok := info["ipv4"].(string); ok && ipv4 != "" {
		return true
	}
	if ipv6, ok := info["ipv6"].(string); ok && ipv6 != "" {
		return true
	}
	return false
}

func appendClientRegionFromGeoIP(info map[string]any) {
	cfg, err := kv.GetAs[bool](settings.GeoIPEnabledKey, true)
	if err != nil || !cfg {
		return
	}

	for _, key := range []string{"ipv4", "ipv6"} {
		ipStr, ok := info[key].(string)
		if !ok || ipStr == "" {
			continue
		}
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		record, _ := geoipruntime.GetGeoInfo(ip)
		if record == nil {
			continue
		}
		region := geoipprovider.GetRegionUnicodeEmoji(record.ISOCode)
		if region == "" {
			continue
		}
		info["region"] = region
		return
	}
}
