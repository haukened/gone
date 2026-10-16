package integration_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// specOp is one documented operation: its path template, method, and for each
// documented status code the text of its response block (with a $ref to
// components/responses expanded), so tests can look for documented headers.
type specOp struct {
	path, method string
	codes        map[string]string
}

var (
	specPathRe   = regexp.MustCompile(`^  (/\S*):\s*$`)
	specMethodRe = regexp.MustCompile(`^    (get|post|put|delete|head|patch|options):\s*$`)
	specCodeRe   = regexp.MustCompile(`^        '?"?(\d{3})"?'?:\s*$`)
	specNameRe   = regexp.MustCompile(`^    (\w+):\s*$`)
	specRefRe    = regexp.MustCompile(`\$ref:\s*'#/components/responses/(\w+)'`)
)

// loadSpec reads docs/openapi.yaml with a line-based extractor. It understands
// only the layout the file uses (2-space indent: paths, 4: methods, 8: status
// codes), which is all the drift test needs; it is not a YAML parser.
//
// Parameters:
//   - t: test handle.
//
// Returns the documented operations in file order.
func loadSpec(t *testing.T) []specOp {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	shared := specComponentResponses(lines)
	var (
		ops           []specOp
		cur           *specOp
		inResponses   bool
		code          string
		block         strings.Builder
		inPaths       bool
		path, method  string
		flushCodeFunc = func() {
			if cur != nil && code != "" {
				text := block.String()
				if m := specRefRe.FindStringSubmatch(text); m != nil {
					text += shared[m[1]]
				}
				cur.codes[code] = text
			}
			code = ""
			block.Reset()
		}
	)
	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "paths:"):
			inPaths = true
			continue
		case inPaths && len(ln) > 0 && ln[0] != ' ' && ln[0] != '#':
			flushCodeFunc()
			inPaths = false
		}
		if !inPaths {
			continue
		}
		if m := specPathRe.FindStringSubmatch(ln); m != nil {
			flushCodeFunc()
			path, inResponses = m[1], false
			continue
		}
		if m := specMethodRe.FindStringSubmatch(ln); m != nil {
			flushCodeFunc()
			method, inResponses = strings.ToUpper(m[1]), false
			ops = append(ops, specOp{path: path, method: method, codes: map[string]string{}})
			cur = &ops[len(ops)-1]
			continue
		}
		if cur == nil {
			continue
		}
		if ln == "      responses:" {
			inResponses = true
			continue
		}
		if !inResponses {
			continue
		}
		if m := specCodeRe.FindStringSubmatch(ln); m != nil {
			flushCodeFunc()
			code = m[1]
			continue
		}
		block.WriteString(ln + "\n")
	}
	flushCodeFunc()
	if len(ops) == 0 {
		t.Fatal("no operations found in docs/openapi.yaml")
	}
	return ops
}

// specComponentResponses returns the text of each components/responses entry,
// by name.
//
// Parameters:
//   - lines: the spec's lines.
//
// Returns name to block text.
func specComponentResponses(lines []string) map[string]string {
	out := map[string]string{}
	in, name := false, ""
	for _, ln := range lines {
		switch {
		case ln == "  responses:":
			in = true
		case in && len(ln) > 2 && ln[2] != ' ':
			in = false
		case in:
			if m := specNameRe.FindStringSubmatch(ln); m != nil {
				name = m[1]
				continue
			}
			out[name] += ln + "\n"
		}
	}
	return out
}
