package ratelimit

import (
	"net/netip"
	"strings"
)

// maxForwardedHops bounds how many X-Forwarded-For entries are examined, so a
// huge header cannot make key derivation expensive.
const maxForwardedHops = 16

// unknownKey groups requests whose peer address cannot be parsed.
var unknownKey = netip.PrefixFrom(netip.IPv6Unspecified(), 128)

// ClientKey derives the rate-limit key for a request.
//
// The connection's peer address is used unless that peer is a trusted proxy.
// For a trusted peer, X-Forwarded-For entries are walked from right to left,
// skipping trusted hops; the first untrusted address is the client. If an
// entry cannot be parsed, the last trusted hop is used instead, which groups
// such requests under the proxy rather than letting a client choose its key.
// IPv4 clients are keyed by /32 and IPv6 clients by /64.
//
// Parameters:
//   - remoteAddr: http.Request.RemoteAddr ("ip:port").
//   - xff: all X-Forwarded-For header values, in received order.
//   - trusted: proxy prefixes whose X-Forwarded-For is honored.
//
// Returns:
//   - netip.Prefix: the masked client key.
//   - bool: true when X-Forwarded-For was present but ignored because the
//     peer is not trusted or could not be parsed.
func ClientKey(remoteAddr string, xff []string, trusted []netip.Prefix) (netip.Prefix, bool) {
	peer, ok := parseHop(remoteAddr)
	if !ok {
		return unknownKey, len(xff) > 0
	}
	if !isTrusted(peer, trusted) {
		return maskClient(peer), len(xff) > 0
	}
	return maskClient(pickClient(peer, forwardedFromRight(xff), trusted)), false
}

// parseHop parses an address with or without a port, removing any IPv6 zone
// and unmapping IPv4-mapped IPv6 addresses.
//
// Parameters:
//   - s: "ip", "ip:port", or "[ipv6]:port".
//
// Returns:
//   - netip.Addr: the normalized address.
//   - bool: false when s is not an address.
func parseHop(s string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		ap, perr := netip.ParseAddrPort(s)
		if perr != nil {
			return netip.Addr{}, false
		}
		addr = ap.Addr()
	}
	return addr.WithZone("").Unmap(), true
}

// isTrusted reports whether addr belongs to any trusted prefix.
//
// Parameters:
//   - addr: a normalized address.
//   - trusted: trusted proxy prefixes.
//
// Returns:
//   - bool: true when addr is inside a trusted prefix.
func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// forwardedFromRight returns up to maxForwardedHops X-Forwarded-For entries,
// nearest hop first. Header values are read from last to first, and the
// entries within each value from right to left.
//
// Parameters:
//   - xff: X-Forwarded-For header values in received order.
//
// Returns:
//   - []string: trimmed entries, nearest hop first.
func forwardedFromRight(xff []string) []string {
	hops := make([]string, 0, maxForwardedHops)
	for i := len(xff) - 1; i >= 0 && len(hops) < maxForwardedHops; i-- {
		rest := xff[i]
		for len(hops) < maxForwardedHops {
			idx := strings.LastIndexByte(rest, ',')
			hops = append(hops, strings.TrimSpace(rest[idx+1:]))
			if idx < 0 {
				break
			}
			rest = rest[:idx]
		}
	}
	return hops
}

// pickClient walks forwarded hops, nearest first, and returns the first
// address that is not a trusted proxy. An unparsable hop stops the walk and
// the last trusted address is returned.
//
// Parameters:
//   - peer: the trusted connection peer.
//   - hops: X-Forwarded-For entries, nearest hop first.
//   - trusted: trusted proxy prefixes.
//
// Returns:
//   - netip.Addr: the client address.
func pickClient(peer netip.Addr, hops []string, trusted []netip.Prefix) netip.Addr {
	client := peer
	for _, hop := range hops {
		addr, ok := parseHop(hop)
		if !ok {
			return client
		}
		client = addr
		if !isTrusted(addr, trusted) {
			return addr
		}
	}
	return client
}

// maskClient converts an address into its rate-limit key: /32 for IPv4 and
// /64 for IPv6.
//
// Parameters:
//   - addr: a valid, normalized address.
//
// Returns:
//   - netip.Prefix: the masked key.
func maskClient(addr netip.Addr) netip.Prefix {
	bits := 64
	if addr.Is4() {
		bits = 32
	}
	return netip.PrefixFrom(addr, bits).Masked()
}
