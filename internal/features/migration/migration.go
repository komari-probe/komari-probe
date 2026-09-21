// Package migration serves the authenticated database migration guide used by
// both legacy monitoring imports and Metric Store structure upgrades.
package migration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/features/auth"
	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/metricruntime"
	"github.com/komari-monitor/komari/internal/platform/migrations"
	"github.com/komari-monitor/komari/internal/platform/respond"
	"github.com/komari-monitor/komari/internal/platform/settings"
	jsonrpc "github.com/komari-monitor/komari/internal/transport/jsonrpc"
	"github.com/komari-monitor/komari/pkg/kv"
	"github.com/komari-monitor/komari/pkg/tsdb"
	"gorm.io/gorm"
)

const (
	PagePath = "/admin/database-migration"
	APIPath  = "/api/admin/database-migration"
)

const largeDatasetThreshold int64 = 300_000
const structureProgressCeiling = 80.0

type Mode string

const (
	ModeLegacyMonitoring Mode = "legacy_monitoring"
	ModeMetricStructure  Mode = "metric_store_restructure"
)

// Status is the shared polling payload for both migration modes. Mode-specific
// fields are omitted when they do not apply.
type Status struct {
	Mode     Mode    `json:"mode"`
	State    string  `json:"state"`
	Phase    string  `json:"phase"`
	Progress float64 `json:"progress"`
	Error    string  `json:"error,omitempty"`

	Summary         *migrations.LegacyMonitoringSummary `json:"summary,omitempty"`
	Table           string                              `json:"table,omitempty"`
	SourceRowsDone  int64                               `json:"source_rows_done,omitempty"`
	SourceRowsTotal int64                               `json:"source_rows_total,omitempty"`
	WrittenPoints   int64                               `json:"written_points,omitempty"`
	TargetDriver    string                              `json:"target_driver,omitempty"`

	CurrentMetric string  `json:"current_metric,omitempty"`
	RowsDone      int64   `json:"rows_done,omitempty"`
	RowsTotal     int64   `json:"rows_total,omitempty"`
	MetricsDone   int     `json:"metrics_done,omitempty"`
	MetricsTotal  int     `json:"metrics_total,omitempty"`
	BeforeBytes   int64   `json:"before_bytes,omitempty"`
	AfterBytes    int64   `json:"after_bytes,omitempty"`
	SavedBytes    int64   `json:"saved_bytes,omitempty"`
	SavedPercent  float64 `json:"saved_percent,omitempty"`
}

type Controller struct {
	mode Mode
	db   *gorm.DB

	active atomic.Bool
	mu     sync.RWMutex
	status Status
	done   chan struct{}
	once   sync.Once
}

type startRequest struct {
	Driver              string `json:"driver"`
	DSN                 string `json:"dsn"`
	ConfirmSQLiteRisk   bool   `json:"confirm_sqlite_risk"`
	ConfirmLargeDataset bool   `json:"confirm_large_dataset"`
}

func NewLegacyController(db *gorm.DB, summary migrations.LegacyMonitoringSummary) *Controller {
	return &Controller{
		mode: ModeLegacyMonitoring,
		db:   db,
		status: Status{
			Mode:            ModeLegacyMonitoring,
			State:           "idle",
			Phase:           "ready",
			Summary:         &summary,
			SourceRowsTotal: summary.MonitoringRows,
		},
		done: make(chan struct{}),
	}
}

func NewStructureController() *Controller {
	return &Controller{
		mode:   ModeMetricStructure,
		status: Status{Mode: ModeMetricStructure, State: "ready", Phase: "ready"},
		done:   make(chan struct{}),
	}
}

func (c *Controller) Activate() { c.active.Store(true) }

func (c *Controller) Deactivate() { c.active.Store(false) }

func (c *Controller) Done() <-chan struct{} { return c.done }

