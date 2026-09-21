package auditlog

import (
	"strings"

	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/models"
	"gorm.io/gorm"
)

// Query returns one page of audit log entries, newest first, optionally
// filtered to an exact message type.
func Query(limit, page int, msgType string) ([]models.Log, int64, error) {
	db := dbcore.GetDBInstance()
	var logs []models.Log
	var total int64
	offset := (page - 1) * limit
	countQuery := filterByMessageType(db.Model(&models.Log{}), msgType)
	logsQuery := filterByMessageType(db.Model(&models.Log{}), msgType)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := logsQuery.Order("time desc").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func filterByMessageType(query *gorm.DB, msgType string) *gorm.DB {
	if msgType = strings.TrimSpace(msgType); msgType != "" {
		return query.Where("msg_type = ?", msgType)
	}
	return query
}
