package migrations

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/sonar-probe/sonar/pkg/logger"

	"github.com/sonar-probe/sonar/internal/platform/models"
	"github.com/sonar-probe/sonar/internal/platform/settings"
	"github.com/sonar-probe/sonar/pkg/kv"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type legacyModelConfig struct {
	ID                         uint    `json:"id,omitempty" gorm:"primaryKey;autoIncrement"`
	Sitename                   string  `json:"sitename" gorm:"type:varchar(100);not null"`
	Description                string  `json:"description" gorm:"type:text"`
	Theme                      string  `json:"theme" gorm:"type:varchar(100);default:'default'"`
	PrivateSite                bool    `json:"private_site" gorm:"default:false"`
	APIKey                     string  `json:"api_key" gorm:"type:varchar(255);default:''"`
	AutoDiscoveryKey           string  `json:"auto_discovery_key" gorm:"type:varchar(255);default:''"`
	ScriptDomain               string  `json:"script_domain" gorm:"type:varchar(255);default:''"`
	SendIPAddrToGuest          bool    `json:"send_ip_addr_to_guest" gorm:"default:false"`
	EulaAccepted               bool    `json:"eula_accepted" gorm:"default:false"`
	GeoIPEnabled               bool    `json:"geo_ip_enabled" gorm:"default:true"`
	GeoIPProvider              string  `json:"geo_ip_provider" gorm:"type:varchar(20);default:'ip-api'"`
	OAuthEnabled               bool    `json:"o_auth_enabled" gorm:"default:false"`
	OAuthProvider              string  `json:"o_auth_provider" gorm:"type:varchar(50);default:'github'"`
	DisablePasswordLogin       bool    `json:"disable_password_login" gorm:"default:false"`
	CustomHead                 string  `json:"custom_head" gorm:"type:longtext"`
	CustomBody                 string  `json:"custom_body" gorm:"type:longtext"`
	NotificationEnabled        bool    `json:"notification_enabled" gorm:"default:false"`
	NotificationMethod         string  `json:"notification_method" gorm:"type:varchar(64);default:'none'"`
	NotificationTemplate       string  `json:"notification_template" gorm:"type:longtext;default:'{{emoji}}{{emoji}}{{emoji}}\nEvent: {{event}}\nClients: {{client}}\nMessage: {{message}}\nTime: {{time}}'"`
	ExpireNotificationEnabled  bool    `json:"expire_notification_enabled" gorm:"default:false"`
	ExpireNotificationLeadDays int     `json:"expire_notification_lead_days" gorm:"default:7"`
	LoginNotification          bool    `json:"login_notification" gorm:"default:false"`
	TrafficLimitPercentage     float64 `json:"traffic_limit_percentage" gorm:"default:80.00"`
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

func (legacyModelConfig) TableName() string {
	return "configs"
}

type legacyConfig struct {
	ID                         uint      `json:"id,omitempty"`
	Sitename                   string    `json:"sitename"`
	Description                string    `json:"description"`
	Theme                      string    `json:"theme"`
	PrivateSite                bool      `json:"private_site"`
	APIKey                     string    `json:"api_key"`
	AutoDiscoveryKey           string    `json:"auto_discovery_key"`
	ScriptDomain               string    `json:"script_domain"`
	SendIPAddrToGuest          bool      `json:"send_ip_addr_to_guest"`
	EulaAccepted               bool      `json:"eula_accepted"`
	BaseScriptsURLKey          string    `json:"base_scripts_url"`
	GeoIPEnabled               bool      `json:"geo_ip_enabled"`
	GeoIPProvider              string    `json:"geo_ip_provider"`
	OAuthEnabled               bool      `json:"o_auth_enabled"`
	OAuthProvider              string    `json:"o_auth_provider"`
	DisablePasswordLogin       bool      `json:"disable_password_login"`
	CustomHead                 string    `json:"custom_head"`
	CustomBody                 string    `json:"custom_body"`
	NotificationEnabled        bool      `json:"notification_enabled"`
	NotificationMethod         string    `json:"notification_method"`
	NotificationTemplate       string    `json:"notification_template"`
	ExpireNotificationEnabled  bool      `json:"expire_notification_enabled"`
	ExpireNotificationLeadDays int       `json:"expire_notification_lead_days"`
	LoginNotification          bool      `json:"login_notification"`
	TrafficLimitPercentage     float64   `json:"traffic_limit_percentage"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

func (legacyConfig) TableName() string {
	return "configs"
}

func migrateDeprecatedMetricRetentionConfig(db *gorm.DB) error {
	if !db.Migrator().HasTable(&kv.ConfigItem{}) {
		return nil
	}
	return db.Delete(&kv.ConfigItem{}, "key = ?", "metric_retention_days").Error
}

func migrateRemovedCompatibilityConfig(db *gorm.DB) error {
	if !db.Migrator().HasTable(&kv.ConfigItem{}) {
		return nil
	}
	return db.Delete(&kv.ConfigItem{}, "key IN ?", []string{
		"nezha_compat_enabled",
		"nezha_compat_listen",
		"low_resource_mode",
	}).Error
}

func migrateLegacyOidcConfig(db *gorm.DB) error {
	if db.Migrator().HasTable(&models.OidcProvider{}) {
		return nil
	}

	logger.InfoArgs("migration", "[>1.0.2] Merge OidcProvider table....")
	var oldData struct {
		OAuthClientID     string `gorm:"column:o_auth_client_id"`
		OAuthClientSecret string `gorm:"column:o_auth_client_secret"`
	}
	if err := db.Raw("SELECT * FROM configs LIMIT 1").Scan(&oldData).Error; err != nil {
		return fmt.Errorf("get legacy OIDC config: %w", err)
	}

	if err := db.AutoMigrate(&models.OidcProvider{}); err != nil {
		return err
	}
	addition, err := json.Marshal(map[string]string{
		"client_id":     oldData.OAuthClientID,
		"client_secret": oldData.OAuthClientSecret,
	})
	if err != nil {
		return fmt.Errorf("marshal legacy OIDC config: %w", err)
	}
	if err := db.Save(&models.OidcProvider{
		Name:     "github",
		Addition: string(addition),
	}).Error; err != nil {
		return err
	}

	if err := db.AutoMigrate(&legacyModelConfig{}); err != nil {
		return err
	}
	return db.Model(&legacyModelConfig{}).Where("id = 1").Update("o_auth_provider", "github").Error
}

func migrateLegacyMessageSenderConfig(db *gorm.DB) error {
	if db.Migrator().HasTable(&models.MessageSenderProvider{}) {
		return nil
	}

	logger.InfoArgs("migration", "[>1.0.2] Migrate MessageSender configuration....")
	var oldData struct {
		TelegramBotToken   string `gorm:"column:telegram_bot_token"`
		TelegramChatID     string `gorm:"column:telegram_chat_id"`
		TelegramEndpoint   string `gorm:"column:telegram_endpoint"`
		EmailHost          string `gorm:"column:email_host"`
		EmailPort          int    `gorm:"column:email_port"`
		EmailUsername      string `gorm:"column:email_username"`
		EmailPassword      string `gorm:"column:email_password"`
		EmailSender        string `gorm:"column:email_sender"`
		EmailReceiver      string `gorm:"column:email_receiver"`
		EmailUseSSL        bool   `gorm:"column:email_use_ssl"`
		NotificationMethod string `gorm:"column:notification_method"`
	}
	if err := db.Raw("SELECT * FROM configs LIMIT 1").Scan(&oldData).Error; err != nil {
		return fmt.Errorf("get legacy message sender config: %w", err)
	}
	if err := db.AutoMigrate(&models.MessageSenderProvider{}); err != nil {
		return err
	}

	if oldData.NotificationMethod == "telegram" && oldData.TelegramBotToken != "" {
		telegramConfig := map[string]any{
			"bot_token": oldData.TelegramBotToken,
			"chat_id":   oldData.TelegramChatID,
			"endpoint":  oldData.TelegramEndpoint,
		}
		if telegramConfig["endpoint"] == "" {
			telegramConfig["endpoint"] = "https://api.telegram.org/bot"
		}
		if err := saveLegacyMessageSenderConfig(db, "telegram", telegramConfig); err != nil {
			return err
		}
	}

	if oldData.NotificationMethod == "email" && oldData.EmailHost != "" {
		emailConfig := map[string]any{
			"host":     oldData.EmailHost,
			"port":     oldData.EmailPort,
			"username": oldData.EmailUsername,
			"password": oldData.EmailPassword,
			"sender":   oldData.EmailSender,
			"receiver": oldData.EmailReceiver,
			"use_ssl":  oldData.EmailUseSSL,
		}
		if err := saveLegacyMessageSenderConfig(db, "email", emailConfig); err != nil {
			return err
		}
	}

	for _, column := range []string{
		"telegram_bot_token",
		"telegram_chat_id",
		"telegram_endpoint",
		"email_host",
		"email_port",
		"email_username",
		"email_password",
		"email_sender",
		"email_receiver",
		"email_use_ssl",
	} {
		if hasTableColumn(db, "configs", column) {
			if err := db.Migrator().DropColumn(&legacyModelConfig{}, column); err != nil {
				return err
			}
		}
	}

	return nil
}

func saveLegacyMessageSenderConfig(db *gorm.DB, name string, config map[string]any) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal legacy %s message sender config: %w", name, err)
	}
	return db.Save(&models.MessageSenderProvider{
		Name:     name,
		Addition: string(configJSON),
	}).Error
}

func migrateLegacyConfigToItems(db *gorm.DB) error {
	logger.InfoArgs("migration", "[>1.1.4] Moving legacy config data...")

	var oldData legacyConfig
	if err := db.Order("id desc").First(&oldData).Error; err != nil {
		if err := db.Migrator().DropTable("configs"); err != nil {
			return err
		}
		return db.AutoMigrate(&kv.ConfigItem{})
	}

	newRows, err := legacyConfigRows(oldData)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Migrator().DropTable("configs"); err != nil {
			return err
		}
		if err := tx.AutoMigrate(&kv.ConfigItem{}); err != nil {
			return err
		}
		if len(newRows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&newRows).Error
	})
}

// legacyDefaultSitename is the stock site name shipped by upstream Komari Monitor.
// Instances that never customized it get rebranded to Sonar's own default on migration.
const legacyDefaultSitename = "Komari"

// defaultSitenameRebrandMigrationKey guards migrateDefaultSitenameRebrand so it only
// ever touches the sitename once, letting an admin freely rename the site back to
// "Komari" later without every restart silently reverting it.
const defaultSitenameRebrandMigrationKey = "internal_default_sitename_rebrand_done"

// migrateDefaultSitenameRebrand covers instances that already finished the
// configs -> config-items migration (see migrateLegacyConfigToItems) before this
// rebrand existed, so they were left with the untouched upstream "Komari" default
// baked into the new key-value store instead of Sonar's own default.
func migrateDefaultSitenameRebrand(db *gorm.DB) error {
	if !db.Migrator().HasTable(&kv.ConfigItem{}) {
		return nil
	}

	var marker kv.ConfigItem
	if err := db.Where("key = ?", defaultSitenameRebrandMigrationKey).First(&marker).Error; err == nil && marker.Value == "true" {
		return nil
	}

	legacyDefaultValue, err := json.Marshal(legacyDefaultSitename)
	if err != nil {
		return fmt.Errorf("marshal legacy default sitename: %w", err)
	}

	var sitename kv.ConfigItem
	if err := db.Where("key = ?", settings.SitenameKey).First(&sitename).Error; err == nil && sitename.Value == string(legacyDefaultValue) {
		sonarValue, err := json.Marshal("Sonar")
		if err != nil {
			return fmt.Errorf("marshal rebranded sitename: %w", err)
		}
		if err := db.Model(&kv.ConfigItem{}).Where("key = ?", settings.SitenameKey).Update("value", string(sonarValue)).Error; err != nil {
			return fmt.Errorf("rebrand default sitename: %w", err)
		}
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&kv.ConfigItem{Key: defaultSitenameRebrandMigrationKey, Value: "true"}).Error
}

func legacyConfigRows(oldData legacyConfig) ([]kv.ConfigItem, error) {
	if oldData.Sitename == legacyDefaultSitename {
		oldData.Sitename = "Sonar"
	}

	val := reflect.ValueOf(oldData)
	typ := reflect.TypeOf(oldData)
	newRows := make([]kv.ConfigItem, 0, val.NumField())

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("json")
		key := strings.Split(tag, ",")[0]
		if key == "" || key == "-" || key == "id" {
			continue
		}

		jsonBytes, err := json.Marshal(val.Field(i).Interface())
		if err != nil {
			return nil, fmt.Errorf("marshal legacy config %s: %w", key, err)
		}
		newRows = append(newRows, kv.ConfigItem{
			Key:   key,
			Value: string(jsonBytes),
		})
	}

	return newRows, nil
}
