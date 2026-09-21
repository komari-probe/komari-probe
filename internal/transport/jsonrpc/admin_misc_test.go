package jsonrpc

import (
	"testing"
)

func TestRemoveRetiredLowResourceMode(t *testing.T) {
	cfg := map[string]interface{}{
		"low_resource_mode": true,
		"sitename":          "Komari",
	}

	removeRetiredLowResourceMode(cfg)

	if _, ok := cfg["low_resource_mode"]; ok {
		t.Fatal("retired low resource mode must not be persisted")
	}
	if cfg["sitename"] != "Komari" {
		t.Fatal("unrelated settings must be preserved")
	}
}
