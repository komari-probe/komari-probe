package oauth

import (
	_ "github.com/sonar-probe/sonar/internal/features/auth/oauth/factory"
	_ "github.com/sonar-probe/sonar/internal/features/auth/oauth/generic"
	_ "github.com/sonar-probe/sonar/internal/features/auth/oauth/github"
	_ "github.com/sonar-probe/sonar/internal/features/auth/oauth/qq"
)

func All() {
	//empty function to ensure all OIDC providers are registered
}
