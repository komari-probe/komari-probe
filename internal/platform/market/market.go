// Package market holds the small validation/ID helpers shared by every
// admin-managed marketplace (currently the theme and plugin markets).
package market

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"time"

	"github.com/komari-monitor/komari/internal/database/models"
)

const (
	// PackageMaxSize limits a downloaded market package (theme/plugin zip).
	PackageMaxSize = 100 << 20
	// CatalogMaxSize limits a downloaded market catalog document.
	CatalogMaxSize = 2 << 20
	// CacheTTL is how long a fetched catalog is cached before refetching.
	CacheTTL = 10 * time.Minute
)

// IsValidShort validates a market entry short name (used as both the config
// key and the on-disk directory name).
func IsValidShort(short string) bool {
	if short == "" || short == "default" {
		return false
	}
	for _, r := range short {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// ValidateURLSyntax reports whether rawURL is a syntactically valid,
// credential-free HTTP(S) URL.
func ValidateURLSyntax(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return errors.New("must be a valid HTTP or HTTPS URL")
	}
	return nil
}

// IsText reports whether a market entry field is usable: a non-empty string
// or an i18n object with at least one non-empty value.
func IsText(value any) bool {
	return models.IsLocalizedText(value)
}

// NewSourceID generates a random ID for a newly added market source.
func NewSourceID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
