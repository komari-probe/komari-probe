package geoip

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// IPInfoService 使用 ipinfo.io 服务实现 GeoIPService 接口。
type IPInfoService struct {
	noopLifecycle
	Client *http.Client
	// 每天 1000 次请求，限制由 IP 地址的所有人共享。
	// APIToken string
}

// ipInfoResponse 定义了 ipinfo.io 服务返回的 JSON 响应的结构，只包含免费额度可用的字段。
type ipInfoResponse struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"` // ipinfo.io 的 "country" 字段就是 ISO 2-letter code
	Loc      string `json:"loc"`     // Latitude,Longitude
	Org      string `json:"org"`
	Postal   string `json:"postal"`
	Timezone string `json:"timezone"`
}

// NewIPInfoService 创建并返回一个 IPInfoService 的新实例。
func NewIPInfoService() (*IPInfoService, error) {
	return &IPInfoService{
		Client: &http.Client{Timeout: httpProviderTimeout},
	}, nil
}

// Name 返回服务的名称。
func (s *IPInfoService) Name() string {
	return "ipinfo.io"
}

// GetGeoInfo 使用 ipinfo.io 服务检索给定 IP 地址的地理位置信息。
// 免费额度主要提供国家信息。
func (s *IPInfoService) GetGeoInfo(ip net.IP) (*GeoInfo, error) {
	// IPinfo 免费额度不需要 API token 就可以查询基本的 IP 信息。
	// API URL: https://ipinfo.io/json (查询自身IP) 或 https://ipinfo.io/YOUR_IP/json
	apiURL := fmt.Sprintf("https://ipinfo.io/%s/json", ip.String())

	resp, err := s.Client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get geo info from ipinfo.io: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipinfo.io returned non-200 status: %d %s", resp.StatusCode, resp.Status)
	}

	var apiResp ipInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode ipinfo.io response: %w", err)
	}

	// IPinfo 的 "country" 字段直接是 ISO 2-letter code，例如 "US"、"CN"；免费额度
	// 不提供完整国家名称，这里用 ISO 代码同时作为 Name 的占位值。
	return &GeoInfo{
		ISOCode: apiResp.Country, // IPinfo 的 'country' 字段就是 ISO 2-letter code
		Name:    apiResp.Country, // 免费额度通常只提供 ISO 编码，这里暂时用 ISO 编码作为名称
	}, nil
}
