package migrations

import (
	"fmt"
	"strings"

	"github.com/sonar-probe/sonar/pkg/logger"

	"github.com/sonar-probe/sonar/internal/platform/models"
	"gorm.io/gorm"
)

type Context struct {
	DB *gorm.DB
}

// Run executes one-shot startup migrations before current runtime paths are used.
func Run(ctx Context) error {
	db := ctx.DB
	if db == nil {
		return fmt.Errorf("migration database is nil")
	}

	legacyConfigTable := hasLegacyConfigTable(db)
	if err := migrateLegacyTimestampColumns(db); err != nil {
		return err
	}

	if legacyConfigTable {
		if err := migrateLegacyOidcConfig(db); err != nil {
			return err
		}
		if err := migrateLegacyMessageSenderConfig(db); err != nil {
			return err
		}
	}
	if err := migrateLegacyClientInfo(db); err != nil {
		return err
	}
	if err := migrateLegacyLoadNotification(db); err != nil {
		return err
	}
	if err := migrateLegacyPingAllClientsExpansion(db); err != nil {
		return err
	}
	if legacyConfigTable {
		if err := migrateLegacyConfigToItems(db); err != nil {
			return err
		}
	}
	if err := migrateDeprecatedMetricRetentionConfig(db); err != nil {
		return err
	}
	if err := migrateRemovedCompatibilityConfig(db); err != nil {
		return err
	}
	if err := migrateDefaultSitenameRebrand(db); err != nil {
		return err
	}
	if err := markTimestampMigrationDone(db); err != nil {
		return fmt.Errorf("mark UTC timestamp migration done: %w", err)
	}

	return nil
}

func migrateLegacyLoadNotification(db *gorm.DB) error {
	if db.Migrator().HasColumn(&models.LoadNotification{}, "client") {
		logger.InfoArgs("migration", "[>0.1.4] Rebuilding LoadNotification table....")
		return db.Migrator().DropTable(&models.LoadNotification{})
	}
	return nil
}

func hasLegacyConfigTable(db *gorm.DB) bool {
	if !db.Migrator().HasTable("configs") {
		return false
	}
	return hasTableColumn(db, "configs", "id") ||
		hasTableColumn(db, "configs", "sitename")
}

func hasTableColumn(db *gorm.DB, tableName, columnName string) bool {
	columns, err := db.Migrator().ColumnTypes(tableName)
	if err != nil {
		return false
	}
	for _, column := range columns {
		if strings.EqualFold(column.Name(), columnName) {
			return true
		}
	}
	return false
}
