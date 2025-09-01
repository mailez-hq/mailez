package auth

// OIDCConfig carries the federated sign-in (SSO) settings from the runtime
// configuration into the auth manager. It lives here (not in core.Config)
// because core imports auth for the session middleware — the reverse
// dependency would be an import cycle. The server assembles the struct from
// core.Config.
//
// The feature itself is enterprise-only: oidc_ee.go (build tag mailez_ee)
// mounts the /sso/oidc/start and /sso/oidc/callback routes, while
// oidc_ce.go provides no-op fallbacks so the community build reports the
// feature as unavailable.
type OIDCConfig struct {
	// Issuer is the OIDC provider root URL; discovery is read from
	// <Issuer>/.well-known/openid-configuration.
	Issuer string
	// ClientID / ClientSecret are the OAuth2 client credentials registered
	// at the provider (token endpoint auth: client_secret_post).
	ClientID     string
	ClientSecret string
	// RedirectURL is the redirect_uri registered at the provider, e.g.
	// https://mail.example.com/api/v1/sso/oidc/callback.
	RedirectURL string
}
