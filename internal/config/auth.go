package config

import (
	"fmt"

	"github.com/haukened/gone/v3/internal/auth"
)

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

// kubernetesPlainHeaderWarning is logged when a plain-header provider runs
// on Kubernetes, where proxy pod addresses change and trusting the pod
// network trusts every pod in the cluster.
const kubernetesPlainHeaderWarning = "GONE_AUTH=%s on Kubernetes: any pod that can reach Gone can forge this header unless the proxy runs in Gone's own pod and GONE_TRUSTED_PROXIES is 127.0.0.1 only; use a signed provider. See docs/deploy/kubernetes.md#sign-in"

// authWarnings returns the sign-in startup warnings, such as a provider
// without GONE_TRUSTED_PROXIES, or a plain-header provider on Kubernetes.
//
// Parameters:
//   - cfg: the validated configuration.
//   - onKubernetes: whether Gone runs in a Kubernetes pod
//     (KUBERNETES_SERVICE_HOST is set).
//
// Returns the warnings, nil when there are none.
func authWarnings(cfg *Config, onKubernetes bool) []string {
	s := cfg.AuthSettings()
	w, _ := s.Validate(len(cfg.TrustedPrefixes) > 0)
	if onKubernetes && s.PlainHeader() {
		w = append(w, fmt.Sprintf(kubernetesPlainHeaderWarning, s.Provider))
	}
	return w
}
