package auth

import (
	"fmt"
	"net/url"
	"strings"
)

// checkHeaderName accepts an empty name or one of letters, digits and dashes.
// Underscores are refused because proxies such as nginx drop them.
//
// Parameters:
//   - name: GONE_AUTH_HEADER.
//
// Returns an error naming the variable, or nil.
func checkHeaderName(name string) error {
	if len(name) > headerMax {
		return fmt.Errorf("GONE_AUTH_HEADER: at most %d characters", headerMax)
	}
	if !validHeaderName(name) {
		return fmt.Errorf("GONE_AUTH_HEADER: %q may use only letters, digits and dashes", name)
	}
	return nil
}

// validHeaderName reports whether name uses only letters, digits and dashes.
func validHeaderName(name string) bool {
	for _, r := range name {
		if !headerChar(r) {
			return false
		}
	}
	return true
}

// headerChar reports a letter, digit or dash.
func headerChar(r rune) bool {
	return r == '-' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
}

// checkHTTPS accepts an empty value or an absolute https URL with a host.
//
// Parameters:
//   - name: the environment variable, for the message.
//   - raw: the value.
//
// Returns an error naming the variable, or nil.
func checkHTTPS(name, raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("%s: must be an https:// URL, got %q", name, raw)
	}
	return nil
}

// checkIssuer requires the OIDC issuer to be an https URL; other providers'
// issuers (a Teleport cluster name) are plain strings.
//
// Parameters:
//   - provider: GONE_AUTH.
//   - issuer: GONE_AUTH_ISSUER.
//
// Returns an error naming the variable, or nil.
func checkIssuer(provider, issuer string) error {
	if provider != oidcKey {
		return nil
	}
	return checkHTTPS("GONE_AUTH_ISSUER", issuer)
}

// checkHost accepts an empty value or a bare host name, optionally with a
// port: no scheme, path or user.
//
// Parameters:
//   - name: the environment variable, for the message.
//   - host: the value.
//
// Returns an error naming the variable, or nil.
func checkHost(name, host string) error {
	if host == "" {
		return nil
	}
	u, err := url.Parse("https://" + host)
	if err != nil || u.Host != host || u.User != nil {
		return fmt.Errorf("%s: must be a host name such as gone.example.com, got %q", name, host)
	}
	return nil
}

// checkChoice accepts an empty value or one of choices.
//
// Parameters:
//   - name: the environment variable, for the message.
//   - value: the setting.
//   - choices: the accepted values.
//
// Returns an error naming the variable, or nil.
func checkChoice(name, value string, choices ...string) error {
	if value == "" {
		return nil
	}
	for _, c := range choices {
		if value == c {
			return nil
		}
	}
	return fmt.Errorf("%s: use one of %s, got %q", name, strings.Join(choices, ", "), value)
}

// checkSignInURL accepts an empty value, an https URL, or a same-origin
// path starting with a single "/".
//
// Parameters:
//   - raw: GONE_AUTH_SIGNIN_URL.
//
// Returns an error naming the variable, or nil.
func checkSignInURL(raw string) error {
	if raw == "" || strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") && !strings.Contains(raw, `\`) {
		return nil
	}
	return checkHTTPS("GONE_AUTH_SIGNIN_URL", raw)
}
