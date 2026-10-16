package integration_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/envelope"
)

// fragmentCase parses s with ParseFragment and crypto.js parseFragment.
//
// Parameters:
//   - name: case name.
//   - s: the raw fragment, without '#'.
//
// Returns the case.
func fragmentCase(name, s string) diffCase {
	return diffCase{
		name: name,
		goV: func(*testing.T) verdict {
			f, err := envelope.ParseFragment(s)
			return goVerdict(fmt.Sprintf("v%d %x", f.Version(), f.Key()), err)
		},
		js:      diffReq{Op: "fragment", Fragment: s},
		jsValue: func(_ *testing.T, r diffReply) string { return fmt.Sprintf("v%d %x", r.Version, r.Key) },
	}
}

// replyFragmentCase parses s with ParseReplyFragment and cryptoV3.js
// parseReplyFragment.
//
// Parameters:
//   - name: case name.
//   - s: the raw fragment, without '#'.
//
// Returns the case.
func replyFragmentCase(name, s string) diffCase {
	return diffCase{
		name: name,
		goV: func(*testing.T) verdict {
			f, err := envelope.ParseReplyFragment(s)
			return goVerdict(fmt.Sprintf("%x %s", f.PublicKey, f.Fill), err)
		},
		js:      diffReq{Op: "replyFragment", Fragment: s},
		jsValue: func(_ *testing.T, r diffReply) string { return fmt.Sprintf("%x %s", r.Key, r.Fill) },
	}
}

// fragmentCases are secret-link fragment boundaries (docs/protocol.md §7).
//
// Returns the cases.
func fragmentCases() []diffCase {
	key := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	pad := func(prefix string, n int) string { return prefix + strings.Repeat("A", n-len(prefix)) }
	return []diffCase{
		fragmentCase("v1 valid", "v1:"+key),
		fragmentCase("v2 valid", "v2:"+key),
		fragmentCase("percent-encoded payload", "v1:%41"+key[2:]),
		fragmentCase("percent-encoded colon", "v1%3A"+key),
		fragmentCase("second #", "v1:"+key+"#"),
		fragmentCase("second # inside", "v1:"+key[:20]+"#"+key[20:]),
		fragmentCase("512 chars, unsupported version", pad("v9:", 512)),
		fragmentCase("513 chars, unsupported version", pad("v9:", 513)),
		fragmentCase("512 chars, v1", pad("v1:", 512)),
		fragmentCase("version 01", "v01:"+key),
		fragmentCase("version 0", "v0:"+key),
		fragmentCase("version 3", "v3:"+key),
		fragmentCase("version 255", "v255:"+key),
		fragmentCase("version 256", "v256:"+key),
		fragmentCase("payload 42 chars", "v1:"+key[:42]),
		fragmentCase("payload 44 chars", "v1:"+key+"A"),
		fragmentCase("payload non-canonical last char", "v1:"+key[:42]+"B"),
		fragmentCase("payload padded", "v1:"+key+"="),
		fragmentCase("payload standard alphabet", "v1:"+key[:41]+"+/"),
		fragmentCase("empty payload", "v1:"),
	}
}

// replyFragmentCases are reply-link fragment boundaries (docs/protocol.md
// §7.4).
//
// Parameters:
//   - t: the test.
//
// Returns the cases.
func replyFragmentCases(t *testing.T) []diffCase {
	pubR := newV3Keys(t).req.PublicKey().Bytes()
	pub := base64.RawURLEncoding.EncodeToString(pubR)
	fill := strings.Repeat("A", 43)
	dot := func(prefix string, n int) string {
		return prefix + strings.Repeat("A", 300) + "." + strings.Repeat("A", n-len(prefix)-301)
	}
	cases := []diffCase{
		replyFragmentCase("reply valid", "v3:"+pub+"."+fill),
		replyFragmentCase("reply two dots", "v3:"+pub+"."+fill+"."+fill),
		replyFragmentCase("reply dot percent-encoded", "v3:"+pub+"%2E"+fill),
		replyFragmentCase("reply percent-encoded key", "v3:%42"+pub[1:]+"."+fill),
		replyFragmentCase("reply second #", "v3:"+pub+"."+fill+"#"),
		replyFragmentCase("reply 512 chars, unsupported version", dot("v4:", 512)),
		replyFragmentCase("reply 513 chars, unsupported version", dot("v4:", 513)),
		replyFragmentCase("reply version 03", "v03:"+pub+"."+fill),
		replyFragmentCase("reply version 1", "v1:"+pub+"."+fill),
		replyFragmentCase("reply version 256", "v256:"+pub+"."+fill),
		replyFragmentCase("reply fill 42 chars", "v3:"+pub+"."+fill[:42]),
		replyFragmentCase("reply fill 44 chars", "v3:"+pub+"."+fill+"A"),
		replyFragmentCase("reply fill non-canonical last char", "v3:"+pub+"."+fill[:42]+"B"),
		replyFragmentCase("reply key compressed", "v3:"+base64.RawURLEncoding.EncodeToString(withByte(pubR, 0, 2)[:33])+"."+fill),
		replyFragmentCase("reply key hybrid prefix 0x06", "v3:"+base64.RawURLEncoding.EncodeToString(withByte(pubR, 0, 6))+"."+fill),
		replyFragmentCase("reply key non-canonical last char", "v3:"+pub[:86]+"B."+fill),
	}
	// ParseReplyFragment checks the version before the payload grammar;
	// parseReplyFragment matches the whole grammar first. Both reject.
	for _, s := range []string{"v4:" + pub, "v4:%%"} {
		c := replyFragmentCase("reply version 4 bad payload "+s[:5]+" (Go: unsupported, JS: invalid)", s)
		c.anyReject = true
		cases = append(cases, c)
	}
	return cases
}

// TestDifferentialFragments feeds the same fragments to the Go and JS
// fragment parsers and fails when they disagree.
//
// Parameters:
//   - t: the test.
func TestDifferentialFragments(t *testing.T) {
	runDiff(t, append(fragmentCases(), replyFragmentCases(t)...))
}
