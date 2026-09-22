package node

import (
	"sort"
	"sync"
	"time"

	"github.com/sonar-probe/sonar/internal/platform/protocol"
	"github.com/sonar-probe/sonar/pkg/wsconn"
)

var (
	connectedClients = make(map[string]*wsconn.SafeConn)
	v2Clients        = make(map[string]struct{})
	latestReport     = make(map[string]*protocol.Report)
	recentReports    = make(map[string][]protocol.Report)
	// presenceOnly stores online state for non-WebSocket agents.
	// value keeps connectionID and a soft expiration to avoid flicker
	presenceOnly = make(map[string]struct {
		id     int64
		expire time.Time
	})
	connMu = sync.RWMutex{}
)

const recentReportRetention = time.Minute

func GetConnectedClients() map[string]*wsconn.SafeConn {
	connMu.RLock()
	defer connMu.RUnlock()
	clientsCopy := make(map[string]*wsconn.SafeConn)
	for k, v := range connectedClients {
		clientsCopy[k] = v
	}
	return clientsCopy
}

func SetConnectedClients(uuid string, conn *wsconn.SafeConn) {
	connMu.Lock()
	defer connMu.Unlock()
	connectedClients[uuid] = conn
}

func MarkV2Client(uuid string) {
	connMu.Lock()
	defer connMu.Unlock()
	v2Clients[uuid] = struct{}{}
}

func IsV2Client(uuid string) bool {
	connMu.RLock()
	defer connMu.RUnlock()
	_, ok := v2Clients[uuid]
	return ok
}

func DeleteClientConditionally(uuid string, connToRemove *wsconn.SafeConn) {
	connMu.Lock()
	defer connMu.Unlock()

	// 检查当前 map 里的 conn 是否就是要删除的这一个
	if currentConn, exists := connectedClients[uuid]; exists && currentConn == connToRemove {
		delete(connectedClients, uuid)
		delete(v2Clients, uuid)
	}
}
func DeleteConnectedClients(uuid string) {
	connMu.Lock()
	defer connMu.Unlock()
	// 只从 map 中删除，不再负责关闭连接
	delete(connectedClients, uuid)
	delete(v2Clients, uuid)
}

// SetPresence sets or clears presence for non-WebSocket agents.
// When present=false, it only clears if the connectionID matches current one.
// KeepAlivePresence sets presence with TTL for non-WebSocket agents.
func KeepAlivePresence(uuid string, connectionID int64, ttl time.Duration) {
	connMu.Lock()
	defer connMu.Unlock()
	presenceOnly[uuid] = struct {
		id     int64
		expire time.Time
	}{id: connectionID, expire: time.Now().Add(ttl)}
}

var defaultPresenceTTL = 20 * time.Second

// SetPresence keeps compatibility with existing callers.
func SetPresence(uuid string, connectionID int64, present bool) {
	connMu.Lock()
	defer connMu.Unlock()
	if present {
		presenceOnly[uuid] = struct {
			id     int64
			expire time.Time
		}{id: connectionID, expire: time.Now().Add(defaultPresenceTTL)}
		return
	}
	if cur, ok := presenceOnly[uuid]; ok && cur.id == connectionID {
		delete(presenceOnly, uuid)
	}
}

// GetAllOnlineUUIDs returns a de-duplicated list of online UUIDs from both WebSocket and non-WebSocket agents.
func GetAllOnlineUUIDs() []string {
	connMu.RLock()
	defer connMu.RUnlock()
	set := make(map[string]struct{})
	for k := range connectedClients {
		set[k] = struct{}{}
	}
	now := time.Now()
	for k, v := range presenceOnly {
		if v.expire.After(now) {
			set[k] = struct{}{}
		}
	}
	res := make([]string, 0, len(set))
	for k := range set {
		res = append(res, k)
	}
	return res
}
func GetLatestReport() map[string]*protocol.Report {
	connMu.RLock()
	defer connMu.RUnlock()
	reportCopy := make(map[string]*protocol.Report)
	for k, v := range latestReport {
		if v == nil {
			continue
		}
		item := *v
		reportCopy[k] = &item
	}
	return reportCopy
}

// RecordReport updates the latest runtime state and keeps only the short raw
// window used by recent-status compatibility endpoints.
func RecordReport(report protocol.Report) {
	if report.UUID == "" {
		return
	}
	if report.UpdatedAt.IsZero() {
		report.UpdatedAt = time.Now().UTC()
	} else {
		report.UpdatedAt = report.UpdatedAt.UTC()
	}
	connMu.Lock()
	defer connMu.Unlock()
	if latest := latestReport[report.UUID]; latest == nil || !report.UpdatedAt.Before(latest.UpdatedAt) {
		item := report
		latestReport[report.UUID] = &item
	}
	cutoff := time.Now().UTC().Add(-recentReportRetention)
	reports := reportsAfter(recentReports[report.UUID], cutoff)
	if report.UpdatedAt.Before(cutoff) {
		recentReports[report.UUID] = reports
		return
	}
	insertAt := sort.Search(len(reports), func(i int) bool {
		return reports[i].UpdatedAt.After(report.UpdatedAt)
	})
	reports = append(reports, protocol.Report{})
	copy(reports[insertAt+1:], reports[insertAt:])
	reports[insertAt] = report
	recentReports[report.UUID] = reports
}

func GetRecentReports(uuid string) []protocol.Report {
	connMu.Lock()
	defer connMu.Unlock()
	reports := reportsAfter(recentReports[uuid], time.Now().UTC().Add(-recentReportRetention))
	if len(reports) == 0 {
		delete(recentReports, uuid)
		return []protocol.Report{}
	}
	recentReports[uuid] = reports
	return append([]protocol.Report(nil), reports...)
}

func reportsAfter(reports []protocol.Report, cutoff time.Time) []protocol.Report {
	first := 0
	for first < len(reports) && reports[first].UpdatedAt.Before(cutoff) {
		first++
	}
	out := make([]protocol.Report, len(reports)-first)
	copy(out, reports[first:])
	return out
}

func DeleteLatestReport(uuid string) {
	connMu.Lock()
	defer connMu.Unlock()
	delete(latestReport, uuid)
	delete(recentReports, uuid)
}
