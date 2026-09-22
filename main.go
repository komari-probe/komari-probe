package main

import (
	"log/slog"
	"strings"

	"github.com/sonar-probe/sonar/cmd"
	"github.com/sonar-probe/sonar/internal/version"
	"github.com/sonar-probe/sonar/pkg/logger"
)

func main() {
	logger.Setup(logLevelFromEnv())

	logger.Infof("server", "Sonar %s (hash: %s)", version.CurrentVersion, version.VersionHash)

	cmd.Execute()
}

// logLevelFromEnv 通过 SONAR_LOG_LEVEL（兼容 KOMARI_LOG_LEVEL）环境变量控制日志级别，默认 info。
// VersionHash 只在 CI 构建时通过 -ldflags 注入，本地手动打相同的构建参数
// 也会得到同样的值，用它来猜测"是不是开发环境"并不可靠，因此单独用一个
// 专门的环境变量，不再依赖版本元数据。
func logLevelFromEnv() slog.Level {
	switch strings.ToLower(cmd.GetEnv("SONAR_LOG_LEVEL", cmd.GetEnv("KOMARI_LOG_LEVEL", "info"))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
