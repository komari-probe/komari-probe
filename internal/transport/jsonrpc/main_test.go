package jsonrpc

import (
	"os"
	"testing"

	"github.com/sonar-probe/sonar/internal/platform/dbcore"
)

// TestMain wires a shared in-memory SQLite database for the whole package's
// tests (originally lived in admin.plugin_test.go, moved here once the
// plugin-specific tests moved to internal/features/plugin).
func TestMain(m *testing.M) {
	dbcore.DatabaseType = dbcore.DatabaseTypeSQLite
	dbcore.DatabaseFile = "file:komari_jsonrpc_test?mode=memory&cache=shared"
	db := dbcore.GetDBInstance()
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	os.Exit(m.Run())
}
