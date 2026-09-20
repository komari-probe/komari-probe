package server

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/settings"
	"github.com/komari-monitor/komari/internal/version"
	"github.com/komari-monitor/komari/pkg/kv"
)

// Bootstrap initializes the data directory, primary database, and settings.
func (a *App) Bootstrap() error {
	if err := os.MkdirAll("./data/theme", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create theme directory: %w", err)
	}
	if err := os.MkdirAll("./data/plugin", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}
	if err := os.MkdirAll("./data/plugin-data", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create plugin storage directory: %w", err)
	}

	dbcore.SetVersionID(version.CurrentVersion + "-" + version.VersionHash)
	if err := dbcore.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	a.dbReady = true
	a.addCleanup("database", func(context.Context) error { return dbcore.Close() })

	gin.SetMode(gin.ReleaseMode)
	settings, err := kv.GetManyAs[settings.Settings]()
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}
	a.settings = settings
	return nil
}
