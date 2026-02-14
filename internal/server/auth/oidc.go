package auth

import "net/url"

// OIDCConfig holds OIDC SSO configuration.
type OIDCConfig struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
}

// OIDCProvider manages OIDC authentication flow.
type OIDCProvider struct {
	Config OIDCConfig
}

// NewOIDCProvider creates a new OIDC provider.
// Full implementation requires github.com/coreos/go-oidc — stubbed for now.
func NewOIDCProvider(cfg OIDCConfig) *OIDCProvider {
	return &OIDCProvider{Config: cfg}
}

// AuthorizationURL returns the URL to redirect the user to for SSO login.
func (p *OIDCProvider) AuthorizationURL(state string) string {
	params := url.Values{}
	params.Set("client_id", p.Config.ClientID)
	params.Set("redirect_uri", p.Config.RedirectURL)
	params.Set("response_type", "code")
	params.Set("scope", "openid email profile")
	params.Set("state", state)
	return p.Config.Issuer + "/authorize?" + params.Encode()
}
