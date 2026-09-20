package factory

import (
	logger "github.com/komari-monitor/komari/pkg/log"

	"github.com/komari-monitor/komari/pkg/formfields"
)

var (
	providers                = make(map[string]IOidcProvider)
	providerConstructor      = make(map[string]OidcConstructor)
	providersAdditionalItems = make(map[string][]formfields.Field)
)

func RegisterOidcProvider(constructor OidcConstructor) {
	provider := constructor()
	if provider == nil {
		panic("OIDC provider constructor returned nil")
	}
	providerConstructor[provider.GetName()] = constructor
	if _, exists := providers[provider.GetName()]; exists {
		logger.InfoArgs("oauth", "OIDC provider already registered: "+provider.GetName())
	}
	providers[provider.GetName()] = provider

	// 使用反射来提取提供程序的配置字段
	config := provider.GetConfiguration()
	items := formfields.Parse(config)
	providersAdditionalItems[provider.GetName()] = items
}

func GetProviderConfigs() map[string][]formfields.Field {
	return providersAdditionalItems
}

func GetAllOidcProviders() map[string]IOidcProvider {
	return providers
}

func GetConstructor(name string) (OidcConstructor, bool) {
	constructor, exists := providerConstructor[name]
	return constructor, exists
}

func Initialize() {
	for _, provider := range providers {
		if err := provider.Init(); err != nil {
			logger.Errorf("oauth", "Failed to initialize OIDC provider %s: %v", provider.GetName(), err)
		}
	}
}
