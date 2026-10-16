package httpx

import (
	"net/http"

	"github.com/haukened/gone/v3/internal/domain"
)

// claimHeader returns the X-Gone-Claim request header. Like the manage and
// fill tokens it must appear at most once: a repeated header is an invalid
// claim token, so it can never be mistaken for "no claim" (a fresh claim) or
// resolve to whichever copy a proxy happened to put first.
//
// Parameters:
//   - r: incoming request.
//
// Returns the token ("" when the header is absent), or domain.ErrInvalidClaim
// when it is repeated.
func claimHeader(r *http.Request) (string, error) {
	vals := r.Header.Values(HeaderClaim)
	switch len(vals) {
	case 0:
		return "", nil
	case 1:
		return vals[0], nil
	}
	return "", domain.ErrInvalidClaim
}
