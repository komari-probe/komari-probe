package auditlog

import (
	"time"

	"github.com/sonar-probe/sonar/internal/platform/dbcore"
	"github.com/sonar-probe/sonar/internal/platform/models"
	"github.com/sonar-probe/sonar/pkg/logger"
)

// Log persists an audit log entry. Failures are logged and otherwise
// swallowed: callers use this for best-effort auditing, not as a source of
// error handling.
func Log(ip, uuid, message, msgType string) {
	db := dbcore.GetDBInstance()
	logEntry := &models.Log{
		IP:      ip,
		UUID:    uuid,
		Message: message,
		MsgType: msgType,
		Time:    time.Now().UTC(),
	}
	if err := db.Create(logEntry).Error; err != nil {
		logger.Error("audit", "failed to persist audit event", "error", err, "type", msgType)
	}
}

// EventLog persists a system-generated audit log entry (no IP/UUID actor).
func EventLog(eventType, message string) {
	Log("", "", message, eventType)
}

// RemoveOldLogs deletes audit log entries older than 30 days.
func RemoveOldLogs() {
	db := dbcore.GetDBInstance()
	threshold := time.Now().UTC().AddDate(0, 0, -30)
	if err := db.Where("time < ?", threshold).Delete(&models.Log{}).Error; err != nil {
		logger.ErrorArgs("audit", "Failed to remove old logs:", err)
	}
}
