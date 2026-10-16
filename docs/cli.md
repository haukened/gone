# The `gone` CLI

`gone` sends and opens one-time secrets from a terminal, and can ask someone for a secret or answer such a request. It speaks the same protocol as the web UI ([protocol.md](protocol.md)), so a secret sent from the browser can be opened with the CLI and the other way round, and a request made in either can be answered from the other.

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

The release also contains `SHA256SUMS`, the `goned` server archives for Linux, and the signed build provenance as `gone_<tag>.sigstore.json` (a Sigstore bundle, for offline checks with `gh attestation verify ... --bundle`) and `gone_<tag>.intoto.jsonl` (the same statement as an in-toto DSSE envelope).

### Verify, then install

Every archive carries a [build-provenance attestation](https://docs.github.com/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds) signed by the release workflow. Check it before you run anything:

```sh
TAG=v3.11.0
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

Need a secret from someone instead?

```sh
gone request --label "staging DB"
#   Link:    https://gone.hauken.us/reply/3f9a…#v3:…
#   ID:      3f9a1c2e…
#   Expires: 2026-08-10T18:30:00Z

# They answer in a browser, or with: printf 'hunter2' | gone reply '<link>'
gone request list             # see which requests have a reply
gone request open 3f9a        # print the reply, then delete it
```

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
- **Server policy.** Before reading anything, `gone` fetches the server's policy (`GET /api/policy`, see [Policies](configuration.md#policies)). If the server requires a passphrase and no `--passphrase-*` flag is given, or more files are attached than it allows, `gone` exits 2 with error kind `policy` and nothing is read, prompted for or sent. A server older than v3.12 has no policy route and is treated as having no policy.
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
  -s, --server URL           the server you trust (default: $GONE_SERVER, config, https://gone.hauken.us)
      --allow-origin ORIGIN  also trust this server for this command; repeat or comma-separate
      --insecure             allow an http:// link on localhost
      --timeout DURATION     per-request timeout (default 1m)
```

- The link must point at the server you use, or one you allow with `--allow-origin`; otherwise nothing is sent (see [Which servers a link may reach](#which-servers-a-link-may-reach)).
- The message is written to standard output, attachments to `--out`. The secret is deleted from the server only after everything has been decrypted and written.
- A link of `-` is read from standard input (surrounding whitespace is ignored), so it stays out of the process list and shell history: `gone get - < link.txt`.
- `--message-out PATH` writes the message to a new file instead of standard output, with mode `0600`, byte for byte (`--raw` has no effect on it). An empty message still creates an empty file. The file is written to a temporary name in the same directory, synced, then hard-linked into place, so `PATH` either does not exist or is complete. It is never overwritten: if `PATH` exists, even as a dangling symlink, `get` fails with exit 8. On Windows the mode is not applied; the file inherits the directory's permissions.
- `--out` must be an existing, writable directory. `--message-out` must name a file that does not exist yet, in an existing directory that is writable and supports hard links. Both are checked before the link is opened, so a bad path never burns the secret. The same goes for `--passphrase-file` with a link that has no passphrase (exit 2).
- If writing fails *after* the link was opened (for example, an attachment cannot be saved), whatever this run wrote is removed and the deletion is not confirmed. The secret is still lost: the server has already handed it out once.
- **Existing files are never overwritten.** If `report.csv` exists, the attachment is saved as `report (1).csv`, and so on. Files are created exclusively with mode `0600` inside the output directory, and symlinks are not followed out of it.
- Attachment names are sanitized as in [protocol.md §6.1](protocol.md#61-file-names); Windows reserved names (`CON`, `NUL`, …), trailing dots and spaces, and the characters `<>:"|?*` (replaced with `_` on every platform) are also made safe. A leading `.` or `-` is replaced with `_` (`.zshenv` is saved as `_zshenv`), so a sender cannot plant a hidden dotfile or a name that looks like a command-line option.
- For a passphrase-protected link, `gone` prompts on the terminal (up to three tries). The ciphertext is downloaded once and every try runs against that copy. An empty or invalid entry (over 1024 bytes, or not UTF-8) uses up a try, like a wrong passphrase. Before the third try, `gone` warns that it is the last one: the link has already been opened, so a third wrong passphrase loses the secret for good (exit 4, and the error says so). Ctrl-D cancels at once, and because the link has already been opened, cancelling loses the secret. In scripts, use `--passphrase-file`; exactly one trailing newline (`\n` or `\r\n`) is removed from the file. That gives a single try: opening the link has already started the secret's deletion, so a wrong passphrase in the file means the secret is gone, and the error says it can no longer be recovered.
- If the download is cut off after the server handed out the secret, `gone` fetches the same ciphertext again with its claim token, up to twice, while the claim lease lasts (two minutes by default; see [protocol.md §8.2](protocol.md#82-claim-get-apisecretid)). Nobody else can fetch it in that time.
- On a terminal, control characters in the message are escaped so a sender cannot inject terminal escape sequences. `--raw` turns this off. When standard output is a pipe or file, the message is written byte-for-byte. With `--json`, DEL, C1 and bidirectional-override characters are always written as `\uXXXX` escapes, so the output is safe on a terminal and still decodes to the original text. (A message that is not valid UTF-8 can't round-trip through JSON: invalid bytes become U+FFFD. Use plain output for binary messages.)

### `gone status`

```text
gone status <manage-link|-> [-s URL] [--allow-origin ORIGIN]... [--json] [--insecure] [--timeout DURATION]
```

Prints `State: pending`, with creation and expiry times, while the secret is still waiting. A manage link of `-` is read from standard input. Once it has been opened, revoked or has expired, the server answers "not found" (exit 3); it does not reveal which.

### `gone revoke`

```text
gone revoke <manage-link|-> [-s URL] [--allow-origin ORIGIN]... [--json] [--insecure] [--timeout DURATION]
```

Deletes the secret. Revoking an already-gone secret exits 3. Revoke is reliable only before the recipient opens the link: someone who has already downloaded the ciphertext keeps it.

### `gone request`

```text
gone request [-l TEXT] [-t DURATION] [-s URL] [--wait [-o DIR] [--message-out PATH] [--raw]] [--json]
```

Asks someone for a secret and prints a **request link** to send them, then exits. A key pair is made on this machine: the private key and the request's manage token are saved on this device (see [Requests on this device](#requests-on-this-device)) and never leave it. The server learns only how long to wait. The person with the secret answers on that link in a browser, or with [`gone reply`](#gone-reply), and only this device can open the reply, once.

- `-l, --label TEXT` is a note so you can tell requests apart. Up to 80 characters are kept; a longer label is cut to 80. It stays on this device and is never sent.
- `-t, --ttl` is how long the link works (default `1h`). A reply is then kept for the same length of time, counted from when it arrives.
- `-s, --server` picks the server as for `send`.
- `--wait` waits for the reply and opens it, taking the same output flags as `request open`. The request is saved first, so Ctrl-C stops waiting but keeps it; resume with `gone request open ID --wait`.
- The request link can't open anything; it only lets someone send you one reply. Send it to the right person.

### `gone request list`

```text
gone request list [--offline] [--json] [--insecure] [--timeout DURATION]
```

Lists the requests saved on this device, newest first, and asks each one's server for its state:

```text
ID        LABEL       STATE        ASKED                 EXPIRES
3f9a1c2e  staging DB  reply ready  2026-08-10 17:30 UTC  2026-08-10 19:30 UTC
b7d0e411  -           waiting      2026-08-10 17:12 UTC  2026-08-10 18:12 UTC
```

A request the server no longer has (expired, cancelled, or already opened) is shown once as `gone` and then forgotten. If a server can't be reached the state is `unknown`. `--offline` shows the last known state without contacting any server. Checking state is read-only on the server.

### `gone request open`

```text
gone request open <id> [--wait] [-o DIR] [--message-out PATH] [--raw] [--json] [--insecure] [--timeout DURATION]
```

Downloads the reply, decrypts it with the saved key, and prints the message and saves attachments exactly like [`gone get`](#gone-get). It then deletes the reply from the server and forgets the request here. `<id>` is the full ID or a unique prefix of at least 4 characters. Without `--wait`, a request that has no reply yet exits 3 (`not_ready`); with it, `gone` checks every 10 seconds until the reply arrives or the request expires.

### `gone request cancel`

```text
gone request cancel <id> [--json] [--insecure] [--timeout DURATION]
```

Cancels the request, or deletes a reply before you open it, and forgets it here. The link stops working at once. Cancelling a request that is already gone exits 3.

### `gone reply`

```text
gone reply <request-link|-> [-f FILE]... [--message-file PATH] [-s URL] [--allow-origin ORIGIN]... [--json] [--insecure] [--timeout DURATION]
```

Answers someone's request. The link is checked with the server first, so if the request was already answered, cancelled, or has expired, `gone` exits 3 without reading the message. The message comes from standard input or `--message-file`, plus up to 10 attachments (fewer if the server's policy says so), exactly as for `send`. A reply never needs a passphrase, even on a server that requires one for `send`. It is encrypted to the requester's public key from the link: not even you can open it afterwards. Prints `Sent. Only the requester can open it.` and an `Expires:` line with when the reply is deleted if the requester never opens it. A request link of `-` is read from standard input.

Only answer a request link you trust: anyone can make one, so check it came from the person you expect.

### Requests on this device

Requests are saved in `requests.gob` in the `gone` config directory (`~/.config/gone` on Linux, `~/Library/Application Support/gone` on macOS, `%AppData%\gone` on Windows), readable only by you (`0600`, in a `0700` directory). Each entry holds the request ID, its server, your label, the manage token, the private key, the request link and the last known state. That is as private as a browser's site storage, which is where the web UI keeps its requests: anyone who can read the file as you can open a reply that arrives.

The file is internal to `gone`: it is a binary (Go gob) file, not meant to be opened, edited or copied. Use `gone request list` to see what's in it. Every change takes a lock (`requests.lock`), writes a new file and renames it into place, so several `gone` commands running at once never lose a request and the file is never half-written. Like ssh with a private key, `gone` refuses to use the file (exit 8) if other users can read or write it, if it belongs to someone else, or if it is a symlink, and refuses a config directory that others can write to. It says why and prints the `chmod` that fixes it, rather than fixing it silently, because the keys may already have been exposed. (Windows protects your profile with ACLs instead, so these checks don't apply there.) An entry is removed when its reply is opened, the request is cancelled, or it expires. A request made here can only be opened here, and one made in a browser can only be opened in that browser.

### `gone set server`

```text
gone set server <url> [--insecure]
```

Saves the default server for `send` and `request`. The value must be a bare origin such as `https://gone.example.com`. The file is `gone/config.json` under your user configuration directory (`$XDG_CONFIG_HOME`, or `~/.config` if unset, on Linux; `~/Library/Application Support` on macOS; `%AppData%` on Windows), written with mode `0600`. On Linux and macOS, `gone` refuses to read a config file that other users can write to or that belongs to someone else (exit 8, naming the file), because it decides which server every command trusts. Fix it with `chmod 600` on the file.

`get`, `status`, `revoke` and `reply` use the server named in the link, but only if it is the server you use; see [Which servers a link may reach](#which-servers-a-link-may-reach). `request list`, `request open` and `request cancel` use the server each saved request was made on.

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
- **Links from other servers.** A script that opens links from a server other than its default passes `--server URL` or `--allow-origin ORIGIN`, or sets `GONE_SERVER`; any other link exits 2 (`untrusted_origin`) without contacting anything.
- **Exit codes** are listed in [Exit codes](#exit-codes); with `--json`, errors are JSON on standard error.
- **Clean up.** Delete the received file when it has been used.

**Ask a person for a secret and wait for it.** A script or agent that needs a credential it doesn't have can request it and block:

```sh
gone request --label "prod DB for migration" --wait --message-out ./db.pw --json </dev/null > events.json &
# The first line appears at once: hand its link to the person who has the secret.
head -1 events.json | jq -r .link
wait   # the second line appears once they answer: {"message_file":{"path":"./db.pw","size":24},"files":[]}
```

The secret lands in a new `0600` file and never passes through output. If the wait is interrupted, `gone request open ID --wait --message-out ./db.pw` picks it up again.

**Answer a request** without the secret passing through a pipe:

```sh
gone reply - --message-file /root/.db-password --json < request-link.txt
```

---

## Server selection and transport

`send` and `request` pick their server from, in order: `--server`, the `GONE_SERVER` environment variable, the saved config, then `https://gone.hauken.us`. When `--server` or `GONE_SERVER` overrides a server saved with `gone set server`, a one-line notice on standard error names the server in use (`Using https://… (from GONE_SERVER), not the configured server https://….`).

### Which servers a link may reach

A link names its own server, and anyone can make a link that points anywhere. Opening a link sends a request that tells its server your IP address and when you opened it, so `get`, `status`, `revoke` and `reply` only contact a link's server if it is:

- the server you use, chosen exactly as for `send`: `--server`, `GONE_SERVER`, the saved config, then `https://gone.hauken.us`; or
- named with `--allow-origin ORIGIN` on this command. Repeat the flag or separate origins with commas. Each value must be a bare origin such as `https://gone.example.com`; it applies to this command only.

Any other link is refused before anything is sent, with exit 2 (JSON error kind `untrusted_origin`). The message names both servers, never the link's ID or key:

```text
gone: refused: this link is for https://gone.example.com, but no server is set, so gone only trusts the default https://gone.hauken.us; nothing was sent. If you trust https://gone.example.com, add --allow-origin https://gone.example.com for this command, or make it your server with: gone set server https://gone.example.com
```

If you use your own server, run `gone set server https://your.server` once and its links just work. Origins are compared after lowercasing and dropping a default port, so `https://Gone.Example:443` and `https://gone.example` are the same server.

### Transport

The standard proxy variables (`HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY`) are honored for every `https://` request. An `http://` server (below) is always contacted directly, never through a proxy.

- **HTTPS only.** `http://` origins and links are refused on every command. The one exception is a server on this machine, named `localhost` or a literal loopback address (`127.0.0.0/8` or `::1`), with `--insecure`, for local testing; browsers make the same exception. Other names, including `*.localhost`, are looked up in DNS like any host, so they are refused. Any other `http://` server is refused even with `--insecure`.
- **No redirects.** A redirect is treated as a server error, so a secret is never sent to, or fetched from, an origin other than the one you named.
- **Bounded responses.** Responses larger than the protocol allows are rejected before they are read.

---

## Exit codes

| Code | Meaning                                                                |
| ---: | ---------------------------------------------------------------------- |
| 0    | Success                                                                |
| 1    | Internal error, or interrupted (Ctrl-C)                                |
| 2    | Usage error: bad flag, bad link, bad server URL, weak passphrase, a link for a server you don't use (`untrusted_origin`), or input the server's policy refuses (`policy`) |
| 3    | Not found: already opened, revoked, expired, or never existed; for `request open` without `--wait`, no reply yet |
| 4    | Wrong passphrase (or the link is damaged); the secret is lost          |
| 5    | Rate limited by the server                                             |
| 6    | Integrity failure: the ciphertext or payload failed authentication     |
| 7    | Network or server error, response too large, or deletion not confirmed |
| 8    | Local I/O error: reading input files, writing attachments or config, or an unsafe config file |

The server cannot tell "already opened", "revoked" and "expired" apart from the outside, so they share code 3. A request with no reply yet is also exit 3, with error kind `not_ready` in JSON, so a script can tell it from `not_found`.

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

`request`:

```json
{"id":"ID","link":"https://…/reply/ID#v3:PUBLIC_KEY.FILL_TOKEN","label":"staging DB","expires_at":"2026-08-10T18:30:00Z"}
```

`request --wait` prints **two lines**: the line above as soon as the request is made, then the `request open` result once the reply arrives.

`request list` (an array, newest first; `state` is `waiting`, `ready`, `gone` or `unknown`):

```json
[{"id":"ID","label":"staging DB","state":"ready","server":"https://gone.hauken.us","created_at":"…","expires_at":"…","link":"https://…/reply/ID#v3:…"}]
```

`request open`: the same as `get` (or `get --message-out`). `request cancel`: `{"cancelled":true}`. `reply`: `{"sent":true,"expires_at":"…"}`.

Errors are one line of JSON on **standard error**, and the exit code is still set:

```json
{"error":"rate_limited","message":"rate limited; retry after 30s","retry_after":30}
```

| `error`            | Exit |
| ------------------ | ---: |
| `internal`         | 1    |
| `interrupted`      | 1    |
| `usage`            | 2    |
| `untrusted_origin` | 2    |
| `policy`           | 2    |
| `not_found`        | 3    |
| `not_ready`        | 3    |
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
- **What `gone` never prints.** Error messages and reports never contain the plaintext, a passphrase, a link's key or `#fragment`, a manage token, a request's private key, or the secret ID. Network errors name the server's origin, not the request path. The only exceptions are the outputs you ask for: the message from `get` or `request open` without `--message-out`, the links from `send`, the request link from `request` and `request list --json` (it can't open anything), and a `--passphrase-generate` passphrase. Values you type as arguments are your own exposure (they are already in `ps` and history), and `gone` may repeat a mistyped flag value in a usage error.
