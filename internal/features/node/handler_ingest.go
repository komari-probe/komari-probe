package node

import (
	"context"
	"fmt"
	"time"

	"github.com/komari-monitor/komari/internal/platform/clients"
	"github.com/komari-monitor/komari/internal/platform/metricruntime"
	"github.com/komari-monitor/komari/internal/platform/models"
	"github.com/komari-monitor/komari/internal/platform/protocol"
)

// ingest.go
// agent 上报数据的传输无关处理逻辑。v2 的 HTTP/WebSocket JSON-RPC 入口
// 经过协议解析后，统一调用这里的函数落库并更新运行时状态。

// ingestReport 保存一次负载上报并刷新运行时状态。
// markPresence 为 true 时按 POST 上报会话刷新在线状态（WS 连接自行管理在线状态，应传 false）。
func ingestReport(uuid string, report protocol.Report, markPresence bool) error {
	report.UUID = uuid
	report.UpdatedAt = time.Now().UTC()
	if err := clients.ReportVerify(report); err != nil {
		return err
	}
	savedReport, err := metricruntime.WriteReport(context.Background(), report)
	if err != nil {
		return err
	}
	RecordReport(savedReport)
	MarkV2Client(uuid)
	if markPresence {
		refreshPostPresence(uuid)
	}
	return nil
}

// ingestBasicInfo 保存客户端基础信息。fallbackIP 在上报未携带 IP 时用作兜底。
func ingestBasicInfo(uuid string, info map[string]any, fallbackIP string) error {
	if info == nil {
		info = map[string]any{}
	}
	return saveClientBasicInfo(info, uuid, fallbackIP)
}

// ingestPingResult 保存一条 ping 探测结果。
func ingestPingResult(uuid string, taskID uint, value int) error {
	if pingResultRecorder == nil {
		return fmt.Errorf("ping result recorder is not registered")
	}
	return pingResultRecorder(models.PingRecord{
		Client: uuid,
		TaskID: taskID,
		Value:  value,
		Time:   time.Now().UTC(),
	})
}
