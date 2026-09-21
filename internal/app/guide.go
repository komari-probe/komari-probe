package app

// This file hosts the whole "guide" subsystem: install / metric-store
// recovery / database migration all present themselves as a temporary,
// narrowly scoped web server (a "guide") instead of the normal application
// router. detection (what guide is required, if any) and hosting (how a
// guide is served and torn down) are kept together because neither half is
// meaningful without the other.

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/features/auth"
	installweb "github.com/komari-monitor/komari/internal/features/install"
	migrationweb "github.com/komari-monitor/komari/internal/features/migration"
	recoveryweb "github.com/komari-monitor/komari/internal/features/recovery"
	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/frontend"
	"github.com/komari-monitor/komari/internal/platform/metricruntime"
	"github.com/komari-monitor/komari/internal/platform/migrations"
	"github.com/komari-monitor/komari/internal/platform/origincheck"
	"github.com/komari-monitor/komari/internal/platform/respond"
	"github.com/komari-monitor/komari/pkg/logger"
)

// InstallRequired reports whether the instance still needs the first-run guide.
func (a *App) InstallRequired() (bool, error) {
	return installweb.IsRequired(dbcore.GetDBInstance())
}

type DatabaseMigrationRequirement struct {
	mode    migrationweb.Mode
	summary migrations.LegacyMonitoringSummary
}

func (r DatabaseMigrationRequirement) Required() bool { return r.mode != "" }

// DatabaseMigrationRequired checks both supported migration inputs before the
// normal Metric Store connection is initialized. Structure upgrades run first
// when both inputs exist, then the startup loop detects the legacy tables on
// its next pass.
func (a *App) DatabaseMigrationRequired() (DatabaseMigrationRequirement, error) {
	structureRequired, err := metricruntime.StructureUpgradeRequired(context.Background())
	if err != nil {
		return DatabaseMigrationRequirement{}, err
	}
	if structureRequired {
		return DatabaseMigrationRequirement{mode: migrationweb.ModeMetricStructure}, nil
	}
	legacyRequired, summary, err := migrations.LegacyMonitoringMigrationRequired(dbcore.GetDBInstance())
	if err != nil {
		return DatabaseMigrationRequirement{}, err
	}
	if legacyRequired {
		return DatabaseMigrationRequirement{mode: migrationweb.ModeLegacyMonitoring, summary: summary}, nil
	}
	return DatabaseMigrationRequirement{}, nil
}

// RunInstallGuide exposes only first-run installation APIs. It intentionally
// does not mount authentication or normal application routes.
func (a *App) RunInstallGuide() (bool, error) {
	return a.runGuideServer(installweb.NewController(dbcore.GetDBInstance()), guideServerConfig{
		pagePath:   installweb.PagePath,
		missingAPI: "Not found in install mode",
		logMessage: "First-run installation guide is available on %s",
	})
}

// RunMetricStoreRecovery keeps login available while exposing only the
// administrator-protected metric-store recovery API.
func (a *App) RunMetricStoreRecovery(initialErr error) (bool, error) {
	a.initOAuth()
	return a.runGuideServer(recoveryweb.NewController(initialErr, metricStoreReconnectAttempts), guideServerConfig{
		pagePath:         recoveryweb.PagePath,
		missingAPI:       "Not found in database recovery mode",
		logMessage:       "Metric store recovery is available on %s",
		requireIdentity:  true,
		restrictedStatic: true,
	})
}

// RunDatabaseMigration serves the same authenticated guide and status model
// for either migration input while leaving each conversion engine independent.
func (a *App) RunDatabaseMigration(requirement DatabaseMigrationRequirement) (bool, error) {
	a.initOAuth()
	var controller guideController
	switch requirement.mode {
	case migrationweb.ModeMetricStructure:
		controller = migrationweb.NewStructureController()
	case migrationweb.ModeLegacyMonitoring:
		controller = migrationweb.NewLegacyController(dbcore.GetDBInstance(), requirement.summary)
	default:
		return false, nil
	}
	return a.runGuideServer(controller, guideServerConfig{
		pagePath:         migrationweb.PagePath,
		missingAPI:       "Not found in database migration mode",
		logMessage:       "Database migration guide is available on %s",
		requireIdentity:  true,
		restrictedStatic: true,
	})
}

type guideController interface {
	Activate()
	Deactivate()
	Register(*gin.Engine)
	Done() <-chan struct{}
}

type guideServerConfig struct {
	pagePath         string
	missingAPI       string
	logMessage       string
	requireIdentity  bool
	restrictedStatic bool
}

// runGuideServer hosts one temporary, narrowly scoped guide. It deliberately
// constructs a new Gin engine for each mode so normal routes never leak into
// installation, upgrade, or recovery flows.
func (a *App) runGuideServer(controller guideController, cfg guideServerConfig) (bool, error) {
	controller.Activate()
	defer controller.Deactivate()

	r := gin.New()
	r.Use(logger.GinLogger(), logger.GinRecovery(), noStoreAPIResponses())
	if cfg.requireIdentity {
		cors := origincheck.NewCorsController(a.settings.CorsOriginCheckEnabled, a.settings.CorsAllowedOrigins)
		r.Use(cors.Middleware(), auth.IdentityMiddleware())
	}
	controller.Register(r)

	serveStatic := frontend.Static
	if cfg.restrictedStatic {
		serveStatic = frontend.StaticRestricted
	}
	serveStatic(r.Group("/"), func(handlers ...gin.HandlerFunc) {
		r.NoRoute(guideNoRoute(cfg.pagePath, cfg.missingAPI, handlers))
	})

	server := &http.Server{Addr: a.listenAddr, Handler: r}
	a.engine = r
	a.server = server
	logger.Infof("server", cfg.logMessage, a.listenAddr)
	serverErr := serveInBackground(server)

	quit, stopWatchingQuit := watchQuitSignal()
	defer stopWatchingQuit()

	select {
	case err := <-serverErr:
		return false, fmt.Errorf("listen in guide mode: %w", err)
	case <-quit:
		return false, a.Shutdown()
	case <-controller.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return false, fmt.Errorf("stop guide server: %w", err)
		}
		a.server = nil
		a.engine = nil
		return true, nil
	}
}

func noStoreAPIResponses() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	}
}

func guideNoRoute(pagePath, missingAPI string, handlers []gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		if strings.HasPrefix(requestPath, "/api") {
			respond.Error(c, http.StatusNotFound, missingAPI)
			return
		}
		if c.Request.Method == http.MethodGet && requestPath != pagePath && filepath.Ext(requestPath) == "" {
			c.Redirect(http.StatusTemporaryRedirect, pagePath)
			return
		}
		for _, handler := range handlers {
			handler(c)
			if c.IsAborted() {
				return
			}
		}
	}
}
