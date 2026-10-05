package envelope

import (
	"strings"

	"github.com/haukened/gone/v3/internal/domain"
)

type fragmentCase struct {
	Input   string `json:"input"`
	Version int    `json:"version,omitempty"`
	Key     string `json:"key,omitempty"`
	Error   string `json:"error,omitempty"`
}

// fragmentInputs returns protocol v1 fragment vector and fuzz seed inputs.
//
// Returns:
//   - []string: the fragment input strings.
func fragmentInputs() []string {
	k := domain.EncodeB64URL(seq(0, domain.KeySize))
	zeros := strings.Repeat("A", 43)
	return []string{
		"v1:" + k, "v1:" + zeros, "v1:" + strings.Repeat("_", 42) + "w",
		"", "v1:", "v1" + k, "#v1:" + k, "V1:" + k, "x1:" + k, "v:" + k,
		"v0:" + k, "v01:" + k, "v+1:" + k, "v1.0:" + k, "v256:" + k, "v1000:" + k,
		"v3:" + k, "v255:" + k,
		"v1:" + k + "=", "v1:" + strings.Repeat("A", 42) + "B", "v1:" + zeros[:42], "v1:" + zeros + "A",
		"v1:" + zeros[:41] + "+/", "v1:" + zeros[:42] + "%", "v1%3A" + k, "%761:" + k, "v1:" + k + "#", "v1:" + k + " ",
		"v1:" + domain.EncodeB64URL(seq(0, 31)), "v1:" + domain.EncodeB64URL(seq(0, 33)),
	}
}

// buildFragments computes fragment vector outputs for each input.
//
// Parameters:
//   - ins: the fragment inputs to parse.
//
// Returns:
//   - []fragmentCase: the completed fragment vector cases.
func buildFragments(ins []string) []fragmentCase {
	out := make([]fragmentCase, len(ins))
	for i, s := range ins {
		c := fragmentCase{Input: s}
		f, err := ParseFragment(s)
		if err != nil {
			c.Error = errCode(err)
		} else {
			c.Version, c.Key = int(f.Version()), hx(f.Key())
		}
		out[i] = c
	}
	return out
}

type stringCase struct {
	In  string `json:"in"`
	Out string `json:"out"`
}

type sanitizeVectors struct {
	Names []stringCase `json:"names"`
	Types []stringCase `json:"types"`
}

// buildSanitize computes file-name and MIME-type sanitizer vectors.
//
// Returns:
//   - sanitizeVectors: the sanitizer vector cases.
func buildSanitize() sanitizeVectors {
	v := sanitizeVectors{}
	for _, n := range sanitizeNameInputs() {
		v.Names = append(v.Names, stringCase{n, SanitizeFileName(n)})
	}
	for _, ty := range sanitizeTypeInputs() {
		v.Types = append(v.Types, stringCase{ty, SafeType(ty)})
	}
	return v
}

// sanitizeNameInputs returns file names used by sanitizer vectors.
//
// Returns:
//   - []string: the file-name sanitizer inputs.
func sanitizeNameInputs() []string {
	return []string{
		"report.pdf", "", ".", "..", "../../etc/passwd", `..\..\x`, `C:\Users\a\evil.exe`, "dir/", "/", "a/..",
		"  name.txt \t", "\u00a0\u3000x\ufeff", "a\x00b\x1fc\x7fd\u0085e", "invoice\u202etxt.exe",
		"a\u200bb\u200fc\u2066d\u206fe\u061cf\u180eg", "\u200b..\u200b", "\u2028x\u2029", "\u1680\u205fy\u202f",
		strings.Repeat("é", 300), strings.Repeat("😀", 300), strings.Repeat("\u200b", 300) + "ok",
		strings.Repeat("a", 254) + " b", "日本語.txt", "\ufffd",
	}
}

// sanitizeTypeInputs returns MIME types used by sanitizer vectors.
//
// Returns:
//   - []string: the MIME-type sanitizer inputs.
func sanitizeTypeInputs() []string {
	return []string{
		"application/pdf", "Image/PNG", " text/plain ; charset=utf-8", "text/plain;", "text/html",
		"image/svg+xml", "", "application/x-7z-compressed", "text/plaın", "TEXT/PLAİN", "\u00a0text/csv\u3000",
		"text/csv\x00", "\ttext/csv\n", "video/mp4;codecs=avc1",
	}
}

type acceptCase struct {
	In string `json:"in"`
	OK bool   `json:"ok"`
}

type serverVectors struct {
	Versions []acceptCase `json:"versions"`
	Nonces   []acceptCase `json:"nonces"`
}

// buildServer computes protocol header acceptance vectors.
//
// Returns:
//   - serverVectors: the server header vector cases.
func buildServer() serverVectors {
	v := serverVectors{}
	for _, s := range versionHeaderInputs() {
		_, err := domain.ParseVersion(s)
		v.Versions = append(v.Versions, acceptCase{s, err == nil})
	}
	for _, s := range nonceHeaderInputs() {
		ok := domain.ValidateProtocol(domain.ProtocolV1, s) == nil
		v.Nonces = append(v.Nonces, acceptCase{s, ok})
	}
	return v
}

// versionHeaderInputs returns protocol version header vector inputs.
//
// Returns:
//   - []string: the version header inputs.
func versionHeaderInputs() []string {
	return []string{"1", "2", "3", "0", "01", "+1", "-1", " 1", "1 ", "1.0", "256", "1000", "", "a", "１"}
}

// nonceHeaderInputs returns nonce header vector inputs.
//
// Returns:
//   - []string: the nonce header inputs.
func nonceHeaderInputs() []string {
	return []string{
		"AAAAAAAAAAAAAAAA", "oKGio6Slpqeoqaqr", "____________----", "AAAAAAAAAAAAAAA", "AAAAAAAAAAAAAAAAA",
		"AAAAAAAAAAAAAAA=", "AAAAAAAAAAAAAAA+", "AAAAAAAAAAAAAAA/", "AAAAAAAAAAAAAAA ", "", "AAAAAAAAAAAAAAAAAAAA",
	}
}
