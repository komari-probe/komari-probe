package oauth

import (
	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/models"
)

// store.go
// OIDC provider 配置的存取。原先在 internal/database/oauth.go，因为
// 父包 features/auth 的 admin handler 与本包（父包 -> 子包）都需要用到，
// 若放在父包会导致本包反过来 import 父包造成循环，故下沉到这里
// （与 features/notification/messagesender/store.go 是同一处理方式）。

func GetOidcConfigByName(name string) (*models.OidcProvider, error) {
	db := dbcore.GetDBInstance()
	var config models.OidcProvider
	if err := db.Where("name = ?", name).First(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func DeleteOidcConfigByName(name string) error {
	db := dbcore.GetDBInstance()
	return db.Delete(&models.OidcProvider{}, "name = ?", name).Error
}

func SaveOidcConfig(config *models.OidcProvider) error {
	db := dbcore.GetDBInstance()
	if err := db.Save(config).Error; err != nil {
		return err
	}
	return nil
}
