package messagesender

import (
	"github.com/sonar-probe/sonar/internal/platform/dbcore"
	"github.com/sonar-probe/sonar/internal/platform/models"
)

func GetConfigByName(name string) (*models.MessageSenderProvider, error) {
	db := dbcore.GetDBInstance()
	var config models.MessageSenderProvider
	if err := db.Where("name = ?", name).First(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func SaveConfig(config *models.MessageSenderProvider) error {
	db := dbcore.GetDBInstance()
	return db.Save(config).Error
}
