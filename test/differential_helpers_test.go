package integration_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/haukened/gone/v3/internal/domain"
	"github.com/haukened/gone/v3/internal/envelope"
)

// verdict is one implementation's answer to a differential case: the value
// it accepted the input as, or the class of error it rejected it with.
type verdict struct {
	ok    bool
	value string
	class string
}

// String renders the verdict for failure messages.
//
// Returns "accept <value>" or "reject <class>".
func (v verdict) String() string {
	if v.ok {
		return "accept " + v.value
	}
	return "reject " + v.class
}

// diffReq is a differential request to the interop bridge.
type diffReq struct {
	Op         string `json:"op"`
	Version    uint8  `json:"version,omitempty"`
	Key        string `json:"key,omitempty"`
	Fragment   string `json:"fragment,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Message    string `json:"message,omitempty"`
	Private    string `json:"private,omitempty"`
	Nonce      []byte `json:"nonce,omitempty"`
	Body       []byte `json:"body,omitempty"`
}

// diffReply is the union of the bridge replies the differential ops return.
type diffReply struct {
	Message string   `json:"message"`
	Files   []jsFile `json:"files"`
	Version uint8    `json:"version"`
	Key     []byte   `json:"key"`
	Fill    string   `json:"fill"`
	Nonce   []byte   `json:"nonce"`
	Body    []byte   `json:"body"`
}

// diffCase feeds one input to both implementations.
type diffCase struct {
	// name identifies the case.
	name string
	// goV computes the Go implementation's verdict.
	goV func(t *testing.T) verdict
	// js is the same input as a bridge request.
	js diffReq
	// jsValue renders an accepted bridge reply like goV renders its value.
	jsValue func(t *testing.T, r diffReply) string
	// anyReject compares only accept versus reject, not the error class,
	// for inputs where the two parsers check the same rules in a different
	// order (recorded in the case name).
	anyReject bool
}

// jsClasses maps the browser code's error messages onto the classes Go's
// errors are mapped to; coded errors already use the class name.
var jsClasses = map[string]string{
	"truncated envelope":                "truncated",
	"invalid header length":             "length",
	"ERR_ENCODING_INVALID_ENCODED_DATA": "json",
	"invalid envelope header":           "shape",
	"too many files":                    "count",
	"invalid file entry":                "entry",
	"envelope size mismatch":            "sum",
}

// goEnvelopeClasses maps the reasons of Go's ErrInvalidEnvelope to classes.
var goEnvelopeClasses = map[string]string{
	"truncated": "truncated", "header length": "length", "header json": "json",
	"version": "shape", "msg size": "shape", "files": "shape",
	"file entry": "entry", "size mismatch": "sum",
}

// goSentinels maps Go's sentinel errors to the browser code's error codes.
var goSentinels = []struct {
	err   error
	class string
}{
	{envelope.ErrTooManyFiles, "count"},
	{envelope.ErrInvalidFragment, "invalid_fragment"},
	{domain.ErrInvalidVersion, "unsupported_version"},
	{envelope.ErrMalformed, "malformed"},
	{envelope.ErrInvalidPassphrase, "invalid_passphrase"},
	{envelope.ErrDecrypt, "decrypt"},
	{envelope.ErrInvalidKey, "invalid_key"},
}

// goClass returns the class of a Go error.
//
// Parameters:
//   - err: an error from internal/envelope.
//
// Returns the class, or the error text when it is not a known error.
func goClass(err error) string {
	if errors.Is(err, envelope.ErrInvalidEnvelope) {
		_, reason, _ := strings.Cut(err.Error(), envelope.ErrInvalidEnvelope.Error()+": ")
		return classOr(goEnvelopeClasses, reason)
	}
	for _, s := range goSentinels {
		if errors.Is(err, s.err) {
			return s.class
		}
	}
	return err.Error()
}

// classOr looks key up in classes, falling back to key itself.
//
// Parameters:
//   - classes: known names.
//   - key: an error reason or message.
//
// Returns the class.
func classOr(classes map[string]string, key string) string {
	if c, ok := classes[key]; ok {
		return c
	}
	return key
}

// goVerdict turns a Go result into a verdict.
//
// Parameters:
//   - value: the accepted value, rendered.
//   - err: the error, or nil.
//
// Returns the verdict.
func goVerdict(value string, err error) verdict {
	if err != nil {
		return verdict{class: goClass(err)}
	}
	return verdict{ok: true, value: value}
}

// jsVerdict runs a case's request through the browser code.
//
// Parameters:
//   - t: the test.
//   - c: the case.
//
// Returns the browser code's verdict.
func jsVerdict(t *testing.T, c diffCase) verdict {
	t.Helper()
	var r diffReply
	if code := runBridge(t, c.js, &r); code != "" {
		return verdict{class: classOr(jsClasses, code)}
	}
	return verdict{ok: true, value: c.jsValue(t, r)}
}

// runDiff runs every case against both implementations in parallel and
// fails on any disagreement.
//
// Parameters:
//   - t: the test.
//   - cases: the cases.
func runDiff(t *testing.T, cases []diffCase) {
	t.Helper()
	requireNode(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			g, j := c.goV(t), jsVerdict(t, c)
			t.Logf("Go: %v; JS: %v", g, j)
			if !sameVerdict(g, j, c.anyReject) {
				t.Errorf("Go and JS disagree:\n  Go: %v\n  JS: %v", g, j)
			}
		})
	}
}

// sameVerdict reports whether two verdicts agree: both accept the same
// value, or both reject with the same class (any class when anyReject).
//
// Parameters:
//   - g: the Go verdict.
//   - j: the JS verdict.
//   - anyReject: whether rejection classes may differ.
//
// Returns true when they agree.
func sameVerdict(g, j verdict, anyReject bool) bool {
	if g.ok != j.ok {
		return false
	}
	return anyReject && !g.ok || g == j
}

// payloadValue renders a decoded payload: message, then each file's
// sanitized name, type and size.
//
// Parameters:
//   - msg: message text.
//   - files: the files.
//
// Returns the rendering.
func payloadValue(msg string, files []jsFile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "msg=%q", msg)
	for _, f := range files {
		fmt.Fprintf(&b, " file=%q|%q|%d", f.Name, f.Type, len(f.Data))
	}
	return b.String()
}

// jsPayload renders a bridge reply that carries a decoded payload.
//
// Parameters:
//   - t: the test (unused; matches diffCase.jsValue).
//   - r: the reply.
//
// Returns the rendering.
func jsPayload(_ *testing.T, r diffReply) string {
	return payloadValue(r.Message, r.Files)
}

// goPayload decodes plaintext with Go and renders it like jsPayload.
//
// Parameters:
//   - pt: plaintext.
//   - err: an error that came before decoding, or nil.
//
// Returns the verdict.
func goPayload(pt []byte, err error) verdict {
	if err != nil {
		return goVerdict("", err)
	}
	p, err := envelope.Unpack(pt)
	files := make([]jsFile, len(p.Files))
	for i, f := range p.Files {
		files[i] = jsFile{Name: f.Name, Type: f.Type, Data: f.Data}
	}
	return goVerdict(payloadValue(string(p.Message), files), err)
}
