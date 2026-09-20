package geoip // 与 geoip.go 保持相同的包名，表示它们是同一个包的组成部分

import (
	"errors"
	"fmt"
	"github.com/komari-monitor/komari/pkg/logger"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath" // 新增导入，用于处理文件路径
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// GeoIPURL 是 MaxMind 数据库的下载地址。
var GeoIPURL = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb"

// GeoIPFilePath 是本地存储 MaxMind 数据库的路径。
var GeoIPFilePath = "./data/GeoLite2-Country.mmdb"

// geoIPDownloadTimeout 限制数据库下载耗时。downloadDatabase 在持有写锁期间同步
// 执行这次下载，远端卡住时如果不设超时会让锁无限期无法释放，导致所有查询挂起。
var geoIPDownloadTimeout = 60 * time.Second

// geoIPDownloadClient 是下载 MaxMind 数据库专用的 HTTP 客户端。
var geoIPDownloadClient = &http.Client{Timeout: geoIPDownloadTimeout}

// GeoIPRecord 结构体定义了 MaxMind 数据库查询结果的原始结构。
// 它是 MaxMind 库特有的，用于从 .mmdb 文件中解析数据。
type GeoIPRecord struct {
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
}

// MaxMindGeoIPService 是 GeoIPService 接口的一个具体实现，
// 它使用 MaxMind 数据库作为后端。
type MaxMindGeoIPService struct {
	// maxMindDBReader 内部持有的 MaxMind 数据库读取器实例。
	maxMindDBReader *maxminddb.Reader
	// dbFilePath 是 MaxMind 数据库文件的路径。
	dbFilePath string
	// mu 用于保护对 maxMindDBReader 的并发访问，确保线程安全。
	mu sync.RWMutex
}

// Name 返回服务的名称。
func (s *MaxMindGeoIPService) Name() string {
	return "MaxMind"
}

// NewMaxMindGeoIPService 创建并返回一个 MaxMindGeoIPService 实例。
// 它负责初始化服务，包括尝试加载或下载数据库。
func NewMaxMindGeoIPService() (*MaxMindGeoIPService, error) {
	dbFilePath := GeoIPFilePath
	service := &MaxMindGeoIPService{
		dbFilePath: dbFilePath,
	}

	// 确保数据目录存在
	if err := os.MkdirAll(filepath.Dir(dbFilePath), os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create data directory for MaxMind database: %w", err)
	}

	// 检查数据库文件是否存在，如果不存在则尝试下载
	if _, err := os.Stat(dbFilePath); errors.Is(err, fs.ErrNotExist) {
		if err := service.UpdateDatabase(); err != nil {
			return nil, fmt.Errorf("failed to download initial MaxMind database: %w", err)
		}
	}

	// 初始化或重新加载 MaxMind 数据库。
	if err := service.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize MaxMind database: %w", err)
	}
	return service, nil
}

// initialize 初始化或重新加载 MaxMind 数据库。
// 这是一个内部方法，供 NewMaxMindGeoIPService 和 UpdateDatabase 调用。
// 它会关闭现有连接（如果存在）并重新打开数据库文件。
func (s *MaxMindGeoIPService) initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果已存在数据库读取器，则先关闭它。
	if s.maxMindDBReader != nil {
		s.maxMindDBReader.Close()
		s.maxMindDBReader = nil
	}

	// 尝试打开新的数据库文件。
	reader, err := maxminddb.Open(s.dbFilePath)
	if err != nil {
		return fmt.Errorf("error opening MaxMind database at %s: %w", s.dbFilePath, err)
	}
	s.maxMindDBReader = reader
	return nil
}

// GetGeoInfo 根据 IP 地址获取 MaxMind 的地理位置信息。
// 它查询 MaxMind 数据库并将其特有的 GeoIPRecord 转换为通用的 GeoInfo 结构体。
func (s *MaxMindGeoIPService) GetGeoInfo(ip net.IP) (*GeoInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.maxMindDBReader == nil {
		return nil, fmt.Errorf("MaxMind database is not initialized or failed to open")
	}
	if ip == nil {
		return nil, fmt.Errorf("IP address cannot be nil")
	}

	var record GeoIPRecord // 使用原始的 GeoIPRecord 结构体来接收查询结果
	err := s.maxMindDBReader.Lookup(ip, &record)
	if err != nil {
		// 返回错误，但避免直接返回 maxminddb 库的内部错误，提供更友好的信息
		return nil, fmt.Errorf("error looking up IP %s in MaxMind database: %w", ip.String(), err)
	}

	// 将 MaxMind 的特定结构体转换为通用的 GeoInfo 结构体
	geoInfo := &GeoInfo{
		ISOCode: record.Country.ISOCode,
		// 尝试获取英文国家名称，如果不存在则使用 ISO 代码作为备用
		Name: record.Country.Names["en"],
	}
	if geoInfo.Name == "" && geoInfo.ISOCode != "" {
		geoInfo.Name = geoInfo.ISOCode // 如果没有英文名称，回退到 ISO 代码
	}
	return geoInfo, nil
}

// UpdateDatabase 实现了 GeoIPService 接口的 UpdateDatabase 方法。
// 它会下载最新的 GeoLite2-Country.mmdb 文件并重新加载数据库。
func (s *MaxMindGeoIPService) UpdateDatabase() error {
	if err := s.downloadDatabase(); err != nil {
		return err
	}
	// initialize 自己获取锁，必须在 downloadDatabase 释放锁之后调用，以避免死锁。
	return s.initialize()
}

// downloadDatabase 下载最新的 MaxMind 数据库文件并写入本地磁盘。
// 用 defer 释放锁，确保下载或写盘失败时不会让锁永久保持锁定状态。
func (s *MaxMindGeoIPService) downloadDatabase() error {
	s.mu.Lock() // 获取写锁，确保更新过程的互斥性
	defer s.mu.Unlock()

	resp, err := geoIPDownloadClient.Get(GeoIPURL) // GeoIPURL 是预定义的 MaxMind 数据库下载地址
	if err != nil {
		return fmt.Errorf("failed to initiate MaxMind database download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download MaxMind database: HTTP status %s", resp.Status)
	}

	// 确保数据目录存在（NewMaxMindGeoIPService 已处理，但这里再次确保以防直接调用）
	if err := os.MkdirAll(filepath.Dir(s.dbFilePath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create data directory for MaxMind database update: %w", err)
	}

	out, err := os.Create(s.dbFilePath) // 创建或覆盖本地数据库文件
	if err != nil {
		return fmt.Errorf("failed to create MaxMind database file at %s: %w", s.dbFilePath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body) // 将下载内容写入文件
	if err != nil {
		return fmt.Errorf("failed to write MaxMind database file: %w", err)
	}
	return nil
}

// Close 实现了 GeoIPService 接口的 Close 方法。
// 它关闭 MaxMind 数据库读取器，释放文件句柄和其他资源。
func (s *MaxMindGeoIPService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxMindDBReader != nil {
		err := s.maxMindDBReader.Close()
		s.maxMindDBReader = nil // 清空读取器实例
		if err != nil {
			return fmt.Errorf("error closing MaxMind database: %w", err)
		}
	}
	logger.InfoArgs("geoip", "MaxMind GeoIP service closed.")
	return nil
}
