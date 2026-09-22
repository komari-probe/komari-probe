package migration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/platform/dbcore"
	"github.com/sonar-probe/sonar/internal/platform/metricruntime"
	"github.com/sonar-probe/sonar/internal/platform/migrations"
	"github.com/sonar-probe/sonar/internal/platform/respond"
	"github.com/sonar-probe/sonar/pkg/kv"
	"github.com/sonar-probe/sonar/pkg/tsdb"
)

const largeDatasetThreshold int64 = 300_000

type startRequest struct {
	Driver              string `json:"driver"`
	DSN                 string `json:"dsn"`
	ConfirmSQLiteRisk   bool   `json:"confirm_sqlite_risk"`
	ConfirmLargeDataset bool   `json:"confirm_large_dataset"`
}

func (c *Controller) startLegacy(ctx *gin.Context) {
	var request startRequest
	if err := decodeJSON(ctx, &request); err != nil {
		respond.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := metricConfig(request.Driver, request.DSN)
	if err != nil {
		respond.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := migrations.InspectLegacyMonitoring(c.db)
	if err != nil {
		respond.Error(ctx, http.StatusInternalServerError, "failed to inspect legacy monitoring data")
		return
	}
	driver := metricruntime.ResolveDriverFromConfig(cfg.Driver, cfg.DSN)
	if driver == tsdb.DriverSQLite && summary.ServerCount > 5 && summary.RetentionDays > 7 && !request.ConfirmSQLiteRisk {
		respond.Error(ctx, http.StatusConflict, "SQLite risk confirmation is required")
		return
	}
	if summary.LoadRows+summary.LatencyRows > largeDatasetThreshold && !request.ConfirmLargeDataset {
		respond.Error(ctx, http.StatusConflict, "large dataset confirmation is required")
		return
	}

	c.mu.Lock()
	if c.operationActiveLocked() || c.status.State == "completed" {
		c.mu.Unlock()
		respond.Error(ctx, http.StatusConflict, "database migration is already running or completed")
		return
	}
	c.status = Status{
		Mode:            c.mode,
		State:           "migrating",
		Phase:           "connecting",
		Summary:         &summary,
		SourceRowsTotal: summary.MonitoringRows,
		TargetDriver:    string(driver),
	}
	c.mu.Unlock()

	go c.runLegacy(*cfg, summary.RetentionDays)
	respond.SuccessMessage(ctx, "database migration started", gin.H{})
}

func (c *Controller) runLegacy(cfg metricruntime.MetricStoreConfig, legacyRetentionDays int) {
	ctx := context.Background()
	mainBefore, err := dbcore.StorageSize()
	if err != nil {
		c.failTarget(err, cfg.DSN, "measuring")
		return
	}
	store, err := metricruntime.OpenStoreForMigration(ctx, &cfg, legacyRetentionDays)
	if err != nil {
		c.failTarget(err, cfg.DSN, "connecting")
		return
	}
	defer store.Close()
	targetBefore, err := store.StorageSize(ctx)
	if err != nil {
		c.failTarget(err, cfg.DSN, "measuring")
		return
	}

	if err := kv.SetMany(map[string]any{
		metricruntime.MetricDBDriverKey: cfg.Driver,
		metricruntime.MetricDBDSNKey:    cfg.DSN,
	}); err != nil {
		c.failTarget(err, cfg.DSN, "saving_target")
		return
	}

	_, err = migrations.MigrateLegacyMonitoring(ctx, c.db, store, func(progress migrations.LegacyMonitoringProgress) {
		c.mu.Lock()
		c.status.Phase = progress.Phase
		c.status.Table = progress.Table
		c.status.SourceRowsDone = progress.SourceRowsDone
		c.status.SourceRowsTotal = progress.SourceRowsTotal
		c.status.WrittenPoints = progress.WrittenPoints
		if progress.SourceRowsTotal > 0 {
			c.status.Progress = float64(progress.SourceRowsDone) / float64(progress.SourceRowsTotal) * 100
		}
		c.mu.Unlock()
	})
	if err != nil {
		c.failTarget(err, cfg.DSN, "migrating")
		return
	}
	if err := store.RebuildCoarserRollups(ctx, time.Hour); err != nil {
		c.failTarget(err, cfg.DSN, "migrating")
		return
	}

	c.mu.Lock()
	c.status.Phase = "vacuuming"
	c.status.Progress = 100
	c.mu.Unlock()
	if _, err := store.Compact(ctx, time.Now().UTC()); err != nil {
		c.failTarget(err, cfg.DSN, "vacuuming")
		return
	}
	if err := store.ReclaimSpace(ctx); err != nil {
		c.failTarget(err, cfg.DSN, "vacuuming")
		return
	}

	c.mu.Lock()
	c.status.Phase = "finalizing"
	c.mu.Unlock()
	finalizePhase := "finalizing"
	if err := migrations.CompleteLegacyMonitoringMigration(c.db, func() error {
		finalizePhase = "vacuuming"
		c.mu.Lock()
		c.status.Phase = finalizePhase
		c.mu.Unlock()
		return dbcore.ReclaimSpace(ctx)
	}); err != nil {
		c.failTarget(err, cfg.DSN, finalizePhase)
		return
	}
	mainAfter, err := dbcore.StorageSize()
	if err != nil {
		c.failTarget(err, cfg.DSN, "measuring")
		return
	}
	targetAfter, err := store.StorageSize(ctx)
	if err != nil {
		c.failTarget(err, cfg.DSN, "measuring")
		return
	}
	beforeBytes := mainBefore + targetBefore
	afterBytes := mainAfter + targetAfter
	savedBytes := beforeBytes - afterBytes
	if savedBytes < 0 {
		savedBytes = 0
	}
	savedPercent := 0.0
	if beforeBytes > 0 {
		savedPercent = float64(savedBytes) / float64(beforeBytes) * 100
	}

	c.mu.Lock()
	c.status.State = "completed"
	c.status.Phase = "completed"
	c.status.Table = ""
	c.status.Progress = 100
	c.status.BeforeBytes = beforeBytes
	c.status.AfterBytes = afterBytes
	c.status.SavedBytes = savedBytes
	c.status.SavedPercent = savedPercent
	c.status.Error = ""
	c.mu.Unlock()
	c.completeLater()
}

func metricConfig(requestedDriver, requestedDSN string) (*metricruntime.MetricStoreConfig, error) {
	requestedDriver = strings.ToLower(strings.TrimSpace(requestedDriver))
	requestedDSN = strings.TrimSpace(requestedDSN)
	if requestedDriver != string(tsdb.DriverSQLite) && requestedDriver != string(tsdb.DriverMySQL) && requestedDriver != string(tsdb.DriverPostgreSQL) {
		return nil, fmt.Errorf("driver must be sqlite, mysql, or postgresql")
	}
	if requestedDSN == "" {
		if requestedDriver != string(tsdb.DriverSQLite) {
			return nil, fmt.Errorf("dsn is required for remote databases")
		}
		requestedDSN = "./data/metrics.db"
	}
	resolved := metricruntime.ResolveDriverFromConfig(requestedDriver, requestedDSN)
	if string(resolved) != requestedDriver {
		return nil, fmt.Errorf("dsn does not match the selected database type")
	}
	cfg, err := kv.GetManyAs[metricruntime.MetricStoreConfig]()
	if err != nil {
		return nil, fmt.Errorf("load metric store defaults: %w", err)
	}
	cfg.Driver = requestedDriver
	cfg.DSN = requestedDSN
	return cfg, nil
}
