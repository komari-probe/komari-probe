package server

import (
	"errors"
	"os"

	logger "github.com/komari-monitor/komari/pkg/log"
)

// Start builds an App and runs it through the full startup lifecycle:
// bootstrap, first-run install guide, database migration, metric store
// connection/recovery, and finally the normal serving loop. It blocks until
// the process should exit; a fatal error at any phase logs and exits the
// process, so a normal return means a clean, voluntary stop (e.g. an
// incomplete install/recovery guide).
func Start(listenAddr string) {
	a := New(Options{ListenAddr: listenAddr})
	if err := a.Bootstrap(); err != nil {
		_ = a.Shutdown()
		logger.Fatalf("server", "server startup failed at %q: %v", "bootstrap", err)
	}

	installRequired, err := a.InstallRequired()
	if err != nil {
		_ = a.Shutdown()
		logger.Fatalf("server", "server startup failed at %q: %v", "detect-first-run-install", err)
	}
	if installRequired {
		completed, err := a.RunInstallGuide()
		if err != nil {
			_ = a.Shutdown()
			logger.Fatalf("server", "server startup failed at %q: %v", "run-first-run-install", err)
		}
		if !completed {
			return
		}
	}

	for {
		requirement, err := a.DatabaseMigrationRequired()
		if err != nil {
			completed, recoveryErr := a.RunMetricStoreRecovery(err)
			if recoveryErr != nil {
				_ = a.Shutdown()
				logger.Fatalf("server", "server startup failed at %q: %v", "database-migration-detection-recovery", recoveryErr)
			}
			if !completed {
				return
			}
			continue
		}
		if !requirement.Required() {
			break
		}
		completed, err := a.RunDatabaseMigration(requirement)
		if err != nil {
			_ = a.Shutdown()
			logger.Fatalf("server", "server startup failed at %q: %v", "run-database-migration", err)
		}
		if !completed {
			return
		}
	}

	// Metric store is the only startup phase allowed to enter the recovery
	// guide: the primary database is already ready from Bootstrap, so login
	// stays available while an admin corrects the DSN.
	if err := a.ConnectMetricStoreWithRetry(); err != nil {
		completed, recoveryErr := a.RunMetricStoreRecovery(err)
		if recoveryErr != nil {
			_ = a.Shutdown()
			logger.Fatalf("server", "server startup failed at %q: %v", "metric-store-recovery", recoveryErr)
		}
		if !completed {
			return
		}
	}

	// Remaining phases: any failure here must stop the process rather than
	// serve from a half-initialized state.
	type stage struct {
		name string
		fn   func() error
	}
	stages := []stage{
		{"init-stores", a.InitStores},
		{"init-providers", a.InitProviders},
		{"start-background", a.StartBackground},
		{"build-router", a.BuildRouter},
	}
	for _, s := range stages {
		if err := s.fn(); err != nil {
			_ = a.Shutdown()
			logger.Fatalf("server", "server startup failed at %q: %v", s.name, err)
		}
	}

	if err := a.Run(); err != nil {
		if errors.Is(err, ErrRestartRequested) {
			logger.Infof("server", "Service stopped for restart: %v", err)
			os.Exit(1)
		}
		logger.Fatalf("server", "server exited with error: %v", err)
	}
}