func (c *Controller) Register(r *gin.Engine) {
	r.POST("/api/login", auth.Login)
	r.GET("/api/me", jsonrpc.Bind("public:getMe", jsonrpc.WithRaw()))
	r.GET("/api/oauth", auth.OAuth)
	r.GET("/api/oauth_callback", auth.OAuthCallback)

	g := r.Group(APIPath, c.requireActive)
	g.GET("/auth", c.authStatus)
	authorized := g.Group("", auth.RequireRole(auth.RoleAdmin))
	authorized.GET("/status", c.getStatus)
	authorized.POST("/start", c.start)
	if c.mode == ModeMetricStructure {
		authorized.POST("/discard", c.discard)
	}
}

func (c *Controller) requireActive(ctx *gin.Context) {
	if !c.active.Load() {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	ctx.Next()
}

func (c *Controller) authStatus(ctx *gin.Context) {
	oauthEnabled, _ := kv.GetAs[bool](settings.OAuthEnabledKey, false)
	oauthProvider, _ := kv.GetAs[string](settings.OAuthProviderKey, "github")
	disablePassword, _ := kv.GetAs[bool](settings.DisablePasswordLoginKey, false)
	respond.Success(ctx, gin.H{
		"oauth_enabled":          oauthEnabled,
		"oauth_provider":         oauthProvider,
		"password_login_enabled": !disablePassword,
		"mode":                   c.mode,
	})
}

func (c *Controller) getStatus(ctx *gin.Context) {
	c.mu.RLock()
	status := c.status
	c.mu.RUnlock()
	respond.Success(ctx, status)
}

func (c *Controller) start(ctx *gin.Context) {
	if c.mode == ModeMetricStructure {
		c.startStructure(ctx)
		return
	}
	c.startLegacy(ctx)
}

func (c *Controller) discard(ctx *gin.Context) {
	if c.mode != ModeMetricStructure {
		respond.Error(ctx, http.StatusNotFound, "history discard is unavailable for this migration")
		return
	}
	c.mu.Lock()
	if c.operationActiveLocked() || c.status.State == "completed" {
		c.mu.Unlock()
		respond.Error(ctx, http.StatusConflict, "database migration is already running or completed")
		return
	}
	c.status = Status{Mode: c.mode, State: "discarding", Phase: "discarding"}
	c.mu.Unlock()

	go c.runDiscard()
	respond.SuccessMessage(ctx, "historical metric data deletion started", gin.H{})
}

func (c *Controller) startStructure(ctx *gin.Context) {
	c.mu.Lock()
	if c.operationActiveLocked() || c.status.State == "completed" {
		c.mu.Unlock()
		respond.Error(ctx, http.StatusConflict, "database migration is already running or completed")
		return
	}
	c.status = Status{Mode: c.mode, State: "copying", Phase: "preparing"}
	c.mu.Unlock()

	go c.runStructure()
	respond.SuccessMessage(ctx, "database migration started", gin.H{})
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

func (c *Controller) operationActiveLocked() bool {
	switch c.status.State {
	case "cleaning", "discarding", "migrating", "copying", "reclaiming":
		return true
	default:
		return false
	}
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

func (c *Controller) runStructure() {
	result, err := metricruntime.RestructureConfiguredStore(context.Background(), func(progress metricruntime.RestructureProgress) {
		c.mu.Lock()
		c.status.Phase = progress.Phase
		c.status.CurrentMetric = progress.CurrentMetric
		c.status.RowsDone = progress.RowsDone
		c.status.RowsTotal = progress.RowsTotal
		c.status.MetricsDone = progress.MetricsDone
		c.status.MetricsTotal = progress.MetricsTotal
		c.status.Progress = structureProgressPercent(progress)
		if progress.Phase == "reclaiming" {
			c.status.State = "reclaiming"
		} else if progress.Phase == "discarding" {
			c.status.State = "discarding"
		} else if c.status.State == "reclaiming" || c.status.State == "discarding" {
			c.status.State = "copying"
		}
		c.mu.Unlock()
	})
	if err != nil {
		c.fail(metricruntime.RedactConnectionError(err.Error(), ""), "failed")
		return
	}

	saved := result.BeforeBytes - result.AfterBytes
	if saved < 0 {
		saved = 0
	}
	percent := 0.0
	if result.BeforeBytes > 0 {
		percent = float64(saved) / float64(result.BeforeBytes) * 100
	}
	c.mu.Lock()
	c.status = Status{
		Mode: c.mode, State: "completed", Phase: "completed", Progress: 100,
		RowsDone: result.RowsCopied, RowsTotal: result.RowsCopied,
		MetricsDone: result.Metrics, MetricsTotal: result.Metrics,
		BeforeBytes: result.BeforeBytes, AfterBytes: result.AfterBytes,
		SavedBytes: saved, SavedPercent: percent,
	}
	c.mu.Unlock()
	c.completeLater()
}

func (c *Controller) runDiscard() {
	result, err := metricruntime.DiscardConfiguredStoreHistory(context.Background(), func(progress metricruntime.RestructureProgress) {
		c.mu.Lock()
		c.status.Phase = progress.Phase
		c.status.CurrentMetric = progress.CurrentMetric
		c.status.RowsDone = progress.RowsDone
		c.status.RowsTotal = progress.RowsTotal
		c.status.MetricsDone = progress.MetricsDone
		c.status.MetricsTotal = progress.MetricsTotal
		switch progress.Phase {
		case "reclaiming":
			c.status.State = "reclaiming"
		case "discarding":
			c.status.State = "discarding"
		}
		c.status.Progress = structureProgressPercent(progress)
		c.mu.Unlock()
	})
	if err != nil {
		c.fail(metricruntime.RedactConnectionError(err.Error(), ""), "discarding")
		return
	}
	saved := result.BeforeBytes - result.AfterBytes
	if saved < 0 {
		saved = 0
	}
	percent := 0.0
	if result.BeforeBytes > 0 {
		percent = float64(saved) / float64(result.BeforeBytes) * 100
	}
	c.mu.Lock()
	c.status = Status{
		Mode: c.mode, State: "completed", Phase: "completed", Progress: 100,
		RowsDone: result.RowsCopied, RowsTotal: result.RowsCopied,
		MetricsDone: result.Metrics, MetricsTotal: result.Metrics,
		BeforeBytes: result.BeforeBytes, AfterBytes: result.AfterBytes,
		SavedBytes: saved, SavedPercent: percent,
	}
	c.mu.Unlock()
	c.completeLater()
}

func (c *Controller) completeLater() {
	time.AfterFunc(1500*time.Millisecond, func() {
		c.Deactivate()
		c.once.Do(func() { close(c.done) })
	})
}

func (c *Controller) fail(message, phase string) {
	c.mu.Lock()
	c.status.State = "failed"
	c.status.Phase = phase
	c.status.Error = message
	c.mu.Unlock()
}

func (c *Controller) failTarget(err error, dsn, phase string) {
	message := err.Error()
	if dsn != "" {
		message = strings.ReplaceAll(message, dsn, "[redacted]")
	}
	c.fail(message, phase)
}

func structureProgressPercent(progress metricruntime.RestructureProgress) float64 {
	if progress.Phase == "reclaiming" {
		return structureProgressCeiling
	}
	if progress.Phase == "discarding" && progress.RowsTotal == 0 {
		return 1
	}
	if progress.RowsTotal <= 0 {
		return 0
	}
	value := float64(progress.RowsDone) / float64(progress.RowsTotal) * structureProgressCeiling
	if value < 0 {
		return 0
	}
	if value > structureProgressCeiling {
		return structureProgressCeiling
	}
	return value
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

func decodeJSON(ctx *gin.Context, target any) error {
	return respond.DecodeJSONBody(ctx, target, 1<<20)
}
