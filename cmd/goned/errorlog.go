package main

import (
	"log"
	"log/slog"
	"regexp"
	"strings"
)

// addrPattern matches IPv4 and bracketed IPv6 socket addresses (ip:port), the
// form net/http and net use when they name a peer in an error message.
var addrPattern = regexp.MustCompile(`\[[0-9A-Fa-f:.%a-zA-Z0-9]*\]:\d+|\b\d{1,3}(?:\.\d{1,3}){3}:\d+`)

// redactedAddr replaces every address found in a server log message.
const redactedAddr = "<addr>"

// redactAddrs removes anything that looks like an IP:port from msg.
//
// Parameters:
//   - msg: a log message written by net/http.
//
// Returns:
//   - string: msg with each address replaced by a placeholder.
func redactAddrs(msg string) string {
	return addrPattern.ReplaceAllString(msg, redactedAddr)
}

// slogWriter forwards net/http's internal log lines to slog at warn level
// with client addresses removed.
type slogWriter struct{}

// Write logs p, redacted and trimmed, and reports it fully consumed.
//
// Parameters:
//   - p: one log line (possibly with a stack trace) from net/http.
//
// Returns:
//   - int: len(p).
//   - error: always nil.
func (slogWriter) Write(p []byte) (int, error) {
	slog.Warn("http server", "msg", strings.TrimSpace(redactAddrs(string(p))))
	return len(p), nil
}

// newErrorLog builds the http.Server.ErrorLog that keeps net/http's own
// messages (handler panics, TLS and accept errors) out of stderr's raw form,
// where they would carry client addresses.
//
// Returns:
//   - *log.Logger: a logger forwarding to slog with addresses redacted.
func newErrorLog() *log.Logger {
	return log.New(slogWriter{}, "", 0)
}
