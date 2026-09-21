package dbcore

import "strings"

// DatabaseTypeSQLite is the only supported main-database driver.
const DatabaseTypeSQLite = "sqlite"

var (
	// DatabaseType and DatabaseFile are populated from the -t/--db-type and
	// -d/--database startup flags (see cmd/root.go) before Initialize runs.
	DatabaseType string
	DatabaseFile string
)

// NormalizeDatabaseType lowercases and trims databaseType, defaulting to
// DatabaseTypeSQLite when empty.
func NormalizeDatabaseType(databaseType string) string {
	databaseType = strings.ToLower(strings.TrimSpace(databaseType))
	if databaseType == "" {
		return DatabaseTypeSQLite
	}
	return databaseType
}

// ApplyDatabaseTypeNormalization normalizes the package-level DatabaseType
// in place and returns the normalized value.
func ApplyDatabaseTypeNormalization() string {
	DatabaseType = NormalizeDatabaseType(DatabaseType)
	return DatabaseType
}

// IsSQLite reports whether the configured database type is SQLite.
func IsSQLite() bool {
	return NormalizeDatabaseType(DatabaseType) == DatabaseTypeSQLite
}

// SupportedDatabaseTypes lists the database types Initialize accepts.
func SupportedDatabaseTypes() string {
	return DatabaseTypeSQLite
}
