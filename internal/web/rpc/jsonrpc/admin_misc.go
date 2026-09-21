package jsonrpc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/komari-monitor/komari/internal/features/ping"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/internal/platform/metricstore"
	"github.com/komari-monitor/komari/internal/platform/records"
	"github.com/komari-monitor/komari/pkg/kv"
	"github.com/komari-monitor/komari/pkg/lifecycle"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// admin.misc.go
// 杂项 admin RPC2 方法：设置、客户端排序相关的记录清理。会话管理已迁移到
// internal/features/auth（见 admin_session.go）。

func parseUintKey(s string) (uint, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	return uint(v), err
}

func init() {
	RegisterWithGroupAndMeta("getSettings", rpc.RoleAdmin, adminGetSettings, &rpc.MethodMeta{
		Name:    "admin:getSettings",
		Summary: "Get all settings",
		Returns: "object",
	})
	RegisterWithGroupAndMeta("editSettings", rpc.RoleAdmin, adminEditSettings, &rpc.MethodMeta{
		Name:    "admin:editSettings",
		Summary: "Update settings (partial)",
		Returns: "null | { restart_required: true, guide_path: string }",
	})
	RegisterWithGroupAndMeta("clearAllRecords", rpc.RoleAdmin, adminClearAllRecords, &rpc.MethodMeta{
		Name:    "admin:clearAllRecords",
		Summary: "Delete all load and ping records",
		Returns: "null",
	})
}

func adminGetSettings(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	cst, err := kv.GetAll()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get settings: "+err.Error(), nil)
	}
	return cst, nil
}

// metricStoreConfigKeys 是与 metrics 独立数据库及 rollup 策略相关、需要
// 触发连接测试 + 热重载的配置键。
//
// 注意：metric_store_enabled 已废弃（metric store 始终启用），不再纳入此集合。
var metricStoreConfigKeys = map[string]struct{}{
	metricstore.MetricDBDriverKey:                         {},
	metricstore.MetricDBDSNKey:                            {},
	metricstore.MetricTablePrefixKey:                      {},
	metricstore.MetricMaxOpenConnsKey:                     {},
	metricstore.MetricMaxIdleConnsKey:                     {},
	metricstore.MetricRollupMinuteRetentionMinutesKey:     {},
	metricstore.MetricRollupFiveMinuteRetentionMinutesKey: {},
	metricstore.MetricRollupHourRetentionHoursKey:         {},
}

// metricKeysTouched 判断本次设置变更是否涉及 metrics 数据库相关键。
func metricKeysTouched(cfg map[string]any) bool {
	for key := range cfg {
		if _, ok := metricStoreConfigKeys[key]; ok {
			return true
		}
	}
	return false
}

func adminEditSettings(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	cfg := make(map[string]any)
	if err := req.BindParams(&cfg); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing request body: "+err.Error(), nil)
	}
	removeRetiredLowResourceMode(cfg)
	if err := validateMetricRollupSettingChanges(cfg); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}

	// 若本次修改涉及 metrics 数据库配置，则在落库前先用「当前配置 + 本次改动」
	// 合并出的目标配置做一次连接测试。metric store 始终启用，只要触及 metrics
	// 相关键就做连接测试，避免把明显无效的连接串保存给用户。
	touchedMetric := metricKeysTouched(cfg)
	if touchedMetric {
		// 数据库类型不再由前端显式选择，而是根据 DSN 自动推断后写回配置，
		// 使后续连接测试、热重载和初始化都使用一致的 driver。
		if v, ok := cfg[metricstore.MetricDBDSNKey]; ok {
			if dsn, ok := v.(string); ok {
				dsn = strings.TrimSpace(dsn)
				cfg[metricstore.MetricDBDSNKey] = dsn
				if driver, inferred := metricstore.InferDriverFromDSN(dsn); inferred {
					cfg[metricstore.MetricDBDriverKey] = string(driver)
				}
			}
		}

		merged, err := mergedMetricConfig(cfg)
		if err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to resolve metric store config: "+err.Error(), nil)
		}
		testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := metricstore.TestConnection(testCtx, merged); err != nil {
			cancel()
			return nil, rpc.MakeError(rpc.InvalidParams,
				"Metrics database connection test failed: "+err.Error(), nil)
		}
		cancel()
	}

	if err := kv.SetMany(cfg); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to update settings: "+err.Error(), nil)
	}

	// 配置已落库，热重载 metric store（无需重启）。旧结构必须在下一次
	// 启动前进入受限迁移页，不能在普通 HTTP 服务仍运行时切换路由。
	if touchedMetric {
		reloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := metricstore.Reload(reloadCtx); err != nil {
			cancel()
			if errors.Is(err, metricstore.ErrStructureUpgradeRequired) {
				auditSettingsUpdate(ctx, cfg)
				lifecycle.RequestRestart(lifecycle.RestartForMetricStoreStructureUpgrade)
				return map[string]any{
					"restart_required": true,
					"guide_path":       "/admin/database-migration",
				}, nil
			}
			return nil, rpc.MakeError(rpc.InternalError,
				"Settings saved but metrics database hot reload failed: "+err.Error(), nil)
		}
		cancel()
	}

	auditSettingsUpdate(ctx, cfg)
	return nil, nil
}

