package auth

import (
	"os"
	"testing"

	"github.com/komari-monitor/komari/internal/platform/dbcore"
)

func TestMain(m *testing.M) {
	dbcore.DatabaseType = dbcore.DatabaseTypeSQLite
	dbcore.DatabaseFile = "file:web_api_public_test?mode=memory&cache=shared"

	db := dbcore.GetDBInstance()
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	os.Exit(m.Run())
}
