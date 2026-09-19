package main

import (
	"log/slog"

	"github.com/komari-monitor/komari/cmd"
	logger "github.com/komari-monitor/komari/pkg/log"
	"github.com/komari-monitor/komari/utils"
)

func main() {
	if utils.VersionHash == "unknown" {
		logger.Setup(slog.LevelDebug)
	} else {
		logger.Setup(slog.LevelInfo)
	}

	logger.Infof("server", "Komari Monitor %s (hash: %s)", utils.CurrentVersion, utils.VersionHash)

	cmd.Execute()
}
