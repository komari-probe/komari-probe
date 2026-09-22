package notification

import (
	"fmt"
	"time"

	"github.com/sonar-probe/sonar/internal/features/notification/messagesender"
	"github.com/sonar-probe/sonar/internal/features/renewal"
	"github.com/sonar-probe/sonar/internal/platform/clients"
	"github.com/sonar-probe/sonar/internal/platform/models"
	"github.com/sonar-probe/sonar/internal/platform/settings"
	"github.com/sonar-probe/sonar/pkg/kv"
	"github.com/sonar-probe/sonar/pkg/timeutil"
)

func CheckExpireScheduledWork() {
	CheckExpire()
}

func CheckExpire() {
	cfg, err := kv.GetMany(map[string]any{
		settings.ExpireNotificationEnabledKey:  false,
		settings.ExpireNotificationLeadDaysKey: 7,
	})
	if err != nil {
		return
	}

	allClients, err := clients.GetAllClientBasicInfo()
	if err != nil {
		return
	}

	checkTime := time.Now().UTC()

	// 过期提醒检查（仅当启用过期通知时）
	if cfg[settings.ExpireNotificationEnabledKey].(bool) {
		notificationLeadDays := int(cfg[settings.ExpireNotificationLeadDaysKey].(float64)) // Json unmarshal 会将数字解析为 float64

		type clientToExpireInfo struct {
			Name     string
			DaysLeft int
		}

		var clientLeadToExpire []clientToExpireInfo

		for _, client := range allClients {
			if client.ExpiredAt == nil {
				continue
			}
			clientExpireTime := client.ExpiredAt.UTC()

			if clientExpireTime.Before(checkTime) {
				continue
			}

			notificationThreshold := checkTime.In(time.Local).AddDate(0, 0, notificationLeadDays).UTC()

			if clientExpireTime.Before(notificationThreshold) || clientExpireTime.Equal(notificationThreshold) {
				daysLeft := timeutil.SystemDateDistance(checkTime, clientExpireTime)

				clientLeadToExpire = append(clientLeadToExpire, clientToExpireInfo{
					Name:     client.Name,
					DaysLeft: daysLeft,
				})
			}
		}

		if len(clientLeadToExpire) > 0 {
			message := ""
			for _, clientInfo := range clientLeadToExpire {
				message += fmt.Sprintf("• %s (%dd)\n", clientInfo.Name, clientInfo.DaysLeft)
			}
			_ = messagesender.SendNotification(models.EventMessage{
				Event:   models.EventExpire,
				Time:    time.Now().UTC(),
				Message: message,
				Emoji:   "⏳",
			})
		}
	}

	for _, client := range allClients {
		renewal.CheckAndAutoRenewal(client)
	}
}
