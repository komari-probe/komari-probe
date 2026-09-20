package main

import (
	"log/slog"
	"testing"
)

func TestLogLevelFromEnv(t *testing.T) {
	cases := []struct {
		value string
		want  slog.Level
	}{
		{"", slog.LevelInfo},
		{"info", slog.LevelInfo},
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"bogus", slog.LevelInfo},
	}
	for _, c := range cases {
		t.Setenv("KOMARI_LOG_LEVEL", c.value)
		if c.value == "" {
			t.Setenv("KOMARI_LOG_LEVEL", "")
		}
		if got := logLevelFromEnv(); got != c.want {
			t.Errorf("KOMARI_LOG_LEVEL=%q: logLevelFromEnv() = %v, want %v", c.value, got, c.want)
		}
	}
}
