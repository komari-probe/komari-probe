package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/features/auth"
	"github.com/komari-monitor/komari/internal/features/auth/oauth"
	"github.com/komari-monitor/komari/internal/features/geoip"
	"github.com/komari-monitor/komari/internal/features/notification/messagesender"
	"github.com/komari-monitor/komari/internal/features/plugin"
	recoveryweb "github.com/komari-monitor/komari/internal/features/recovery"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/internal/platform/security"
	"github.com/komari-monitor/komari/internal/platform/settings"
	"github.com/komari-monitor/komari/internal/transport/router"
	"github.com/komari-monitor/komari/pkg/kv"
	"github.com/komari-monitor/komari/pkg/lifecycle"
	"github.com/komari-monitor/komari/pkg/logger"
)

// ErrRestartRequested is returned after a clean shutdown when a configuration
// change requires the next startup to enter a restricted guide.
var ErrRestartRequested = errors.New("server restart requested")

const (
	// Give in-flight HTTP requests time to finish before the listener closes.
	httpShutdownTimeout = 10 * time.Second
	// Keep an independent budget for report flushing and store teardown. Reusing
	// the HTTP deadline here can skip queued metric writes after a slow request.
	resourceCleanupTimeout = 30 * time.Second
)

func (a *App) registerReloadHandlers(cors *security.CorsController) {
	a.reload.Register("oauth-provider", func(event kv.ConfigEvent) {
		if ok, providerName := kv.IsChangedT[string](event, settings.OAuthProviderKey); ok {
			oauth.ReloadProviderByName(providerName)
		}
	})
	a.reload.Register("geoip-provider", func(event kv.ConfigEvent) {
		if event.IsChanged(settings.GeoIPProviderKey) {
			go geoip.InitGeoIP()
		}
	})
	a.reload.Register("message-sender", func(event kv.ConfigEvent) {
		if event.IsChanged(settings.NotificationMethodKey) {
			go messagesender.Initialize()
		}
	})
	a.reload.Register("cors", func(event kv.ConfigEvent) { cors.Update(event) })
}

// BuildRouter constructs the normal application router and starts reloads.
func (a *App) BuildRouter() error {
	r := gin.New()
	r.Use(logger.GinLogger(), logger.GinRecovery())
	cors := security.NewCorsController(a.settings.CorsOriginCheckEnabled, a.settings.CorsAllowedOrigins)
	r.Use(cors.Middleware(), auth.IdentityMiddleware(), auth.PrivateSiteMiddleware(), noStoreAPIResponses())

	// The recovery UI belongs only to its temporary restricted listener.
	r.GET(recoveryweb.PagePath, func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/")
	})
	router.Register(r)

	// Plugins are loaded after the router exists so server.route can bind
	// routes; a failed plugin only disables itself and is logged.
	plugin.Init(r)
	if err := plugin.LoadAll(); err != nil {
		logger.ErrorArgs("server", "Failed to load some plugins:", err)
	}
	a.addCleanup("plugins", func(context.Context) error { return plugin.CloseAll() })

	a.registerReloadHandlers(cors)
	a.reload.Start()
	a.engine = r
	return nil
}

// Run starts the normal HTTP server and blocks until shutdown or fatal error.
func (a *App) Run() error {
	// The HTML injector runs outside the hook chain so it sees the final
	// response: plugin hooks can still rewrite the body, then the registered
	// head/body fragments are embedded into every text/html page.
	a.server = &http.Server{Addr: a.listenAddr, Handler: plugin.HTMLInjectHandler(plugin.WrapHandler(a.engine))}
	logger.Infof("server", "Starting server on %s ...", a.listenAddr)
	serverErr := serveInBackground(a.server)

	quit, stopWatchingQuit := watchQuitSignal()
	defer stopWatchingQuit()
	select {
	case err := <-serverErr:
		a.onFatal(err)
		return fmt.Errorf("listen: %w", err)
	case reason := <-lifecycle.RestartRequests():
		logger.Infof("server", "Restarting service for %s", reason)
		if err := a.Shutdown(); err != nil {
			logger.Errorf("server", "Cleanup before restart failed: %v", err)
		}
		return fmt.Errorf("%w: %s", ErrRestartRequested, reason)
	case <-quit:
		return a.Shutdown()
	}
}

// Shutdown stops HTTP first, then releases registered resources in LIFO order.
func (a *App) Shutdown() error {
	if a.dbReady {
		auditlog.Log("", "", "server is shutting down", "info")
	}
	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancelHTTP()
	if a.server != nil {
		if err := a.server.Shutdown(httpCtx); err != nil {
			logger.Infof("server", "HTTP server forced to shutdown: %v", err)
		}
	}

	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), resourceCleanupTimeout)
	defer cancelCleanup()
	return a.runCleanups(cleanupCtx)
}

func (a *App) onFatal(err error) {
	if a.dbReady {
		auditlog.Log("", "", "server encountered a fatal error: "+err.Error(), "error")
	}
	ctx, cancel := context.WithTimeout(context.Background(), resourceCleanupTimeout)
	defer cancel()
	if cleanupErr := a.runCleanups(ctx); cleanupErr != nil {
		logger.Errorf("server", "Cleanup after fatal server error failed: %v", cleanupErr)
	}
}

func (a *App) runCleanups(ctx context.Context) error {
	var cleanupErrors []error
	for _, cleanup := range slices.Backward(a.cleanups) {

		if err := cleanup.fn(ctx); err != nil {
			logger.Errorf("server", "cleanup %q failed: %v", cleanup.name, err)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("cleanup %q: %w", cleanup.name, err))
		}
	}
	return errors.Join(cleanupErrors...)
}
