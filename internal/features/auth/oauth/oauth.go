package oauth

import (
	"encoding/json"
	"fmt"
	"github.com/komari-monitor/komari/pkg/logger"
	"sync"

	"github.com/komari-monitor/komari/internal/features/auth/oauth/factory"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/internal/platform/models"
	"github.com/komari-monitor/komari/internal/platform/settings"
	"github.com/komari-monitor/komari/pkg/kv"
)

// defaultProviderName is used whenever no OIDC provider is configured, or the
// configured provider can no longer be found.
const defaultProviderName = "github"

var (
	currentProvider factory.IOidcProvider
	mu              = sync.Mutex{}
	once            = sync.Once{}
)

const removedCloudflareAccessProvider = "CloudflareAccess"

func CurrentProvider() factory.IOidcProvider {
	mu.Lock()
	defer mu.Unlock()
	return currentProvider
}

// Shutdown 销毁当前 OAuth provider，释放其持有的资源。供关闭流程调用。
func Shutdown() error {
	mu.Lock()
	defer mu.Unlock()
	if currentProvider == nil {
		return nil
	}
	err := currentProvider.Destroy()
	currentProvider = nil
	return err
}

func LoadProvider(name string, configJson string) error {
	mu.Lock()
	defer mu.Unlock()
	if currentProvider != nil {
		if err := currentProvider.Destroy(); err != nil {
			logger.Errorf("oauth", "Failed to destroy provider %s: %v", currentProvider.GetName(), err)
		}
	}
	constructor, exists := factory.GetConstructor(name)
	if !exists {
		return fmt.Errorf("provider %s not found", name)
	}
	currentProvider = constructor()
	if err := json.Unmarshal([]byte(configJson), currentProvider.GetConfiguration()); err != nil {
		return fmt.Errorf("failed to unmarshal config for provider %s: %w", name, err)
	}
	err := currentProvider.Init()
	if err != nil {
		return fmt.Errorf("failed to initialize provider %s: %w", name, err)
	}
	return nil
}

// ReloadProviderByName loads the given OIDC provider, falling back to
// defaultProviderName when providerName is empty, "none", or not found. It is
// the entry point for reacting to a change of settings.OAuthProviderKey.
func ReloadProviderByName(providerName string) {
	if providerName == "" || providerName == "none" {
		providerName = defaultProviderName
	}
	oidcProvider, err := GetOidcConfigByName(providerName)
	if err != nil {
		logger.Errorf("oauth", "Failed to get OIDC provider config: %v", err)
		return
	}
	logger.Infof("oauth", "Using %s as OIDC provider", oidcProvider.Name)
	if err := LoadProvider(oidcProvider.Name, oidcProvider.Addition); err != nil {
		auditlog.EventLog("error", fmt.Sprintf("Failed to load OIDC provider: %v", err))
	}
}

func Initialize() error {
	cleanupRemovedProviders()
	once.Do(func() {
		all := factory.GetAllOidcProviders()
		for _, provider := range all {
			if _, err := GetOidcConfigByName(provider.GetName()); err == nil {
				continue
			}
			// 如果数据库中没有该提供者的配置，则保存默认配置
			config := provider.GetConfiguration()
			configBytes, err := json.Marshal(config)
			if err != nil {
				logger.Errorf("oauth", "Failed to marshal config for provider %s: %v", provider.GetName(), err)
				return
			}
			if err := SaveOidcConfig(&models.OidcProvider{
				Name:     provider.GetName(),
				Addition: string(configBytes),
			}); err != nil {
				logger.Errorf("oauth", "Failed to save default config for provider %s: %v", provider.GetName(), err)
				return
			}
		}
	})
	cfg, _ := kv.GetAs[string](settings.OAuthProviderKey, "github")
	if cfg == "" || cfg == "none" {
		return LoadProvider("github", "{}")
	}
	provider, err := GetOidcConfigByName(cfg)
	if err != nil {
		// 如果没有找到配置，使用github provider
		return LoadProvider("github", "{}")
	}
	err = LoadProvider(provider.Name, provider.Addition)
	if err != nil {
		logger.Errorf("oauth", "Failed to load OIDC provider %s: %v", provider.Name, err)
		return err
	}
	return nil
}

func cleanupRemovedProviders() {
	if err := DeleteOidcConfigByName(removedCloudflareAccessProvider); err != nil {
		logger.Errorf("oauth", "Failed to delete removed OIDC provider %s: %v", removedCloudflareAccessProvider, err)
	}

	cfg, _ := kv.GetAs[string](settings.OAuthProviderKey, "github")
	if cfg == removedCloudflareAccessProvider {
		if err := kv.Set(settings.OAuthProviderKey, "github"); err != nil {
			logger.Errorf("oauth", "Failed to reset removed OIDC provider %s: %v", removedCloudflareAccessProvider, err)
		}
	}
}
