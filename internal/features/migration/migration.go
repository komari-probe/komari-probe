// Package migration serves the authenticated database migration guide used by
// both legacy monitoring imports and Metric Store structure upgrades.
package migration

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/features/auth"
	"github.com/sonar-probe/sonar/internal/platform/metricruntime"
	"github.com/sonar-probe/sonar/internal/platform/migrations"
	"github.com/sonar-probe/sonar/internal/platform/respond"
	"github.com/sonar-probe/sonar/internal/platform/settings"
	jsonrpc "github.com/sonar-probe/sonar/internal/transport/jsonrpc"
	"github.com/sonar-probe/sonar/pkg/kv"
	"gorm.io/gorm"
)

const (
	PagePath = "/admin/database-migration"
	APIPath  = "/api/admin/database-migration"
)

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

func (c *Controller) operationActiveLocked() bool {
	switch c.status.State {
	case "cleaning", "discarding", "migrating", "copying", "reclaiming":
		return true
	default:
		return false
	}
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
	// runStructure/runDiscard already go through metricruntime.RedactConnectionError,
	// which also strips password=... and user:pass@host fragments via regex, not
	// just the literal DSN substring. Use the same helper here so a legacy-migration
	// error can't leak credentials that don't appear verbatim in the message.
	c.fail(metricruntime.RedactConnectionError(err.Error(), dsn), phase)
}

func decodeJSON(ctx *gin.Context, target any) error {
	return respond.DecodeJSONBody(ctx, target, 1<<20)
}
