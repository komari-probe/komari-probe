package geoipruntime_test

import (
	"net"
	"testing"

	"github.com/sonar-probe/sonar/internal/platform/geoipruntime"
	provider "github.com/sonar-probe/sonar/pkg/geoip"
)

// 测试GeoIP数据库的初始化和更新功能
func TestMmdb(t *testing.T) {
	svc, err := provider.NewMaxMindGeoIPService()
	if err != nil {
		t.Fatalf("NewMaxMindGeoIPService: %v", err)
	}
	geoipruntime.CurrentProvider = svc
	testIPAddr(t)
}
func TestIpApi(t *testing.T) {
	svc, err := provider.NewIPAPIService()
	if err != nil {
		t.Fatalf("NewIPAPIService: %v", err)
	}
	geoipruntime.CurrentProvider = svc
	testIPAddr(t)
}

func TestGeojs(t *testing.T) {
	svc, err := provider.NewGeoJSService()
	if err != nil {
		t.Fatalf("NewGeoJSService: %v", err)
	}
	geoipruntime.CurrentProvider = svc
	testIPAddr(t)
}

func TestIpInfo(t *testing.T) {
	svc, err := provider.NewIPInfoService()
	if err != nil {
		t.Fatalf("NewIPInfoService: %v", err)
	}
	geoipruntime.CurrentProvider = svc
	testIPAddr(t)
}
func testIPAddr(t *testing.T) {
	// IPv4
	ipaddr := "8.8.8.8"
	ip := net.ParseIP(ipaddr)
	record, err := geoipruntime.GetGeoInfo(ip)
	if err != nil {
		t.Errorf("Failed to get GeoIP info for IP %s: %v", ipaddr, err)
	}

	if record != nil {
		if record.ISOCode == "" && record.Name == "" {
			t.Errorf("Country information is missing for IP %s", ipaddr)
		}
	} else {
		t.Errorf("GeoIP record is nil for IP %s", ipaddr)
	}

	t.Logf("IPv4:[%s]%s - %s", ipaddr, record.ISOCode, record.Name)

	// IPv6
	ipaddr = "2001:4860:4860::8888"
	ip = net.ParseIP(ipaddr)
	record, err = geoipruntime.GetGeoInfo(ip)
	if err != nil {
		t.Errorf("Failed to get GeoIP info for IPv6 %s: %v", ipaddr, err)
	}
	if record != nil {
		if record.ISOCode == "" && record.Name == "" {
			t.Errorf("Country information is missing for IPv6 %s", ipaddr)
		}
	} else {
		t.Errorf("GeoIP record is nil for IPv6 %s", ipaddr)
	}
	t.Logf("IPv6:[%s]%s - %s", ipaddr, record.ISOCode, record.Name)
}

func TestUnicodeEmoji(t *testing.T) {
	ISOCode := "CN"
	emoji := provider.GetRegionUnicodeEmoji(ISOCode)
	if emoji != "🇨🇳" {
		t.Errorf("Expected emoji for %s, got %s", ISOCode, emoji)
	}
	t.Logf("Emoji for %s: %s", ISOCode, emoji)
}
