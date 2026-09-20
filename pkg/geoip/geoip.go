package geoip

import (
	"net"
	"strings"
	"unicode"
)

// GeoInfo 是查询结果的通用结构，各 provider 都转换成这个形状返回。
type GeoInfo struct {
	ISOCode string
	Name    string
}

// GeoIPService 接口定义了获取地理位置信息的核心方法。
// 任何实现此接口的类型都可以作为地理位置服务提供者。
type GeoIPService interface {
	Name() string

	GetGeoInfo(ip net.IP) (*GeoInfo, error)

	UpdateDatabase() error

	Close() error
}

// GetRegionUnicodeEmoji 把两位 ISO 国家代码转换成对应的 Unicode 国旗 emoji。
func GetRegionUnicodeEmoji(isoCode string) string {
	if len(isoCode) != 2 {
		return ""
	}
	isoCode = strings.ToUpper(isoCode)

	if !unicode.IsLetter(rune(isoCode[0])) || !unicode.IsLetter(rune(isoCode[1])) {
		return ""
	}

	rune1 := rune(0x1F1E6 + (rune(isoCode[0]) - 'A'))
	rune2 := rune(0x1F1E6 + (rune(isoCode[1]) - 'A'))
	return string(rune1) + string(rune2)
}
