package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"
)

// app runs one command with the process environment.
type app struct {
	env  *Env
	json bool
}

// command is a subcommand implementation.
type command func(a *app, ctx context.Context, args []string) error

// commands maps subcommand names to implementations.
var commands = map[string]command{
	"send":    (*app).runSend,
	"get":     (*app).runGet,
	"status":  (*app).runStatus,
	"revoke":  (*app).runRevoke,
	"request": (*app).runRequest,
	"reply":   (*app).runReply,
	"set":     func(a *app, _ context.Context, args []string) error { return a.runSet(args) },
}

// Run executes the gone CLI.
//
// Parameters:
//   - ctx: cancellation context, typically tied to SIGINT.
//   - args: command-line arguments without the program name.
//   - env: process environment.
//
// Returns the process exit code.
func Run(ctx context.Context, args []string, env *Env) int {
	a := &app{env: env, json: wantsJSON(args)}
	if len(args) == 0 {
		return a.help(nil)
	}
	switch name := args[0]; name {
	case "help", "-h", "--help":
		return a.help(args[1:])
	case "version", "--version":
		return a.finish(writeText(env.Stdout, "gone "+env.Version+"\n"))
	default:
		cmd, ok := commands[name]
		if !ok {
			return a.finish(usagef("unknown command %q; run \"gone help\"", name))
		}
		err := cmd(a, ctx, args[1:])
		if errors.Is(err, flag.ErrHelp) {
			return a.finish(writeText(env.Stdout, usages[name]))
		}
		return a.finish(err)
	}
}

// help prints general or per-command usage.
//
// Parameters:
//   - args: optional command name.
//
// Returns the exit code.
func (a *app) help(args []string) int {
	if len(args) == 0 {
		return a.finish(writeText(a.env.Stdout, usageMain))
	}
	text, ok := usages[args[0]]
	if !ok {
		return a.finish(usagef("unknown command %q; run \"gone help\"", args[0]))
	}
	return a.finish(writeText(a.env.Stdout, text))
}

// finish reports err, if any, and returns the matching exit code.
//
// Parameters:
//   - err: command result.
//
// Returns the exit code.
func (a *app) finish(err error) int {
	if err == nil {
		return exitOK
	}
	f := classify(err)
	if a.json {
		out := struct {
			Error      string `json:"error"`
			Message    string `json:"message"`
			RetryAfter int64  `json:"retry_after,omitempty"`
		}{f.kind, f.msg, int64((f.retryAfter + time.Second - 1) / time.Second)}
		_ = writeJSON(a.env.Stderr, out)
		return f.code
	}
	msg := "gone: " + escapeTerminal(f.msg)
	if f.code == exitUsage {
		msg += "\n(run \"gone help\" for usage)"
	}
	_, _ = fmt.Fprintln(a.env.Stderr, msg)
	return f.code
}

// wantsJSON reports whether --json appears before any "--" terminator, so
// errors raised while parsing flags are still reported as JSON.
//
// Parameters:
//   - args: command-line arguments.
//
// Returns true when JSON output was requested.
func wantsJSON(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--":
			return false
		case "--json", "-json", "--json=true", "-json=true":
			return true
		}
	}
	return false
}
