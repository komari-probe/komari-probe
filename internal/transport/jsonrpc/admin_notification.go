package jsonrpc

import (
	"github.com/sonar-probe/sonar/internal/features/notification"
)

// admin_notification.go
// 通知相关 RPC2 方法注册（admin 命名空间）：负载告警、离线通知。
// 处理逻辑在 internal/features/notification，这里只做注册。

func init() {
	// load notifications
	reg("addLoadNotification", notification.AdminAddLoadNotification, "Create a load notification")
	reg("deleteLoadNotification", notification.AdminDeleteLoadNotification, "Delete load notifications by ids")
	reg("editLoadNotification", notification.AdminEditLoadNotification, "Edit load notifications")
	reg("getAllLoadNotifications", notification.AdminGetAllLoadNotifications, "List all load notifications")
	// offline notifications
	reg("listOfflineNotifications", notification.AdminListOfflineNotifications, "List offline notifications")
	reg("editOfflineNotification", notification.AdminEditOfflineNotification, "Edit offline notifications")
	reg("enableOfflineNotification", notification.AdminEnableOfflineNotification, "Enable offline notifications for clients")
	reg("disableOfflineNotification", notification.AdminDisableOfflineNotification, "Disable offline notifications for clients")
	// send notification
	reg("sendNotification", notification.AdminSendNotification, "Send a notification")
	reg("testSendMessage", notification.AdminTestSendMessage, "Send a test notification")
}
