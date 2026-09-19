package main

import (
	"log/slog"

	"github.com/komari-monitor/komari/cmd"
	logger "github.com/komari-monitor/komari/pkg/log"
	"github.com/komari-monitor/komari/pkg/version"
)

func main() {
	if version.VersionHash == "unknown" {
		logger.Setup(slog.LevelDebug)
	} else {
		logger.Setup(slog.LevelInfo)
	}

	logger.Infof("server", "Komari Monitor %s (hash: %s)", version.CurrentVersion, version.VersionHash)

	cmd.Execute()
}
