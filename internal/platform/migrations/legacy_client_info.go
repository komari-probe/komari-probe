package migrations

import (
	"fmt"
	"time"

	"github.com/sonar-probe/sonar/pkg/logger"

	"github.com/sonar-probe/sonar/internal/platform/models"
	"gorm.io/gorm"
)

// ClientInfo is the legacy table shape for migrating pre-Client model data.
type ClientInfo struct {
	UUID           string     `json:"uuid,omitempty" gorm:"type:varchar(36);primaryKey;foreignKey:ClientUUID;references:UUID;constraint:OnDelete:CASCADE"`
	Name           string     `json:"name" gorm:"type:varchar(100);not null"`
	CPUName        string     `json:"cpu_name" gorm:"type:varchar(100)"`
	Virtualization string     `json:"virtualization" gorm:"type:varchar(50)"`
	Arch           string     `json:"arch" gorm:"type:varchar(50)"`
	CPUCores       int        `json:"cpu_cores" gorm:"type:int"`
	OS             string     `json:"os" gorm:"type:varchar(100)"`
	GPUName        string     `json:"gpu_name" gorm:"type:varchar(100)"`
	IPv4           string     `json:"ipv4,omitempty" gorm:"type:varchar(100)"`
	IPv6           string     `json:"ipv6,omitempty" gorm:"type:varchar(100)"`
	Region         string     `json:"region" gorm:"type:varchar(100)"`
	Remark         string     `json:"remark,omitempty" gorm:"type:longtext"`
	PublicRemark   string     `json:"public_remark,omitempty" gorm:"type:longtext"`
	MemTotal       int64      `json:"mem_total" gorm:"type:bigint"`
	SwapTotal      int64      `json:"swap_total" gorm:"type:bigint"`
	DiskTotal      int64      `json:"disk_total" gorm:"type:bigint"`
	Version        string     `json:"version,omitempty" gorm:"type:varchar(100)"`
	Weight         int        `json:"weight" gorm:"type:int"`
	Price          float64    `json:"price"`
	BillingCycle   int        `json:"billing_cycle"`
	ExpiredAt      *time.Time `json:"expired_at" gorm:"type:timestamp"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func migrateLegacyClientInfo(db *gorm.DB) error {
	if !db.Migrator().HasTable("client_infos") {
		return nil
	}

	logger.InfoArgs("migration", "[>0.0.5] Legacy ClientInfo table detected, starting data migration...")
	if err := db.AutoMigrate(&models.Client{}); err != nil {
		return err
	}

	var clientInfos []ClientInfo
	if err := db.Find(&clientInfos).Error; err != nil {
		return fmt.Errorf("read legacy ClientInfo table: %w", err)
	}

	for _, info := range clientInfos {
		var client models.Client
		if err := db.Where("uuid = ?", info.UUID).First(&client).Error; err != nil {
			logger.Errorf("migration", "Could not find Client record with UUID %s: %v", info.UUID, err)
			continue
		}

		client.Name = info.Name
		client.CPUName = info.CPUName
		client.Virtualization = info.Virtualization
		client.Arch = info.Arch
		client.CPUCores = info.CPUCores
		client.OS = info.OS
		client.GPUName = info.GPUName
		client.IPv4 = info.IPv4
		client.IPv6 = info.IPv6
		client.Region = info.Region
		client.Remark = info.Remark
		client.PublicRemark = info.PublicRemark
		client.MemTotal = info.MemTotal
		client.SwapTotal = info.SwapTotal
		client.DiskTotal = info.DiskTotal
		client.Version = info.Version
		client.Weight = info.Weight
		client.Price = info.Price
		client.BillingCycle = info.BillingCycle
		client.ExpiredAt = info.ExpiredAt
		if err := db.Save(&client).Error; err != nil {
			return fmt.Errorf("update Client record %s: %w", info.UUID, err)
		}
	}

	if err := db.Migrator().RenameTable("client_infos", "client_infos_backup"); err != nil {
		return fmt.Errorf("backup legacy ClientInfo table: %w", err)
	}
	logger.InfoArgs("migration", "Data migration completed, old table has been backed up as client_infos_backup")
	return nil
}
