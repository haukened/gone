package httpx

import "strings"

// splitRoutePath splits an API path of the form prefix+"{id}[/{action}]" into
// its ID and action. A trailing slash after the ID is not part of the route:
// "/api/secret/{id}/" is a different (unrouted) path from "/api/secret/{id}",
// as "/api/secret/{id}/status/" is from ".../status".
//
// Parameters:
//   - path: request URL path.
//   - prefix: route prefix including both slashes, e.g. "/api/secret/".
//
// Returns the ID and action ("" for none), and false when path lacks the
// prefix, has nothing after it, or ends in a slash after the ID.
func splitRoutePath(path, prefix string) (id, action string, ok bool) {
	rest, found := strings.CutPrefix(path, prefix)
	if !found || rest == "" {
		return "", "", false
	}
	id, action, _ = strings.Cut(rest, "/")
	if action == "" && strings.HasSuffix(rest, "/") {
		return "", "", false
	}
	return id, action, true
}
