package factory

import (
	"github.com/sonar-probe/sonar/pkg/logger"

	"github.com/sonar-probe/sonar/pkg/formfields"
)

var (
	senders                = make(map[string]IMessageSender)
	senderConstructor      = make(map[string]MessageSenderConstructor)
	sendersAdditionalItems = make(map[string][]formfields.Field)
)

func RegisterMessageSender(constructor MessageSenderConstructor) {
	sender := constructor()
	if sender == nil {
		panic("Message sender constructor returned nil")
	}
	senderConstructor[sender.GetName()] = constructor
	if _, exists := senders[sender.GetName()]; exists {
		logger.InfoArgs("message-sender", "Message sender already registered: "+sender.GetName())
	}
	senders[sender.GetName()] = sender

	// 使用反射来提取提供程序的配置字段
	config := sender.GetConfiguration()
	items := formfields.Parse(config)

	sendersAdditionalItems[sender.GetName()] = items
}

func GetSenderConfigs() map[string][]formfields.Field {
	return sendersAdditionalItems
}

func GetAllMessageSenders() map[string]IMessageSender {
	return senders
}

func GetConstructor(name string) (MessageSenderConstructor, bool) {
	constructor, exists := senderConstructor[name]
	return constructor, exists
}

func GetAllMessageSenderNames() []string {
	names := make([]string, 0, len(senders))
	for name := range senders {
		names = append(names, name)
	}
	return names
}

func Initialize() {
	for _, sender := range senders {
		if err := sender.Init(); err != nil {
			logger.Errorf("message-sender", "Failed to initialize message sender %s: %v", sender.GetName(), err)
		}
	}
}
