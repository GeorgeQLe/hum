package auth

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
	return p.Config.Issuer + "/authorize?client_id=" + p.Config.ClientID +
		"&redirect_uri=" + p.Config.RedirectURL +
		"&response_type=code&scope=openid+email+profile&state=" + state
}
