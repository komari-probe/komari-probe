package notification

import (
	"github.com/sonar-probe/sonar/internal/platform/dbcore"
	"github.com/sonar-probe/sonar/internal/platform/models"
	"gorm.io/gorm"
)

func AddLoadNotification(clients []string, name string, metric string, threshold float32, ratio float32, interval int) (uint, error) {
	db := dbcore.GetDBInstance()
	notification := models.LoadNotification{
		Clients:   clients,
		Name:      name,
		Metric:    metric,
		Threshold: threshold,
		Ratio:     ratio,
		Interval:  interval,
	}
	if err := db.Create(&notification).Error; err != nil {
		return 0, err
	}

	return notification.ID, ReloadLoadNotificationSchedule()
}
func DeleteLoadNotification(id []uint) error {
	db := dbcore.GetDBInstance()
	result := db.Where("id IN ?", id).Delete(&models.LoadNotification{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return ReloadLoadNotificationSchedule()
}

func EditLoadNotification(notifications []*models.LoadNotification) error {
	db := dbcore.GetDBInstance()
	for _, notification := range notifications {
		result := db.Model(&models.LoadNotification{}).Where("id = ?", notification.ID).Updates(notification)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
	}

	return ReloadLoadNotificationSchedule()
}

func GetAllLoadNotifications() ([]models.LoadNotification, error) {
	db := dbcore.GetDBInstance()
	var notifications []models.LoadNotification
	if err := db.Find(&notifications).Error; err != nil {
		return nil, err
	}
	return notifications, nil
}

func ReloadLoadNotificationSchedule() error {
	db := dbcore.GetDBInstance()
	var loadNotifications []models.LoadNotification
	if err := db.Find(&loadNotifications).Error; err != nil {
		return err
	}
	return reloadLoadNotificationSchedule(loadNotifications)
}
