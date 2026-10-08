# The `gone` CLI

`gone` sends and opens one-time secrets from a terminal. It speaks the same protocol as the web UI ([protocol.md](protocol.md)), so a secret sent from the browser can be opened with the CLI and the other way round.

Everything is encrypted and decrypted on your machine. The server only ever stores ciphertext, and the key travels in the link's `#fragment`, which is never sent to the server.

The server binary is `goned`. You only need `gone` to send and receive secrets.

---

## Install

Each [GitHub release](https://github.com/haukened/gone/releases) contains static binaries (no libc dependency):

| OS      | Architectures  | Archive                           |
| ------- | -------------- | --------------------------------- |
| Linux   | amd64, arm64   | `gone_<tag>_linux_<arch>.tar.gz`  |
| macOS   | amd64, arm64   | `gone_<tag>_darwin_<arch>.tar.gz` |
| Windows | amd64, arm64   | `gone_<tag>_windows_<arch>.zip`   |

The release also contains `SHA256SUMS` and the `goned` server archives for Linux.

### Verify, then install

Every archive carries a [build-provenance attestation](https://docs.github.com/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds) signed by the release workflow. Check it before you run anything:

```sh
TAG=v3.0.0
ARCHIVE=gone_${TAG}_linux_amd64.tar.gz

curl -fsSLO "https://github.com/haukened/gone/releases/download/${TAG}/${ARCHIVE}"
curl -fsSLO "https://github.com/haukened/gone/releases/download/${TAG}/SHA256SUMS"

# 1. The archive was built by this repository's release workflow.
gh attestation verify "$ARCHIVE" --repo haukened/gone

# 2. The archive matches the published checksum.
sha256sum --ignore-missing -c SHA256SUMS   # macOS: shasum -a 256 --ignore-missing -c SHA256SUMS

tar -xzf "$ARCHIVE"
install -m 0755 "gone_${TAG}_linux_amd64/gone" ~/.local/bin/gone
gone version
```

On macOS, a downloaded binary may be quarantined by Gatekeeper. After verifying it, clear the flag with `xattr -d com.apple.quarantine ./gone`.

### Build from source

```sh
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags)" -o gone ./cmd/gone
```

---

## Quick start

```sh
# Send: the message comes from standard input, never from the command line.
printf 'hunter2' | gone send --ttl 30m
#   Link:        https://gone.hauken.us/secret/AbC…#v1:…
#   Manage link: https://gone.hauken.us/manage/AbC…#…
#   Expires:     2026-08-10T18:30:00Z

# Open it (quote the link: see "Quoting links").
gone get 'https://gone.hauken.us/secret/AbC…#v1:…'
```

Send the **link** to the recipient. Keep the **manage link** yourself: it lets you check on the secret or delete it, but it cannot open it.

---

## Commands

### `gone send`

Encrypts a message and/or files and uploads them.

```text
gone send [flags]

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
```

- **The message is read from standard input.** Pipe it in, or type it and press Ctrl-D (Ctrl-Z then Enter on Windows). There is deliberately no flag for the message *text*, so it never lands in shell history or the process list.
- `--message-file PATH` reads the message from a regular file instead, byte for byte (a trailing newline is kept), and standard input is not read at all. Pipes, FIFOs and devices are refused; pipe those into standard input instead.
- To send only files, give standard input nothing: `gone send -f report.pdf </dev/null`.
- The message plus files must fit within the server's size limit (`GONE_MAX_BYTES`, 10 MiB by default); the server also enforces its own minimum and maximum TTL. Independently of the server, `gone` refuses inputs over 64 MiB before encrypting anything (exit 2).
- An attachment's type is guessed from its file extension. Its name is the file's base name.
- At most one `--passphrase-*` flag may be used. A passphrase must be at least 8 characters. `--passphrase-generate` prints five random words from the EFF short wordlist, joined in CamelCase (about 51 bits), to standard error, so a redirected standard output only captures the link.

Example with files and a passphrase:

```sh
gone send -f id_ed25519 -f id_ed25519.pub --passphrase-generate --ttl 2h </dev/null
```

### `gone get`

Downloads, decrypts and deletes a secret.

```text
gone get <link|-> [flags]

  -o, --out DIR              directory for attachments (default .)
      --message-out PATH     write the message to the new file PATH instead of standard output
      --passphrase-file PATH read the passphrase from PATH instead of prompting
      --json                 print the result as JSON
      --raw                  print the message without escaping control characters
      --insecure             allow an http:// link on localhost
      --timeout DURATION     per-request timeout (default 1m)
```

- The message is written to standard output, attachments to `--out`. The secret is deleted from the server only after everything has been decrypted and written.
- A link of `-` is read from standard input (surrounding whitespace is ignored), so it stays out of the process list and shell history: `gone get - < link.txt`.
- `--message-out PATH` writes the message to a new file instead of standard output, with mode `0600`, byte for byte (`--raw` has no effect on it). An empty message still creates an empty file. The file is written to a temporary name in the same directory, synced, then hard-linked into place, so `PATH` either does not exist or is complete. It is never overwritten: if `PATH` exists, even as a dangling symlink, `get` fails with exit 8. On Windows the mode is not applied; the file inherits the directory's permissions.
- `--out` must be an existing, writable directory. `--message-out` must name a file that does not exist yet, in an existing directory that is writable and supports hard links. Both are checked before the link is opened, so a bad path never burns the secret. The same goes for `--passphrase-file` with a link that has no passphrase (exit 2).
- If writing fails *after* the link was opened (for example, an attachment cannot be saved), whatever this run wrote is removed and the deletion is not confirmed. The secret is still lost: the server has already handed it out once.
- **Existing files are never overwritten.** If `report.csv` exists, the attachment is saved as `report (1).csv`, and so on. Files are created exclusively with mode `0600` inside the output directory, and symlinks are not followed out of it.
- Attachment names are sanitized as in [protocol.md §6.1](protocol.md#61-file-names); Windows reserved names (`CON`, `NUL`, …), trailing dots and spaces, and the characters `<>:"|?*` (replaced with `_` on every platform) are also made safe. A leading `.` or `-` is replaced with `_` (`.zshenv` is saved as `_zshenv`), so a sender cannot plant a hidden dotfile or a name that looks like a command-line option.
- For a passphrase-protected link, `gone` prompts on the terminal (up to three tries). The ciphertext is downloaded once and every try runs against that copy. An empty or invalid entry (over 1024 bytes, or not UTF-8) uses up a try, like a wrong passphrase. Ctrl-D cancels at once, and because the link has already been opened, cancelling loses the secret. In scripts, use `--passphrase-file`; exactly one trailing newline (`\n` or `\r\n`) is removed from the file. That gives a single try: opening the link has already started the secret's deletion, so a wrong passphrase in the file means the secret is gone.
- On a terminal, control characters in the message are escaped so a sender cannot inject terminal escape sequences. `--raw` turns this off. When standard output is a pipe or file, the message is written byte-for-byte. With `--json`, DEL, C1 and bidirectional-override characters are always written as `\uXXXX` escapes, so the output is safe on a terminal and still decodes to the original text. (A message that is not valid UTF-8 can't round-trip through JSON: invalid bytes become U+FFFD. Use plain output for binary messages.)

### `gone status`

```text
gone status <manage-link|-> [--json] [--insecure] [--timeout DURATION]
```

Prints `State: pending`, with creation and expiry times, while the secret is still waiting. A manage link of `-` is read from standard input. Once it has been opened, revoked or has expired, the server answers "not found" (exit 3); it does not reveal which.

### `gone revoke`

```text
gone revoke <manage-link|-> [--json] [--insecure] [--timeout DURATION]
```

Deletes the secret. Revoking an already-gone secret exits 3. Revoke is reliable only before the recipient opens the link: someone who has already downloaded the ciphertext keeps it.

### `gone set server`

```text
gone set server <url> [--insecure]
```

Saves the default server for `send`. The value must be a bare origin such as `https://gone.example.com`. The file is `gone/config.json` under your user configuration directory (`$XDG_CONFIG_HOME`, or `~/.config` if unset, on Linux; `~/Library/Application Support` on macOS; `%AppData%` on Windows), written with mode `0600`.

`get`, `status` and `revoke` always use the server named in the link.

### `gone version`, `gone help`

`gone version` prints the build version. `gone help <command>` prints the help for one command.

---

## Scripts and agents

Scripts and AI agents can move secrets *by reference*, so the plaintext never passes through their output, logs or context: file in, link out; link in, file out.

**Send a file's contents** (here a generated restic password):

```sh
gone send --message-file /root/.restic-password --ttl 1h --json </dev/null | jq -r .link
```

The message comes from the file, not from `cat`, so nothing secret flows through a pipe. Add `--passphrase-file PATH` for a v2 link.

**Receive into a file:**

```sh
gone get - --message-out ./restic.pw --json < link.txt
#   {"message_file":{"path":"./restic.pw","size":33},"files":[]}
```

The link is read from standard input, so it needs no quoting and stays out of `ps`. The message lands in a new `0600` file and is never printed. For a passphrase-protected link, add `--passphrase-file PATH`.

Guidelines:

- **Don't use `get --json` without `--message-out`.** Its `message` field is the plaintext.
- **No prompts.** With `--message-file` (send) and `--passphrase-file` (get), `gone` never reads the terminal. Redirect standard input from `/dev/null` (or a link file) anyway, so nothing can block on a TTY.
- **Check before the claim.** Everything `get` can verify locally (the link, the passphrase file, `--out` and `--message-out`) is checked before the link is opened. A local mistake exits with 2 or 8 and the link still works. A failure after the link was opened (a wrong passphrase in the file, an integrity error, or a failed write) loses the secret: ask the sender to send it again.
- **Exit codes** are listed in [Exit codes](#exit-codes); with `--json`, errors are JSON on standard error.
- **Clean up.** Delete the received file when it has been used.

---

## Server selection and transport

`send` picks its server from, in order: `--server`, the `GONE_SERVER` environment variable, the saved config, then `https://gone.hauken.us`.

The standard proxy variables (`HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY`) are honored for every request.

- **HTTPS only.** `http://` origins and links are refused on every command. The one exception is a server on this machine (`localhost`, `*.localhost`, `127.0.0.0/8` or `::1`) with `--insecure`, for local testing; browsers make the same exception. Any other `http://` server is refused even with `--insecure`.
- **No redirects.** A redirect is treated as a server error, so a secret is never sent to, or fetched from, an origin other than the one you named.
- **Bounded responses.** Responses larger than the protocol allows are rejected before they are read.

---

## Exit codes

| Code | Meaning                                                                |
| ---: | ---------------------------------------------------------------------- |
| 0    | Success                                                                |
| 1    | Internal error, or interrupted (Ctrl-C)                                |
| 2    | Usage error: bad flag, bad link, bad server URL, weak passphrase       |
| 3    | Not found: already opened, revoked, expired, or never existed          |
| 4    | Wrong passphrase (or the link is damaged)                              |
| 5    | Rate limited by the server                                             |
| 6    | Integrity failure: the ciphertext or payload failed authentication     |
| 7    | Network or server error, response too large, or deletion not confirmed |
| 8    | Local I/O error: reading input files, writing attachments or config    |

The server cannot tell "already opened", "revoked" and "expired" apart from the outside, so they share code 3.

If `get` decrypts a secret but the server does not confirm its deletion, `gone` prints the message and attachments and then exits 7. The server's claim lease expires on its own and the secret is removed by the janitor.

---

## JSON output

With `--json`, results are a single line of JSON on standard output. Times are RFC 3339 in UTC.

`send`:

```json
{"link":"https://…/secret/ID#v1:KEY","manage_link":"https://…/manage/ID#TOKEN","id":"ID","expires_at":"2026-08-10T18:30:00Z"}
```

`get`:

```json
{"message":"hunter2","files":[{"name":"report.csv","path":"out/report (1).csv","type":"text/csv","size":1024}]}
```

This contains the plaintext. Anything that captures standard output (a log, a CI job, an AI agent's context) captures the secret. To keep it out, use `--message-out`.

`get --message-out`: the `message` key is absent, and `message_file` gives the path as you passed it and the size in bytes:

```json
{"message_file":{"path":"creds/restic.pw","size":17},"files":[]}
```

`status`:

```json
{"state":"pending","created_at":"2026-08-10T17:30:00Z","expires_at":"2026-08-10T18:30:00Z"}
```

`revoke`:

```json
{"revoked":true}
```

Errors are one line of JSON on **standard error**, and the exit code is still set:

```json
{"error":"rate_limited","message":"rate limited; retry after 30s","retry_after":30}
```

| `error`            | Exit |
| ------------------ | ---: |
| `internal`         | 1    |
| `interrupted`      | 1    |
| `usage`            | 2    |
| `not_found`        | 3    |
| `wrong_passphrase` | 4    |
| `rate_limited`     | 5    |
| `integrity`        | 6    |
| `network`          | 7    |
| `server`           | 7    |
| `too_large`        | 7    |
| `not_confirmed`    | 7    |
| `io`               | 8    |

`retry_after` (seconds) is present only for `rate_limited`, and only when the server supplied it. `--passphrase-generate` still prints the passphrase to standard error as plain text, outside the JSON.

---

## Quoting links

Always put links in **single quotes**:

```sh
gone get 'https://gone.hauken.us/secret/AbC#v1:XyZ'
```

The key is after the `#`. Some shells treat `#` or other characters specially, and an unquoted link can be silently cut short. A damaged link is rejected before anything is fetched, so the secret is not burned, but quoting avoids the confusion. Passing the link on standard input with `-` avoids quoting altogether.

---

## Security notes

- **Shell history and the process list.** A link given as an argument can end up in shell history and is briefly visible to other local users in `ps`. Pass `-` instead and supply the link on standard input (`gone get - < link.txt`, `gone status - < manage.txt`). A share link is useless once opened, so open it promptly. A manage link stays valid until the secret is gone.
- **Messages and passphrases never come from arguments.** Messages come from standard input. Passphrases come from a no-echo terminal prompt or a file. Prefer a file with mode `0600`, or a process substitution such as `--passphrase-file <(pass show gone)`.
- **Share the passphrase separately.** A passphrase only helps if it travels on a different channel than the link. Someone who opens a leaked link burns the secret, and can then guess the passphrase offline; a generated passphrase makes that impractical.
- **Untrusted content.** Messages and file names come from the sender. On a terminal, `gone` escapes control characters and bidirectional-override characters in messages and in the file names it prints, and attachments are never written outside `--out`. Review attachments before opening them.
- **Memory.** Keys, passphrases and plaintext are wiped from memory after use where Go makes that possible. This is best effort: the Go runtime can copy data (for example when converting to strings), and the garbage collector does not zero freed memory.
- **Plaintext on disk.** Attachments and `--message-out` files are written in plaintext, readable only by you (`0600`). Delete them when you are done.
- **What `gone` never prints.** Error messages and reports never contain the plaintext, a passphrase, a link's key or `#fragment`, a manage token, or the secret ID. Network errors name the server's origin, not the request path. The only exceptions are the outputs you ask for: the message from `get` without `--message-out`, the links from `send`, and a `--passphrase-generate` passphrase. Values you type as arguments are your own exposure (they are already in `ps` and history), and `gone` may repeat a mistyped flag value in a usage error.
