package migrations

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/komari-monitor/komari/internal/platform/models"
	"gorm.io/gorm"
)

type legacyPingTask struct {
	ID      uint   `gorm:"column:id"`
	Clients string `gorm:"column:clients"`
}

func migrateLegacyPingAllClientsExpansion(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.PingTask{}) || !db.Migrator().HasTable(&models.Client{}) {
		return nil
	}
	if !hasTableColumn(db, "ping_tasks", "all_clients") {
		return nil
	}
	if !hasTableColumn(db, "ping_tasks", "clients") {
		if err := db.Exec("ALTER TABLE ping_tasks ADD COLUMN clients text").Error; err != nil {
			return fmt.Errorf("add clients column for legacy ping task expansion: %w", err)
		}
	}
	if err := db.Table("ping_tasks").
		Where("clients IS NULL OR clients = '' OR clients = '[]' OR clients = 'null'").
		Update("clients", models.StringArray{}).Error; err != nil {
		return fmt.Errorf("normalize legacy ping task clients: %w", err)
	}

	var pingTasks []legacyPingTask
	if err := db.Table("ping_tasks").Select("id, clients").Where("all_clients = ?", true).Scan(&pingTasks).Error; err != nil {
		return fmt.Errorf("find legacy all_clients ping tasks: %w", err)
	}
	if len(pingTasks) == 0 {
		return nil
	}

	var clients []models.Client
	if err := db.Select("uuid").Find(&clients).Error; err != nil {
		return fmt.Errorf("find clients for legacy ping task expansion: %w", err)
	}
	if len(clients) == 0 {
		return nil
	}

	allUUIDs := make(models.StringArray, 0, len(clients))
	for _, client := range clients {
		if client.UUID != "" {
			allUUIDs = append(allUUIDs, client.UUID)
		}
	}
	if len(allUUIDs) == 0 {
		return nil
	}

	for _, task := range pingTasks {
		if !isLegacyPingClientsEmpty(task.Clients) {
			continue
		}
		if err := db.Table("ping_tasks").Where("id = ?", task.ID).Update("clients", allUUIDs).Error; err != nil {
			return fmt.Errorf("expand legacy all_clients ping task %d: %w", task.ID, err)
		}
	}
	return nil
}

func isLegacyPingClientsEmpty(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return true
	}
	var clients []string
	if err := json.Unmarshal([]byte(raw), &clients); err != nil {
		return false
	}
	return len(clients) == 0
}
