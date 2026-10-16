package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// pinFixture is a real server holding one secret, plus an attacker server
// that counts every request it receives.
type pinFixture struct {
	te       *testEnv
	server   string
	attacker string
	hits     *atomic.Int64
	link     string
	manage   string
	request  string
}

// newPinFixture sends a secret to a fresh server and starts an attacker
// server. Link commands run without the automatic --allow-origin.
//
// Parameters:
//   - t: the test.
//
// Returns the fixture.
func newPinFixture(t *testing.T) *pinFixture {
	t.Helper()
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	f := &pinFixture{te: te, server: srv.URL, hits: &atomic.Int64{}}
	f.link, f.manage = sendText(t, te, "pinned secret", "--server", srv.URL)
	f.request = requestJSON(t, te, srv.URL)["link"]
	att := newTLS(t, te, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { f.hits.Add(1) }))
	f.attacker = att.URL
	te.strict = true
	te.reset()
	return f
}

// moved returns link with its origin replaced by the attacker's.
//
// Parameters:
//   - link: a link on the real server.
//
// Returns the rewritten link.
func (f *pinFixture) moved(link string) string {
	return f.attacker + strings.TrimPrefix(link, f.server)
}

// TestLinkOriginRefused checks that a link for another server is refused
// before any request, naming both origins but never the ID or key.
//
// Parameters:
//   - t: the test.
func TestLinkOriginRefused(t *testing.T) {
	f := newPinFixture(t)
	secretPart := strings.TrimPrefix(f.link, f.server+"/secret/")
	managePart := strings.TrimPrefix(f.manage, f.server)
	for _, args := range [][]string{
		{"get", f.moved(f.link)},
		{"status", f.moved(f.manage)},
		{"revoke", f.moved(f.manage)},
		{"reply", f.moved(f.request)},
	} {
		f.te.reset()
		code := f.te.run(args...)
		msg := f.te.stderr.String()
		if code != exitUsage || !strings.Contains(msg, "link is for "+f.attacker) || !strings.Contains(msg, "default https://gone.hauken.us") ||
			!strings.Contains(msg, "gone set server "+f.attacker) || !strings.Contains(msg, "--allow-origin "+f.attacker) {
			t.Errorf("%s: exit %d, stderr %q", args[0], code, msg)
		}
		if strings.Contains(msg, secretPart[:8]) || strings.Contains(msg, managePart[len(managePart)-8:]) {
			t.Errorf("%s: stderr leaks the link: %q", args[0], msg)
		}
	}
	if n := f.hits.Load(); n != 0 {
		t.Fatalf("attacker saw %d requests", n)
	}
}

// TestLinkOriginRefusedConfigJSON checks the message for a configured
// server and the JSON error kind.
//
// Parameters:
//   - t: the test.
func TestLinkOriginRefusedConfigJSON(t *testing.T) {
	f := newPinFixture(t)
	writeConfig(t, f.te, `{"server":"`+f.server+`"}`)
	if code := f.te.run("status", f.moved(f.manage), "--json"); code != exitUsage {
		t.Fatalf("exit %d: %s", code, f.te.stderr)
	}
	var got struct{ Error, Message string }
	if err := json.Unmarshal(f.te.stderr.Bytes(), &got); err != nil || got.Error != "untrusted_origin" ||
		!strings.Contains(got.Message, "gone is set to use "+f.server+" (from config)") {
		t.Fatalf("json %q: %v", f.te.stderr, err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("attacker was contacted")
	}
}

// TestLinkOriginAccepted checks every way of trusting a link's server.
//
// Parameters:
//   - t: the test.
func TestLinkOriginAccepted(t *testing.T) {
	f := newPinFixture(t)
	check := func(name string, args ...string) {
		t.Helper()
		f.te.reset()
		if code := f.te.run(append([]string{"status", f.manage}, args...)...); code != exitOK {
			t.Errorf("%s: exit %d: %s", name, code, f.te.stderr)
		}
	}
	check("allow-origin", "--allow-origin", f.server)
	check("allow-origin list", "--allow-origin", " https://other.example ,,"+f.server+"/")
	check("allow-origin repeated", "--allow-origin", "https://other.example", "--allow-origin", f.server)
	check("server flag", "--server", f.server)
	f.te.vars[serverEnv] = f.server
	check("GONE_SERVER")
	delete(f.te.vars, serverEnv)
	writeConfig(t, f.te, `{"server":"`+f.server+`"}`)
	check("config")
}

// TestLinkOriginFlagErrors checks bad --allow-origin values (never echoed)
// and a bad or unreadable configured server.
//
// Parameters:
//   - t: the test.
func TestLinkOriginFlagErrors(t *testing.T) {
	f := newPinFixture(t)
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"whole link", []string{"--allow-origin", f.link}, exitUsage, "--allow-origin is not a valid server origin"},
		{"http", []string{"--allow-origin", "http://gone.example"}, exitUsage, "--allow-origin uses http"},
		{"bad server flag", []string{"--server", "nope"}, exitUsage, "--server is not a valid server origin"},
	}
	for _, tt := range tests {
		f.te.reset()
		code := f.te.run(append([]string{"get", f.link}, tt.args...)...)
		if msg := f.te.stderr.String(); code != tt.code || !strings.Contains(msg, tt.want) || strings.Contains(msg, "/secret/") {
			t.Errorf("%s: exit %d, stderr %q", tt.name, code, msg)
		}
	}
	f.te.reset()
	writeConfig(t, f.te, "{")
	if code := f.te.run("status", f.manage); code != exitIO {
		t.Fatalf("bad config: exit %d: %s", code, f.te.stderr)
	}
}

// TestServerOverrideNotice checks the stderr notice when --server or
// GONE_SERVER overrides a configured server for send.
//
// Parameters:
//   - t: the test.
func TestServerOverrideNotice(t *testing.T) {
	te := newTestEnv(t)
	srv := newGoneServer(t, te)
	notice := "Using " + srv.URL + " (from --server), not the configured server https://cfg.example."
	sendText(t, te, "a", "--server", srv.URL)
	if strings.Contains(te.stderr.String(), "Using ") {
		t.Fatalf("notice without config: %q", te.stderr)
	}
	writeConfig(t, te, `{"server":"https://cfg.example"}`)
	sendText(t, te, "a", "--server", srv.URL)
	if !strings.Contains(te.stderr.String(), notice) {
		t.Fatalf("stderr %q", te.stderr)
	}
	te.vars[serverEnv] = srv.URL
	sendText(t, te, "a")
	if !strings.Contains(te.stderr.String(), "(from "+serverEnv+")") {
		t.Fatalf("stderr %q", te.stderr)
	}
	writeConfig(t, te, `{"server":"`+srv.URL+`"}`)
	sendText(t, te, "a")
	if strings.Contains(te.stderr.String(), "Using ") {
		t.Fatalf("notice for the configured server: %q", te.stderr)
	}
}
