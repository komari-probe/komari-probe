package jsonrpc

import (
	"github.com/sonar-probe/sonar/internal/features/auth"
	"github.com/sonar-probe/sonar/internal/features/notification"
)

// admin_provider.go
// 消息发送器 provider 配置的方法注册委托给 internal/features/notification；
// OIDC provider 配置的方法注册委托给 internal/features/auth。

func init() {
	reg("getMessageSenderProvider", notification.AdminGetMessageSender, "Get message sender provider config or templates")
	reg("setMessageSenderProvider", notification.AdminSetMessageSender, "Set message sender provider config")
	reg("getOidcProvider", auth.AdminGetOidc, "Get OIDC provider config or templates")
	reg("setOidcProvider", auth.AdminSetOidc, "Set OIDC provider config")
}
