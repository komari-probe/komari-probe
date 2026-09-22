package install

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/features/backup"
	"github.com/sonar-probe/sonar/internal/platform/metricruntime"
	"github.com/sonar-probe/sonar/internal/platform/models"
	"github.com/sonar-probe/sonar/internal/platform/respond"
	"github.com/sonar-probe/sonar/internal/platform/upload"
	"gorm.io/gorm"
)

const (
	PagePath = "/install"
	APIPath  = "/api/install"
)

type Status struct {
	State    string `json:"state"`
	Required bool   `json:"required"`
}

// IsRequired reports whether the instance still needs the first-run install
// guide: true as long as no user account has been created yet.
func IsRequired(db *gorm.DB) (bool, error) {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

type completeRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Sitename    string `json:"sitename"`
	Description string `json:"description"`
	MetricDSN   string `json:"metric_dsn"`
}

type Controller struct {
	db     *gorm.DB
	active atomic.Bool
	mu     sync.Mutex
	state  string
	done   chan struct{}
}

func NewController(db *gorm.DB) *Controller {
	return &Controller{db: db, state: "ready", done: make(chan struct{})}
}

func (c *Controller) Activate() { c.active.Store(true) }

func (c *Controller) Deactivate() { c.active.Store(false) }

func (c *Controller) Done() <-chan struct{} { return c.done }

func (c *Controller) Register(r *gin.Engine) {
	g := r.Group(APIPath, c.requireActive)
	g.GET("/status", c.status)
	g.POST("/complete", c.complete)
	uploadHandler := upload.NewHandler(upload.DefaultStore, map[upload.Purpose]upload.Finalizer{
		upload.PurposeBackup: c.finalizeBackupUpload,
	})
	uploadGroup := g.Group("/upload")
	{
		uploadGroup.POST("/init", uploadHandler.Init)
		uploadGroup.POST("/chunk", uploadHandler.Chunk)
		uploadGroup.POST("/merge", uploadHandler.Merge)
		uploadGroup.POST("/cancel", uploadHandler.Cancel)
	}
}

func (c *Controller) requireActive(ctx *gin.Context) {
	if !c.active.Load() {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	ctx.Next()
}

func (c *Controller) status(ctx *gin.Context) {
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()
	respond.Success(ctx, Status{State: state, Required: state != "completed"})
}

func (c *Controller) finalizeBackupUpload(session upload.Session) (upload.Result, error) {
	// FinalizeUploadedRestore holds the restore lock until the process
	// actually restarts, so a second request can't replace backup.zip in
	// the meantime. Re-implementing that sequence here (open + save +
	// sleep + exit) previously dropped the lock immediately after saving,
	// reopening that race during the install flow.
	if err := backup.FinalizeUploadedRestore(session.ArchivePath, session.Metadata.Filename); err != nil {
		return upload.Result{}, err
	}
	return upload.Result{Message: "backup uploaded; restarting to restore", Data: gin.H{}}, nil
}

func (c *Controller) complete(ctx *gin.Context) {
	var request completeRequest
	if err := decodeJSON(ctx, &request); err != nil {
		respond.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateRequest(&request); err != nil {
		respond.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	c.mu.Lock()
	if c.state != "ready" {
		c.mu.Unlock()
		respond.Error(ctx, http.StatusConflict, "installation is already completed or running")
		return
	}
	c.state = "completing"
	c.mu.Unlock()

	cfg, err := metricConfig(request)
	if err == nil {
		pingCtx, cancel := context.WithTimeout(ctx.Request.Context(), 15*time.Second)
		err = metricruntime.TestConnection(pingCtx, cfg)
		cancel()
	}
	if err != nil {
		c.fail()
		respond.Error(ctx, http.StatusBadRequest, fmt.Sprintf("monitoring database connection failed: %v", err))
		return
	}

	if err := c.createAccountAndSettings(&request, cfg); err != nil {
		c.fail()
		respond.Error(ctx, http.StatusInternalServerError, "failed to save installation settings")
		return
	}

	c.mu.Lock()
	c.state = "completed"
	c.mu.Unlock()
	go func() {
		time.Sleep(500 * time.Millisecond)
		c.Deactivate()
		close(c.done)
	}()
	respond.SuccessMessage(ctx, "installation completed", gin.H{})
}

func (c *Controller) fail() {
	c.mu.Lock()
	c.state = "ready"
	c.mu.Unlock()
}
