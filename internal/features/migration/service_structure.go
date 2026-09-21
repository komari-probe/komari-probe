package migration

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/platform/metricruntime"
	"github.com/komari-monitor/komari/internal/platform/respond"
)

const structureProgressCeiling = 80.0

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
