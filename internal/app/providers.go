package app

import (
	"context"
	"fmt"

	"github.com/komari-monitor/komari/internal/features/auth/oauth"
	"github.com/komari-monitor/komari/internal/features/notification/messagesender"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/internal/platform/geoipruntime"
	"github.com/komari-monitor/komari/pkg/logger"
)

// InitProviders initializes providers needed by the normal application.
func (a *App) InitProviders() error {
	a.initOAuth()

	go geoipruntime.InitGeoIP()
	a.addCleanup("geoip", func(context.Context) error { return geoipruntime.Shutdown() })

	messagesender.Initialize()
	a.addCleanup("message-sender", func(context.Context) error { return messagesender.Shutdown() })
	return nil
}

// initOAuth initializes OAuth once. Restricted authenticated guides need it
// before the normal provider initialization phase.
func (a *App) initOAuth() {
	if a.oauthReady {
		return
	}
	if err := oauth.Initialize(); err != nil {
		logger.Errorf("server", "Failed to initialize OAuth provider: %v", err)
		auditlog.EventLog("error", fmt.Sprintf("Failed to initialize OAuth provider: %v", err))
	}
	a.oauthReady = true
	a.addCleanup("oauth", func(context.Context) error { return oauth.Shutdown() })
}
