package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/komari-monitor/komari/internal/features/notification/messagesender"
	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/models"
	"github.com/komari-monitor/komari/pkg/rpc"
	"gorm.io/gorm/clause"
)

// handler_admin.go
// 通知相关 RPC2 方法的处理逻辑（admin 命名空间）：负载告警、离线通知。
// 方法注册留在 web/rpc/jsonrpc（注册中心在那边），这里只暴露纯逻辑函数。

// AdminSendNotification 发送一条通知。仅供外部（插件/脚本）通过 RPC 调用，
// 内部通知逻辑（offline/renewal/session 等）直接调用 SendNotification。
func AdminSendNotification(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Event models.EventMessage `json:"event"`
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data: "+err.Error(), nil)
	}
	if fmt.Sprint(params.Event.Event) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "event is required", nil)
	}
	if err := messagesender.SendNotification(params.Event); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to send notification: "+err.Error(), nil)
	}
	return nil, nil
}

// AdminTestSendMessage 发送一条测试通知，供后台"测试消息发送渠道"按钮调用。
func AdminTestSendMessage(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if err := messagesender.SendNotification(models.EventMessage{
		Event:   "Test",
		Time:    time.Now().UTC(),
		Message: "This is a test message from Komari.",
	}); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to send message: "+err.Error(), nil)
	}
	return nil, nil
}

func AdminAddLoadNotification(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Clients   []string `json:"clients"`
		Name      string   `json:"name"`
		Metric    string   `json:"metric"`
		Threshold float32  `json:"threshold"`
		Ratio     float32  `json:"ratio"`
		Interval  int      `json:"interval"`
	}
	req.BindParams(&params)
	if len(params.Clients) == 0 || params.Metric == "" || params.Threshold == 0 || params.Ratio == 0 || params.Interval == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "clients, metric, threshold, ratio and interval are required", nil)
	}
	if params.Interval > 4*60 || params.Interval <= 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "Interval must be between 1 and 240 minutes", nil)
	}
	if params.Ratio <= 0 || params.Ratio > 1 {
		return nil, rpc.MakeError(rpc.InvalidParams, "Ratio must be between 0 and 1", nil)
	}
	taskID, err := AddLoadNotification(params.Clients, params.Name, params.Metric, params.Threshold, params.Ratio, params.Interval)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return map[string]any{"task_id": taskID}, nil
}

func AdminDeleteLoadNotification(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		ID []uint `json:"id"`
	}
	req.BindParams(&params)
	if len(params.ID) == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "id is required", nil)
	}
	if err := DeleteLoadNotification(params.ID); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}

func AdminEditLoadNotification(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Notifications []*models.LoadNotification `json:"notifications"`
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data", nil)
	}
	if err := EditLoadNotification(params.Notifications); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}

func AdminGetAllLoadNotifications(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	list, err := GetAllLoadNotifications()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return list, nil
}

func AdminListOfflineNotifications(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var notifications []models.OfflineNotification
	if err := dbcore.GetDBInstance().Model(&models.OfflineNotification{}).Find(&notifications).Error; err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to list offline notifications: "+err.Error(), nil)
	}
	return notifications, nil
}

func AdminEditOfflineNotification(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var notifications []models.OfflineNotification
	if err := req.BindParams(&notifications); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request body: "+err.Error(), nil)
	}
	if len(notifications) == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "At least one notification is required", nil)
	}
	for _, noti := range notifications {
		if noti.Client == "" {
			return nil, rpc.MakeError(rpc.InvalidParams, "Client UUID cannot be empty", nil)
		}
		if noti.GracePeriod <= 0 {
			return nil, rpc.MakeError(rpc.InvalidParams, "GracePeriod must be a positive integer", nil)
		}
	}
	err := dbcore.GetDBInstance().Model(&models.OfflineNotification{}).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "client"}},
			DoUpdates: clause.AssignmentColumns([]string{"enable", "grace_period"}),
		}).
		Select("*").Create(notifications).Error
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to edit offline notifications: "+err.Error(), nil)
	}
	return nil, nil
}

// setOfflineNotificationEnable 是 enable/disable 的共享实现。
func setOfflineNotificationEnable(req *rpc.JsonRpcRequest, enable bool) *rpc.JsonRpcError {
	var uuids []string
	if err := req.BindParams(&uuids); err != nil {
		return rpc.MakeError(rpc.InvalidParams, "Invalid request body: "+err.Error(), nil)
	}
	notifications := make([]models.OfflineNotification, 0, len(uuids))
	for _, uuid := range uuids {
		notifications = append(notifications, models.OfflineNotification{Client: uuid, Enable: enable})
	}
	err := dbcore.GetDBInstance().Model(&models.OfflineNotification{}).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "client"}},
			DoUpdates: clause.AssignmentColumns([]string{"enable"}),
		}).
		Select("client", "enable").Create(notifications).Error
	if err != nil {
		return rpc.MakeError(rpc.InternalError, "Failed to update offline notifications: "+err.Error(), nil)
	}
	return nil
}

func AdminEnableOfflineNotification(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if e := setOfflineNotificationEnable(req, true); e != nil {
		return nil, e
	}
	return nil, nil
}

func AdminDisableOfflineNotification(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if e := setOfflineNotificationEnable(req, false); e != nil {
		return nil, e
	}
	return nil, nil
}