func auditSettingsUpdate(ctx context.Context, cfg map[string]any) {
	message := "update settings: "
	for key := range cfg {
		message += key + ", "
	}
	if len(message) > 2 {
		message = message[:len(message)-2]
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, message, "info")
}

// removeRetiredLowResourceMode keeps older admin clients from recreating its
// config row after the startup migration removes it.
func removeRetiredLowResourceMode(cfg map[string]any) {
	delete(cfg, "low_resource_mode")
}

// mergedMetricConfig 读取当前持久化的 metric store 配置，并把本次请求中涉及的
// metrics 相关键覆盖上去，得到「即将生效」的目标配置，用于落库前的连接测试。
func mergedMetricConfig(cfg map[string]any) (*metricstore.MetricStoreConfig, error) {
	merged, err := kv.GetManyAs[metricstore.MetricStoreConfig]()
	if err != nil {
		return nil, err
	}

	if v, ok := cfg[metricstore.MetricDBDriverKey]; ok {
		if s, ok := v.(string); ok {
			merged.Driver = s
		}
	}

	if v, ok := cfg[metricstore.MetricDBDSNKey]; ok {
		if s, ok := v.(string); ok {
			merged.DSN = s
		}
	}
	if v, ok := cfg[metricstore.MetricTablePrefixKey]; ok {
		if s, ok := v.(string); ok {
			merged.TablePrefix = s
		}
	}
	if v, ok := cfg[metricstore.MetricMaxOpenConnsKey]; ok {
		merged.MaxOpenConns = toInt(v, merged.MaxOpenConns)
	}
	if v, ok := cfg[metricstore.MetricMaxIdleConnsKey]; ok {
		merged.MaxIdleConns = toInt(v, merged.MaxIdleConns)
	}
	if v, ok := cfg[metricstore.MetricRollupMinuteRetentionMinutesKey]; ok {
		merged.RollupMinuteRetentionMinutes = toInt(v, merged.RollupMinuteRetentionMinutes)
	}
	if v, ok := cfg[metricstore.MetricRollupFiveMinuteRetentionMinutesKey]; ok {
		merged.RollupFiveMinuteRetentionMinutes = toInt(v, merged.RollupFiveMinuteRetentionMinutes)
	}
	if v, ok := cfg[metricstore.MetricRollupHourRetentionHoursKey]; ok {
		merged.RollupHourRetentionHours = toInt(v, merged.RollupHourRetentionHours)
	}

	return merged, nil
}

func validateMetricRollupSettingChanges(cfg map[string]any) error {
	keys := []string{
		metricstore.MetricRollupMinuteRetentionMinutesKey,
		metricstore.MetricRollupFiveMinuteRetentionMinutesKey,
		metricstore.MetricRollupHourRetentionHoursKey,
	}
	for _, key := range keys {
		value, ok := cfg[key]
		if !ok {
			continue
		}
		n, err := metricRollupSettingInt(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("%s must be a positive integer", key)
		}
	}
	return nil
}

func metricRollupSettingInt(value any) (int, error) {
	maxInt := float64(^uint(0) >> 1)
	switch value := value.(type) {
	case int:
		return value, nil
	case int8:
		return int(value), nil
	case int16:
		return int(value), nil
	case int32:
		return int(value), nil
	case int64:
		if float64(value) > maxInt || float64(value) < -maxInt-1 {
			return 0, fmt.Errorf("integer overflow")
		}
		return int(value), nil
	case uint:
		if float64(value) > maxInt {
			return 0, fmt.Errorf("integer overflow")
		}
		return int(value), nil
	case uint8:
		return int(value), nil
	case uint16:
		return int(value), nil
	case uint32:
		if float64(value) > maxInt {
			return 0, fmt.Errorf("integer overflow")
		}
		return int(value), nil
	case uint64:
		if float64(value) > maxInt {
			return 0, fmt.Errorf("integer overflow")
		}
		return int(value), nil
	case float32:
		return metricRollupSettingInt(float64(value))
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value > maxInt || value < -maxInt-1 {
			return 0, fmt.Errorf("not an integer")
		}
		return int(value), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(value))
	default:
		return 0, fmt.Errorf("unsupported numeric type")
	}
}

// toInt 将 JSON 解码得到的任意值（通常是 float64 或 string）转换为 int，失败时返回 fallback。
func toInt(v any, fallback int) int {

	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return fallback
}

func adminClearAllRecords(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {

	records.DeleteAll()
	ping.DeleteAllPingRecords()
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "clear all records", "info")
	return nil, nil
}
