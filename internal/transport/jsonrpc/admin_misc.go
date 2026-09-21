package jsonrpc

import (
	"context"
	"errors"
	"time"

	"github.com/komari-monitor/komari/internal/features/ping"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/internal/platform/metricruntime"
	"github.com/komari-monitor/komari/internal/platform/recordquery"
	"github.com/komari-monitor/komari/pkg/kv"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// admin_misc.go
// 杂项 admin RPC2 方法：设置、客户端排序相关的记录清理。会话管理已迁移到
// internal/features/auth（见 admin_session.go）。

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

func adminEditSettings(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	cfg := make(map[string]any)
	if err := req.BindParams(&cfg); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing request body: "+err.Error(), nil)
	}
	removeRetiredLowResourceMode(cfg)
	if err := metricruntime.ValidateRollupSettingChanges(cfg); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}

	// 若本次修改涉及 metrics 数据库配置，则在落库前先用「当前配置 + 本次改动」
	// 合并出的目标配置做一次连接测试。metric store 始终启用，只要触及 metrics
	// 相关键就做连接测试，避免把明显无效的连接串保存给用户。
	if metricruntime.ConfigKeysTouched(cfg) {
		merged, err := metricruntime.NormalizeAndMergeConfig(cfg)
		if err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to resolve metric store config: "+err.Error(), nil)
		}
		testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = metricruntime.TestConnection(testCtx, merged)
		cancel()
		if err != nil {
			return nil, rpc.MakeError(rpc.InvalidParams,
				"Metrics database connection test failed: "+err.Error(), nil)
		}
	}

	if err := kv.SetMany(cfg); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to update settings: "+err.Error(), nil)
	}

	// 配置已落库，热重载 metric store（无需重启）。旧结构必须在下一次
	// 启动前进入受限迁移页，不能在普通 HTTP 服务仍运行时切换路由。
	outcome, err := metricruntime.ReloadAfterConfigChange(ctx, cfg)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError,
			"Settings saved but metrics database hot reload failed: "+err.Error(), nil)
	}
	auditSettingsUpdate(ctx, cfg)
	if outcome.RestartRequired {
		return map[string]any{
			"restart_required": true,
			"guide_path":       outcome.GuidePath,
		}, nil
	}
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

func adminClearAllRecords(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	loadErr := recordquery.DeleteAll()
	pingErr := ping.DeleteAllPingRecords()
	if err := errors.Join(loadErr, pingErr); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to clear all records: "+err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "clear all records", "info")
	return nil, nil
}
