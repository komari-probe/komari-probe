package messagesender

import (
	_ "github.com/komari-monitor/komari/internal/features/notification/messagesender/bark"
	_ "github.com/komari-monitor/komari/internal/features/notification/messagesender/email"
	_ "github.com/komari-monitor/komari/internal/features/notification/messagesender/empty"
	_ "github.com/komari-monitor/komari/internal/features/notification/messagesender/javascript"
	_ "github.com/komari-monitor/komari/internal/features/notification/messagesender/serverchan3"
	_ "github.com/komari-monitor/komari/internal/features/notification/messagesender/serverchanturbo"
	_ "github.com/komari-monitor/komari/internal/features/notification/messagesender/telegram"
	_ "github.com/komari-monitor/komari/internal/features/notification/messagesender/webhook"
)

func All() {
}
