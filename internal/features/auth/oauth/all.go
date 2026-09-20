package oauth

import (
	_ "github.com/komari-monitor/komari/internal/features/auth/oauth/factory"
	_ "github.com/komari-monitor/komari/internal/features/auth/oauth/generic"
	_ "github.com/komari-monitor/komari/internal/features/auth/oauth/github"
	_ "github.com/komari-monitor/komari/internal/features/auth/oauth/qq"
)

func All() {
	//empty function to ensure all OIDC providers are registered
}
