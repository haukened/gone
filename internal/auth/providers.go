package auth

import (
	"sort"
	"strings"

	"github.com/go-jose/go-jose/v4"
)

// spec describes one provider: the header it reads and, for signed
// providers, how to find its keys, issuer and audience.
type spec struct {
	signed   bool
	header   string // fixed header; "" means GONE_AUTH_HEADER
	alg      jose.SignatureAlgorithm
	required option
	allowed  option // includes required
	resolve  func(Settings) endpoints
}

// endpoints are a signed provider's key URL and expected claims. With
// discover set, keysURL is the issuer's OpenID discovery document.
type endpoints struct {
	keysURL  string
	issuer   string
	audience string
	discover bool
}

// specs is every provider GONE_AUTH accepts besides "none".
var specs = map[string]spec{
	"cloudflare": {
		signed: true, header: "Cf-Access-Jwt-Assertion", alg: jose.RS256,
		required: optTeam | optAudience, allowed: optTeam | optAudience,
		resolve: func(s Settings) endpoints {
			domain := cloudflareDomain(s.Team)
			return endpoints{keysURL: "https://" + domain + "/cdn-cgi/access/certs", issuer: "https://" + domain, audience: s.Audience}
		},
	},
	"google-iap": {
		signed: true, header: "X-Goog-Iap-Jwt-Assertion", alg: jose.ES256,
		required: optAudience, allowed: optAudience,
		resolve: func(s Settings) endpoints {
			return endpoints{keysURL: "https://www.gstatic.com/iap/verify/public_key-jwk", issuer: "https://cloud.google.com/iap", audience: s.Audience}
		},
	},
	"pomerium": {
		signed: true, header: "X-Pomerium-Jwt-Assertion", alg: jose.ES256,
		required: optHost, allowed: optHost | optKeysURL,
		resolve: func(s Settings) endpoints {
			return endpoints{keysURL: orDefault(s.KeysURL, "https://"+s.Host+"/.well-known/pomerium/jwks.json"), issuer: s.Host, audience: s.Host}
		},
	},
	"teleport": {
		signed: true, header: "Teleport-Jwt-Assertion", alg: jose.RS256,
		required: optHost | optIssuer | optAudience, allowed: optHost | optIssuer | optAudience,
		resolve: func(s Settings) endpoints {
			return endpoints{keysURL: "https://" + s.Host + "/.well-known/jwks.json", issuer: s.Issuer, audience: s.Audience}
		},
	},
	oidcKey: {
		signed: true, alg: jose.RS256,
		required: optIssuer | optAudience, allowed: optIssuer | optAudience | optHeader | optKeysURL | optAlg,
		resolve: func(s Settings) endpoints {
			if s.KeysURL != "" {
				return endpoints{keysURL: s.KeysURL, issuer: s.Issuer, audience: s.Audience}
			}
			return endpoints{keysURL: strings.TrimSuffix(s.Issuer, "/") + "/.well-known/openid-configuration", issuer: s.Issuer, audience: s.Audience, discover: true}
		},
	},
	"authelia":  {header: "Remote-User"},
	"authentik": {header: "X-Authentik-Username"},
	"tailscale": {header: "Tailscale-User-Login"},
	Custom:      {required: optHeader, allowed: optHeader | optFormat | optValue},
}

// Providers lists the accepted GONE_AUTH values, sorted, for messages.
//
// Returns the provider keys, including "none".
func Providers() []string {
	keys := []string{None}
	for k := range specs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// cloudflareDomain turns GONE_AUTH_TEAM into the team's Access domain.
//
// Parameters:
//   - team: a team name ("acme") or domain ("acme.cloudflareaccess.com").
//
// Returns the domain, or "" for an empty team.
func cloudflareDomain(team string) string {
	if team == "" || strings.Contains(team, ".") {
		return team
	}
	return team + ".cloudflareaccess.com"
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
