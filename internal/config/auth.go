package config

import "github.com/haukened/gone/v3/internal/auth"

// AuthSettings returns the GONE_AUTH* settings for the auth package.
//
// Returns the settings.
func (c *Config) AuthSettings() auth.Settings {
	return auth.Settings{
		Provider: c.Auth, Team: c.AuthTeam, Audience: c.AuthAudience, Issuer: c.AuthIssuer,
		Host: c.AuthHost, KeysURL: c.AuthKeysURL, Alg: c.AuthAlg, Header: c.AuthHeader,
		Format: c.AuthFormat, Value: c.AuthValue, SignInURL: c.AuthSignInURL,
	}
}

// validateAuth checks the sign-in settings against the trusted proxies.
//
// Parameters:
//   - cfg: the configuration, with TrustedPrefixes derived.
//
// Returns the first invalid setting, named by its environment variable.
func validateAuth(cfg *Config) error {
	_, err := cfg.AuthSettings().Validate(len(cfg.TrustedPrefixes) > 0)
	return err
}

// authWarnings returns the sign-in startup warnings, such as a provider
// without GONE_TRUSTED_PROXIES.
//
// Parameters:
//   - cfg: the validated configuration.
//
// Returns the warnings, nil when there are none.
func authWarnings(cfg *Config) []string {
	w, _ := cfg.AuthSettings().Validate(len(cfg.TrustedPrefixes) > 0)
	return w
}
