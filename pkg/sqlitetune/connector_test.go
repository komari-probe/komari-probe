package sqlitetune

import (
	"testing"
	"time"
)

func TestNormalizeAllowsUnlimitedJournalSize(t *testing.T) {
	options, err := normalize(Options{
		BusyTimeout:           5 * time.Second,
		CacheSizeKB:           8,
		WALAutoCheckpoint:     1,
		JournalSizeLimitBytes: -1,
	})
	if err != nil {
		t.Fatalf("normalize unlimited journal size: %v", err)
	}
	if options.JournalSizeLimitBytes != -1 {
		t.Fatalf("journal size limit = %d, want -1", options.JournalSizeLimitBytes)
	}
}

func TestNormalizeRejectsNonPositiveBusyTimeout(t *testing.T) {
	base := Options{
		BusyTimeout:       0,
		CacheSizeKB:       8,
		WALAutoCheckpoint: 1,
	}
	if _, err := normalize(base); err == nil {
		t.Fatal("expected error for zero busy timeout, got nil")
	}

	base.BusyTimeout = -1 * time.Second
	if _, err := normalize(base); err == nil {
		t.Fatal("expected error for negative busy timeout, got nil")
	}
}
