package dbcore

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sonar-probe/sonar/pkg/sqlitetune"
)

const (
	mainSQLiteBusyTimeout       = 5 * time.Second
	mainSQLiteCacheSizeKB       = 8 * 1024
	mainSQLiteWALAutoCheckpoint = 256
	mainSQLiteJournalSizeLimit  = 1 << 20
)

// buildSQLiteDSN builds the main database's connection string from the
// configured database file, adding the busy-timeout and immediate-lock
// parameters every connection needs.
func buildSQLiteDSN(databaseFile string) string {
	if databaseFile == "" {
		databaseFile = "./data/komari.db"
	}

	params := fmt.Sprintf("_busy_timeout=%d&_txlock=immediate", mainSQLiteBusyTimeout.Milliseconds())
	separator := "?"
	if strings.Contains(databaseFile, "?") {
		separator = "&"
	}

	if strings.HasPrefix(databaseFile, "file:") {
		return databaseFile + separator + params
	}

	if databaseFile == ":memory:" {
		return "file::memory:?cache=shared&" + params
	}

	return "file:" + filepath.ToSlash(databaseFile) + separator + params
}

// mainSQLiteOptions returns the per-connection PRAGMA tuning applied to the
// main database by sqlitetune.
func mainSQLiteOptions() sqlitetune.Options {
	return sqlitetune.Options{
		BusyTimeout:           mainSQLiteBusyTimeout,
		CacheSizeKB:           mainSQLiteCacheSizeKB,
		MMapSizeBytes:         0,
		TempStoreMemory:       false,
		CacheSpill:            true,
		WALAutoCheckpoint:     mainSQLiteWALAutoCheckpoint,
		JournalSizeLimitBytes: mainSQLiteJournalSizeLimit,
		Synchronous:           sqlitetune.SynchronousNormal,
	}
}
