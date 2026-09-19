package oauth

import (
	_ "github.com/komari-monitor/komari/internal/web/oauth/factory"
	_ "github.com/komari-monitor/komari/internal/web/oauth/generic"
	_ "github.com/komari-monitor/komari/internal/web/oauth/github"
	_ "github.com/komari-monitor/komari/internal/web/oauth/qq"
)

func All() {
	//empty function to ensure all OIDC providers are registered
}
