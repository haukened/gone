package envelope

import (
	"net/url"
	"strings"

	"github.com/haukened/gone/v3/internal/domain"
)

// ManageLink is a parsed sender manage link (protocol §7.3). It authorizes
// checking or revoking a secret and cannot decrypt it.
type ManageLink struct {
	// Origin is scheme://host[:port], with no path, user info, query or fragment.
	Origin string
	// ID is the secret ID.
	ID domain.SecretID
	// Token is the manage bearer token. Send it only in X-Gone-Manage.
	Token domain.ManageToken
}

// String renders the link as <origin>/manage/<id>#<token>.
//
// Returns the link text.
func (l ManageLink) String() string {
	return l.Origin + "/manage/" + l.ID.String() + "#" + l.Token.String()
}

// NewManageLink validates the parts and builds a manage link.
//
// Parameters:
//   - origin: absolute http(s) origin, e.g. "https://gone.example".
//   - id: secret ID returned by the server.
//   - token: manage token returned by the server.
//
// Returns the link or ErrInvalidManageLink.
func NewManageLink(origin string, id domain.SecretID, token string) (ManageLink, error) {
	u, err := url.Parse(origin)
	if err != nil || !isBareOrigin(u) || !id.Valid() {
		return ManageLink{}, ErrInvalidManageLink
	}
	t, err := domain.ParseManageToken(token)
	if err != nil {
		return ManageLink{}, ErrInvalidManageLink
	}
	return ManageLink{Origin: u.Scheme + "://" + u.Host, ID: id, Token: t}, nil
}

// ParseManageLink parses an absolute manage link. The path must be exactly
// /manage/<id> and the raw (still percent-encoded) fragment must be a valid
// manage token. Nothing is sent anywhere; callers validate before any request.
//
// Parameters:
//   - raw: the full manage link.
//
// Returns the link or ErrInvalidManageLink.
func ParseManageLink(raw string) (ManageLink, error) {
	u, err := parseManageLinkURL(raw)
	if err != nil {
		return ManageLink{}, ErrInvalidManageLink
	}
	idStr, ok := strings.CutPrefix(u.EscapedPath(), "/manage/")
	id, idErr := domain.ParseID(idStr)
	tok, tokErr := domain.ParseManageToken(u.EscapedFragment())
	if !ok || idErr != nil || tokErr != nil {
		return ManageLink{}, ErrInvalidManageLink
	}
	return ManageLink{Origin: u.Scheme + "://" + u.Host, ID: id, Token: tok}, nil
}

// parseManageLinkURL parses and validates the URL-level manage-link shape.
//
// Parameters:
//   - raw: full manage link.
//
// Returns the parsed URL or ErrInvalidManageLink.
func parseManageLinkURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || !isOrigin(u) || u.RawQuery != "" || u.ForceQuery {
		return nil, ErrInvalidManageLink
	}
	return u, nil
}
