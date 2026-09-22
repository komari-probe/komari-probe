package install

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/features/auth"
	"github.com/sonar-probe/sonar/internal/platform/metricruntime"
	"github.com/sonar-probe/sonar/internal/platform/respond"
	"github.com/sonar-probe/sonar/internal/platform/settings"
	"github.com/sonar-probe/sonar/pkg/kv"
)

func (c *Controller) createAccountAndSettings(request *completeRequest, cfg *metricruntime.MetricStoreConfig) error {
	required, err := IsRequired(c.db)
	if err != nil {
		return err
	}
	if !required {
		return fmt.Errorf("installation is already completed")
	}
	user, err := auth.CreateAccountWithDB(c.db, request.Username, request.Password)
	if err != nil {
		return err
	}
	installSettings := map[string]any{
		settings.SitenameKey:            request.Sitename,
		settings.DescriptionKey:         request.Description,
		metricruntime.MetricDBDriverKey: cfg.Driver,
		metricruntime.MetricDBDSNKey:    cfg.DSN,
	}
	if err := kv.SetMany(installSettings); err != nil {
		_ = auth.DeleteAccountByUsernameWithDB(c.db, user.Username)
		return err
	}
	return nil
}

func validateRequest(request *completeRequest) error {
	request.Username = strings.TrimSpace(request.Username)
	request.Sitename = strings.TrimSpace(request.Sitename)
	request.Description = strings.TrimSpace(request.Description)
	request.MetricDSN = strings.TrimSpace(request.MetricDSN)
	if request.Username == "" || utf8.RuneCountInString(request.Username) > 64 {
		return fmt.Errorf("username must be between 1 and 64 characters")
	}
	passwordLength := utf8.RuneCountInString(request.Password)
	if passwordLength < 8 || passwordLength > 256 {
		return fmt.Errorf("password must be between 8 and 256 characters")
	}
	if !hasStrongPassword(request.Password) {
		return fmt.Errorf("password must contain uppercase, lowercase letters, and numbers")
	}
	if request.Sitename == "" || utf8.RuneCountInString(request.Sitename) > 100 {
		return fmt.Errorf("site name must be between 1 and 100 characters")
	}
	if utf8.RuneCountInString(request.Description) > 1000 {
		return fmt.Errorf("site description must be at most 1000 characters")
	}
	if request.MetricDSN == "" {
		return fmt.Errorf("monitoring database DSN is required")
	}
	return nil
}

func hasStrongPassword(password string) bool {
	var upper, lower, digit bool
	for _, char := range password {
		upper = upper || unicode.IsUpper(char)
		lower = lower || unicode.IsLower(char)
		digit = digit || unicode.IsDigit(char)
	}
	return upper && lower && digit
}

func metricConfig(request completeRequest) (*metricruntime.MetricStoreConfig, error) {
	dsn := request.MetricDSN
	driver, ok := metricruntime.InferDriverFromDSN(dsn)
	if !ok {
		return nil, fmt.Errorf("cannot infer monitoring database type from DSN")
	}
	return &metricruntime.MetricStoreConfig{Driver: string(driver), DSN: dsn}, nil
}

func decodeJSON(ctx *gin.Context, target any) error {
	return respond.DecodeJSONBody(ctx, target, 1<<20)
}
