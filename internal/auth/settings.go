// Package auth decides whether a request comes from someone the operator's
// sign-in proxy has signed in (GONE_AUTH). It never keeps the identity:
// signed tokens are verified and plain headers are checked, and the only
// answer is yes or no. See docs/sign-in.md.
package auth

import (
	"errors"
	"fmt"
	"strings"
)

// Provider keys accepted in GONE_AUTH.
const (
	None      = "none"
	Custom    = "custom"
	oidcKey   = "oidc"
	headerMax = 64 // longest accepted header name
)

// Settings are the GONE_AUTH* environment variables, trimmed.
type Settings struct {
	Provider  string // GONE_AUTH; "" means none
	Team      string // GONE_AUTH_TEAM
	Audience  string // GONE_AUTH_AUDIENCE
	Issuer    string // GONE_AUTH_ISSUER
	Host      string // GONE_AUTH_HOST
	KeysURL   string // GONE_AUTH_JWKS_URL
	Alg       string // GONE_AUTH_ALG
	Header    string // GONE_AUTH_HEADER
	Format    string // GONE_AUTH_FORMAT
	Value     string // GONE_AUTH_VALUE
	SignInURL string // GONE_AUTH_SIGNIN_URL
}

// Enabled reports whether sign-in is required.
//
// Returns true unless GONE_AUTH is empty or "none".
func (s Settings) Enabled() bool {
	return s.Provider != "" && s.Provider != None
}

// PlainHeader reports whether the provider trusts a plain header from a
// trusted proxy rather than verifying a signed token.
//
// Returns true for authelia, authentik, tailscale and custom.
func (s Settings) PlainHeader() bool {
	sp, ok := specs[s.Provider]
	return s.Enabled() && ok && !sp.signed
}

// option is one optional GONE_AUTH_* setting, as a bit.
type option uint16

const (
	optTeam option = 1 << iota
	optAudience
	optIssuer
	optHost
	optKeysURL
	optAlg
	optHeader
	optFormat
	optValue
)

// optionNames are the environment variables behind each option, in the
// order errors list them.
var optionNames = []struct {
	opt  option
	name string
	get  func(Settings) string
}{
	{optTeam, "GONE_AUTH_TEAM", func(s Settings) string { return s.Team }},
	{optAudience, "GONE_AUTH_AUDIENCE", func(s Settings) string { return s.Audience }},
	{optIssuer, "GONE_AUTH_ISSUER", func(s Settings) string { return s.Issuer }},
	{optHost, "GONE_AUTH_HOST", func(s Settings) string { return s.Host }},
	{optKeysURL, "GONE_AUTH_JWKS_URL", func(s Settings) string { return s.KeysURL }},
	{optAlg, "GONE_AUTH_ALG", func(s Settings) string { return s.Alg }},
	{optHeader, "GONE_AUTH_HEADER", func(s Settings) string { return s.Header }},
	{optFormat, "GONE_AUTH_FORMAT", func(s Settings) string { return s.Format }},
	{optValue, "GONE_AUTH_VALUE", func(s Settings) string { return s.Value }},
}

// Validate checks the settings for the chosen provider.
//
// Parameters:
//   - trusted: whether GONE_TRUSTED_PROXIES lists at least one proxy.
//
// Returns:
//   - []string: non-fatal warnings to log at startup.
//   - error: the first invalid setting, named by its environment variable.
func (s Settings) Validate(trusted bool) ([]string, error) {
	if !s.Enabled() {
		return nil, s.noneHasOptions()
	}
	sp, ok := specs[s.Provider]
	if !ok {
		return nil, fmt.Errorf("GONE_AUTH: unknown provider %q; use one of %s", s.Provider, strings.Join(Providers(), ", "))
	}
	if err := s.checkOptions(sp); err != nil {
		return nil, err
	}
	if err := s.checkValues(); err != nil {
		return nil, err
	}
	return s.trustWarnings(sp, trusted)
}

