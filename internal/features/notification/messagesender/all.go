package messagesender

import (
	_ "github.com/sonar-probe/sonar/internal/features/notification/messagesender/bark"
	_ "github.com/sonar-probe/sonar/internal/features/notification/messagesender/email"
	_ "github.com/sonar-probe/sonar/internal/features/notification/messagesender/empty"
	_ "github.com/sonar-probe/sonar/internal/features/notification/messagesender/javascript"
	_ "github.com/sonar-probe/sonar/internal/features/notification/messagesender/serverchan3"
	_ "github.com/sonar-probe/sonar/internal/features/notification/messagesender/serverchanturbo"
	_ "github.com/sonar-probe/sonar/internal/features/notification/messagesender/telegram"
	_ "github.com/sonar-probe/sonar/internal/features/notification/messagesender/webhook"
)

func All() {
}
