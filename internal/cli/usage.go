package cli

// usageMain is printed by "gone", "gone help" and "gone --help".
const usageMain = `gone - share one-time secrets from the command line

Usage:
  gone send   [flags]               encrypt and upload a message and/or files
  gone get    <link> [flags]        download, decrypt and delete a secret
  gone status <manage-link> [flags] show whether a secret is still waiting
  gone revoke <manage-link> [flags] delete a secret before it is opened
  gone request [flags]              ask someone for a secret; prints a link to send them
  gone request list|open|cancel     see, open, or cancel the requests saved here
  gone reply  <request-link>        answer someone's request with a secret
  gone set server <url>             save the default server
  gone version                      print the version
  gone help [command]               show help for a command

Secrets are encrypted locally; the server never sees the key.
A link of "-" is read from standard input, keeping it out of the process
list and shell history.
Exit codes: 0 ok, 1 internal, 2 usage, 3 not found, 4 wrong passphrase,
5 rate limited, 6 integrity, 7 network/server, 8 local I/O.
`

// usageSend documents "gone send".
const usageSend = `Usage: gone send [flags]

Reads the message from standard input (pipe it, or type it and press
Ctrl-D; Ctrl-Z then Enter on Windows), or from --message-file. The message
is never taken from the command line, so it stays out of shell history.

Flags:
  -s, --server URL           server origin (default: $GONE_SERVER, config, https://gone.hauken.us)
  -t, --ttl DURATION         lifetime, e.g. 30m or 24h (default 1h)
  -f, --file PATH            attach a file; repeat for more (max 10)
      --message-file PATH    read the message from PATH instead of standard input
      --passphrase-prompt    protect with a passphrase typed at a prompt
      --passphrase-file PATH protect with the passphrase in PATH
      --passphrase-generate  protect with a generated passphrase (printed to stderr)
      --json                 print the result as JSON
      --insecure             allow an http:// server on localhost
      --timeout DURATION     per-request timeout (default 1m)
`

// usageGet documents "gone get".
const usageGet = `Usage: gone get <link|-> [flags]

Downloads and decrypts the secret, writes attachments to the output
directory, prints the message (or writes it to --message-out), then
deletes the secret from the server. Existing files are never overwritten.
A link of "-" is read from standard input.

Flags:
  -o, --out DIR              directory for attachments (default .)
      --message-out PATH     write the message to the new file PATH instead of standard output
      --passphrase-file PATH read the passphrase from PATH instead of prompting
      --json                 print the result as JSON
      --raw                  print the message without escaping control characters
      --insecure             allow an http:// link on localhost
      --timeout DURATION     per-request timeout (default 1m)
`

// usageStatus documents "gone status".
const usageStatus = `Usage: gone status <manage-link|-> [flags]

Shows whether the secret is still waiting to be opened. A manage link of
"-" is read from standard input.

Flags:
      --json                 print the result as JSON
      --insecure             allow an http:// link on localhost
      --timeout DURATION     per-request timeout (default 1m)
`

// usageRevoke documents "gone revoke".
const usageRevoke = `Usage: gone revoke <manage-link|-> [flags]

Deletes the secret so it can no longer be opened. A manage link of "-" is
read from standard input.

Flags:
      --json                 print the result as JSON
      --insecure             allow an http:// link on localhost
      --timeout DURATION     per-request timeout (default 1m)
`

// usageSet documents "gone set".
const usageSet = `Usage: gone set server <url> [--insecure]

Saves the default server used by "gone send". The value must be a bare
origin such as https://gone.example.com. http:// is refused, except for a
localhost server with --insecure here and on every later send.
`

// usageVersion documents "gone version".
const usageVersion = `Usage: gone version

Prints the version.
`

// usages maps command names to their help text.
const usageRequest = `Usage: gone request [flags]
       gone request list [--offline] [flags]
       gone request open <id> [--wait] [flags]
       gone request cancel <id> [flags]

Asks someone for a secret. A key pair is made here and the request is saved
on this device (requests.db in the gone config directory, readable only
by you); the server never sees the private key. Send the printed link to the
person who has the secret; they answer with "gone reply" or in a browser.
Only this device can open the reply, once.

<id> is a request ID or a unique prefix of at least 4 characters, as shown
by "gone request list".

  gone request           make a request and print its link, then exit (or --wait)
  gone request list      list saved requests and ask the server for each state
  gone request open      decrypt the reply, print it or save it, then delete it
  gone request cancel    cancel the request, or delete a reply before opening it

Flags for making a request:
  -l, --label TEXT           a note to tell your requests apart (kept on this device only)
  -t, --ttl DURATION         how long to wait for a reply, e.g. 30m or 24h (default 1h);
                             a reply is then kept as long again
  -s, --server URL           server origin (default: $GONE_SERVER, config, https://gone.hauken.us)
      --wait                 wait for the reply and open it (the request is still saved first)

Flags for opening a reply (open, and request --wait):
      --wait                 wait for the reply if it hasn't arrived yet
  -o, --out DIR              directory for attachments (default .)
      --message-out PATH     write the message to the new file PATH instead of standard output
      --raw                  print the message without escaping control characters

Flags for list:
      --offline              show the last known state without asking the server

Common flags:
      --json                 print the result as JSON (--wait prints two lines)
      --insecure             allow an http:// server on localhost
      --timeout DURATION     per-request timeout (default 1m)
`

const usageReply = `Usage: gone reply <request-link|-> [flags]

Answers someone's request for a secret. The link is checked first, so
nothing is read if the request is no longer open. The message comes from
standard input or --message-file, and is encrypted to the requester's key:
only the device that made the request can open it, once.
A link of "-" is read from standard input.

Flags:
  -f, --file PATH            attach a file; repeat for more (max 10)
      --message-file PATH    read the message from PATH instead of standard input
      --json                 print the result as JSON
      --insecure             allow an http:// link on localhost
      --timeout DURATION     per-request timeout (default 1m)
`

var usages = map[string]string{
	"send":    usageSend,
	"get":     usageGet,
	"status":  usageStatus,
	"revoke":  usageRevoke,
	"request": usageRequest,
	"reply":   usageReply,
	"set":     usageSet,
	"version": usageVersion,
}