// noneHasOptions reports GONE_AUTH_* settings left over without a provider.
//
// Returns an error naming the first one, or nil.
func (s Settings) noneHasOptions() error {
	without := "without GONE_AUTH"
	if s.Provider == None {
		without = "with GONE_AUTH=none"
	}
	for _, o := range optionNames {
		if o.get(s) != "" {
			return fmt.Errorf("%s: set %s; choose a provider or remove it", o.name, without)
		}
	}
	if s.SignInURL != "" {
		return fmt.Errorf("GONE_AUTH_SIGNIN_URL: set %s; choose a provider or remove it", without)
	}
	return nil
}

// checkOptions reports a missing required option or one that doesn't apply.
//
// Parameters:
//   - sp: the chosen provider.
//
// Returns an error naming the variable, or nil.
func (s Settings) checkOptions(sp spec) error {
	for _, o := range optionNames {
		set := o.get(s) != ""
		switch {
		case sp.required&o.opt != 0 && !set:
			return fmt.Errorf("%s: required with GONE_AUTH=%s", o.name, s.Provider)
		case sp.allowed&o.opt == 0 && set:
			return fmt.Errorf("%s: does not apply to GONE_AUTH=%s; remove it", o.name, s.Provider)
		}
	}
	return nil
}

// checkValues checks the form of the options that are set.
//
// Returns an error naming the variable, or nil.
func (s Settings) checkValues() error {
	checks := []func() error{
		func() error { return checkHeaderName(s.Header) },
		func() error { return checkHTTPS("GONE_AUTH_JWKS_URL", s.KeysURL) },
		func() error { return checkIssuer(s.Provider, s.Issuer) },
		func() error { return checkHost("GONE_AUTH_HOST", s.Host) },
		func() error { return checkHost("GONE_AUTH_TEAM", cloudflareDomain(s.Team)) },
		func() error { return checkChoice("GONE_AUTH_ALG", s.Alg, "RS256", "ES256") },
		func() error { return checkChoice("GONE_AUTH_FORMAT", s.Format, "any", "email", "exact") },
		s.checkExact,
		func() error { return checkSignInURL(s.SignInURL) },
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// checkExact requires GONE_AUTH_VALUE with GONE_AUTH_FORMAT=exact, and only
// then.
//
// Returns an error naming the variable, or nil.
func (s Settings) checkExact() error {
	switch {
	case s.Format == "exact" && s.Value == "":
		return errors.New("GONE_AUTH_VALUE: required with GONE_AUTH_FORMAT=exact")
	case s.Format != "exact" && s.Value != "":
		return errors.New("GONE_AUTH_VALUE: applies only with GONE_AUTH_FORMAT=exact; remove it or set the format")
	}
	return nil
}

// trustWarnings enforces GONE_TRUSTED_PROXIES for header providers and
// returns the startup warnings for the rest.
//
// Parameters:
//   - sp: the chosen provider.
//   - trusted: whether GONE_TRUSTED_PROXIES lists a proxy.
//
// Returns the warnings, or an error for a header provider without proxies.
func (s Settings) trustWarnings(sp spec, trusted bool) ([]string, error) {
	var warnings []string
	switch {
	case !trusted && !sp.signed:
		return nil, fmt.Errorf("GONE_TRUSTED_PROXIES: required with GONE_AUTH=%s, which trusts a plain header; list the address of the proxy that sets it", s.Provider)
	case !trusted:
		warnings = append(warnings, fmt.Sprintf("GONE_AUTH=%s without GONE_TRUSTED_PROXIES: tokens are still verified, but put only your proxy in front of Gone and list it in GONE_TRUSTED_PROXIES so client addresses and rate limits are right", s.Provider))
	}
	if s.Format == "exact" && len(s.Value) < 16 {
		warnings = append(warnings, "GONE_AUTH_VALUE is shorter than 16 characters; use a long random value, for example from openssl rand -base64 24")
	}
	return warnings, nil
}
