package app

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/platform/dbcore"
	"github.com/sonar-probe/sonar/internal/platform/settings"
	"github.com/sonar-probe/sonar/internal/version"
	"github.com/sonar-probe/sonar/pkg/kv"
)

// Bootstrap initializes the primary database and settings. Feature-owned data
// directories (theme, plugin, ...) are created lazily by their own packages
// on first use, not pre-created here.
func (a *App) Bootstrap() error {
	dbcore.SetVersionID(version.CurrentVersion + "-" + version.VersionHash)
	if err := dbcore.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	a.dbReady = true
	a.addCleanup("database", func(context.Context) error { return dbcore.Close() })

	gin.SetMode(gin.ReleaseMode)
	loadedSettings, err := kv.GetManyAs[settings.Settings]()
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}
	a.settings = loadedSettings
	return nil
}
