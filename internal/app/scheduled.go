package app

// Background cron-style maintenance work: scheduling it (StartBackground,
// registerScheduledWork) and the individual jobs it drives. Kept apart from
// runtime.go, which owns the HTTP serve/shutdown lifecycle instead.

import (
	"context"
	"errors"
	"time"

	"github.com/komari-monitor/komari/internal/features/auth"
	"github.com/komari-monitor/komari/internal/features/notification"
	"github.com/komari-monitor/komari/internal/features/ping"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/internal/platform/metricruntime"
	"github.com/komari-monitor/komari/pkg/logger"
	"github.com/komari-monitor/komari/pkg/scheduler"
)

// StartBackground starts scheduled work after all stores are ready.
func (a *App) StartBackground() error {
	registerScheduledWork()
	a.addCleanup("scheduler", func(context.Context) error {
		scheduler.StopAll()
		return nil
	})
	return nil
}

func registerScheduledWork() {
	if err := ping.ReloadPingSchedule(); err != nil {
		logger.ErrorArgs("server", "Failed to reload ping schedule:", err)
	}
	if err := notification.ReloadLoadNotificationSchedule(); err != nil {
		logger.ErrorArgs("server", "Failed to reload load notification schedule:", err)
	}
	if err := scheduler.AddFunc("records:cleanup", "@every 30m", cleanupScheduledData); err != nil {
		logger.ErrorArgs("server", "Failed to add cleanup scheduled task:", err)
	}
	if err := scheduler.AddContextFunc("metrics:compact", "@every 5m", true, compactMetricStore); err != nil {
		logger.ErrorArgs("server", "Failed to add metric compact scheduled task:", err)
	}
	if err := scheduler.AddContextFunc("metrics:retention", "@every 1h", true, cleanupMetricStore); err != nil {
		logger.ErrorArgs("server", "Failed to add metric retention scheduled task:", err)
	}
	if err := scheduler.AddFunc("notifier:traffic", "@every 1m", notification.CheckTraffic); err != nil {
		logger.ErrorArgs("server", "Failed to add traffic notification task:", err)
	}
	if err := scheduler.AddFunc("notifier:expire", "0 0 9 * * *", notification.CheckExpireScheduledWork); err != nil {
		logger.ErrorArgs("server", "Failed to add expire notification task:", err)
	}
}

func cleanupScheduledData() {
	auditlog.RemoveOldLogs()
	auth.RemoveExpiredSessions()
}

func compactMetricStore(ctx context.Context) {
	compactCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	written, err := metricruntime.Compact(compactCtx, time.Now().UTC())
	if errors.Is(err, metricruntime.ErrCompactInProgress) {
		return
	}
	if err != nil {
		logger.Errorf("server", "Failed to compact metric store after writing %d rollup buckets: %v", written, err)
		return
	}
	if written > 0 {
		logger.Infof("server", "Metric store compacted %d rollup buckets", written)
	}
}

func cleanupMetricStore(ctx context.Context) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	deleted, err := metricruntime.CleanupExpired(cleanupCtx, time.Now().UTC())
	if errors.Is(err, metricruntime.ErrCompactInProgress) {
		return
	}
	if err != nil {
		logger.Errorf("server", "Failed to clean expired metric data after deleting %d rows: %v", deleted, err)
		return
	}
	if deleted > 0 {
		logger.Infof("server", "Metric retention cleanup deleted %d rows", deleted)
	}
}
