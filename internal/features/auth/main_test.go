package auth

import (
	"os"
	"testing"

	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/flags"
)

func TestMain(m *testing.M) {
	flags.DatabaseType = flags.DatabaseTypeSQLite
	flags.DatabaseFile = "file:web_api_public_test?mode=memory&cache=shared"

	db := dbcore.GetDBInstance()
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	os.Exit(m.Run())
}
