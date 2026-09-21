package generic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/features/auth/oauth/factory"
	"github.com/komari-monitor/komari/pkg/mask"
	"github.com/komari-monitor/komari/pkg/random"
	"github.com/patrickmn/go-cache"
)

func (g *Generic) GetName() string {
	return "generic"
}
func (g *Generic) GetConfiguration() factory.Configuration {
	return &g.Addition
}

func (g *Generic) GetAuthorizationURL(redirectURI string) (string, string) {
	state := random.String(16)

	// 构建GitHub OAuth授权URL
	authURL := fmt.Sprintf(
		"%s?client_id=%s&state=%s&scope=%s&redirect_uri=%s&response_type=code",
		g.Addition.AuthURL,
		url.QueryEscape(g.Addition.ClientId),
		url.QueryEscape(state),
		url.QueryEscape(g.Addition.Scope),
		url.QueryEscape(redirectURI),
	)
	g.stateCache.Set(state, true, cache.DefaultExpiration)
	return authURL, state
}
func (g *Generic) OnCallback(ctx *gin.Context, state string, query map[string]string, callbackURI string) (factory.OidcCallback, error) {
	code := query["code"]

	// 验证state防止CSRF攻击
	if g.stateCache == nil {
		return factory.OidcCallback{}, fmt.Errorf("state cache not initialized")
	}
	if _, ok := g.stateCache.Get(state); !ok {
		return factory.OidcCallback{}, fmt.Errorf("invalid state")
	}
	if state == "" {
		return factory.OidcCallback{}, fmt.Errorf("invalid state")
	}
	// state 只允许使用一次，验证通过后立即失效，防止回调 URL 被重放。
	g.stateCache.Delete(state)

	// 获取code
	if code == "" {
		return factory.OidcCallback{}, fmt.Errorf("no code provided")
	}

	// 获取访问令牌
	data := url.Values{
		"client_id":     {g.Addition.ClientId},
		"client_secret": {g.Addition.ClientSecret},
		"code":          {code},
		"redirect_uri":  {callbackURI},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequest("POST", g.Addition.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return factory.OidcCallback{}, fmt.Errorf("invalid token url: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return factory.OidcCallback{}, fmt.Errorf("failed to get access token: %s", mask.DataMasking(err.Error(), []string{g.Addition.ClientSecret, g.Addition.ClientId}))
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return factory.OidcCallback{}, fmt.Errorf("failed to parse access token response: %s", mask.DataMasking(err.Error(), []string{g.Addition.ClientSecret, g.Addition.ClientId}))
	}

	// 获取用户信息
	userReq, err := http.NewRequest("GET", g.Addition.UserInfoURL, nil)
	if err != nil {
		return factory.OidcCallback{}, fmt.Errorf("invalid user info url: %w", err)
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		return factory.OidcCallback{}, fmt.Errorf("failed to get user info: %v", err)
	}
	defer userResp.Body.Close()

	var user map[string]any
	if err := json.NewDecoder(userResp.Body).Decode(&user); err != nil {
		return factory.OidcCallback{}, fmt.Errorf("failed to parse user info response: %v", err)
	}

	userId, ok := user[g.Addition.UserIDField]
	if !ok {
		return factory.OidcCallback{}, fmt.Errorf("user id field '%s' not found in user info response", g.Addition.UserIDField)
	}

	return factory.OidcCallback{UserId: fmt.Sprintf("%v", userId)}, nil
}
func (g *Generic) Init() error {
	g.stateCache = cache.New(time.Minute*5, time.Minute*10)
	return nil
}
func (g *Generic) Destroy() error {
	g.stateCache.Flush()
	return nil
}

var _ factory.IOidcProvider = (*Generic)(nil)
