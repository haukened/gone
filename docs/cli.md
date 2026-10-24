# The `gone` CLI

`gone` sends and opens one-time secrets from a terminal, and can ask someone for a secret or answer such a request. It speaks the same protocol as the web UI ([protocol.md](protocol.md)), so a secret sent from the browser can be opened with the CLI and the other way round, and a request made in either can be answered from the other.

Everything is encrypted and decrypted on your machine. The server only ever stores ciphertext, and the key travels in the link's `#fragment`, which is never sent to the server.

The server binary is `goned`. You only need `gone` to send and receive secrets. Every command and flag is in the [CLI reference](cli-reference.md).

## Install

### Install script (Linux and macOS)

```sh
curl -fsSL https://getgone.dev/install.sh | sh
```

The script:
- **Downloads** the archive for your system from the latest [GitHub release](https://github.com/haukened/gone/releases).
- **Checks the checksum** against the release's `SHA256SUMS`, and stops without installing anything if it doesn't match.
- **Checks the build provenance** with `gh attestation verify` when the [GitHub CLI](https://cli.github.com/) is installed and signed in. Otherwise it says the check was skipped and how to run it.
- **Installs** `gone` into `~/.local/bin`, without sudo.

Options, given after `sh -s --`:
- `--version vX.Y.Z` installs that release instead of the latest.
- `--dir DIR` installs somewhere else.

Running the script again updates `gone`.

If you'd rather read a script before running it, download it first:

```sh
curl -fsSLO https://getgone.dev/install.sh
less install.sh
sh install.sh
```

The script's work happens in a function called on its last line, so a download that's cut off partway runs nothing.

### Homebrew (macOS and Linux)

```sh
brew install haukened/gone/gone
```

This installs the same release archive as the script, checked against the release's `SHA256SUMS`, from the [haukened/homebrew-gone](https://github.com/haukened/homebrew-gone) tap. Each stable release updates the tap, and `brew upgrade` picks it up.

### Packages (Debian, Ubuntu, Fedora, RHEL)

Each release has `.deb` and `.rpm` packages for amd64 and arm64. Download one from the [release page](https://github.com/haukened/gone/releases), then:

```sh
sudo apt install ./gone_3.13.0_amd64.deb       # Debian, Ubuntu
sudo dnf install ./gone-3.13.0-1.x86_64.rpm    # Fedora, RHEL
```

The `gone` package holds only the CLI. The server is the separate `gone-server` package; see [Running Gone from a package](deploy/packages.md). The packages aren't in a repository yet, so `apt upgrade` won't update them: install the next release's package the same way.

### Archives

Each release also contains static binaries (no libc dependency):

| OS      | Architectures  | Archive                           |
| ------- | -------------- | --------------------------------- |
| Linux   | amd64, arm64   | `gone_<tag>_linux_<arch>.tar.gz`  |
| macOS   | amd64, arm64   | `gone_<tag>_darwin_<arch>.tar.gz` |
| Windows | amd64, arm64   | `gone_<tag>_windows_<arch>.zip`   |

The release also contains:
- **`SHA256SUMS`**, covering every file.
- **The `goned` server archives** for Linux.
- **The signed build provenance** in two forms: `gone_<tag>.sigstore.json` (a Sigstore bundle, for offline checks with `gh attestation verify ... --bundle`) and `gone_<tag>.intoto.jsonl` (the same statement as an in-toto DSSE envelope).

Check an archive before you run it: [Verify a release](verify.md#a-cli-archive) shows how.

### Build from source

```sh
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags)" -o gone ./cmd/gone
```

## Quick start

```sh
# Send: the message comes from standard input, never from the command line.
printf 'hunter2' | gone send --ttl 30m
#   Link:        https://app.getgone.dev/secret/AbC…#v1:…
#   Manage link: https://app.getgone.dev/manage/AbC…#…
#   Expires:     2026-08-10T18:30:00Z

# Open it (quote the link: see "Quoting links").
gone get 'https://app.getgone.dev/secret/AbC…#v1:…'
```

Send the **link** to the recipient. Keep the **manage link** yourself: it lets you check on the secret or delete it, but it cannot open it.

Need a secret from someone instead?

```sh
gone request --label "staging DB"
#   Link:    https://app.getgone.dev/reply/3f9a…#v3:…
#   ID:      3f9a1c2e…
#   Expires: 2026-08-10T18:30:00Z

# They answer in a browser, or with: printf 'hunter2' | gone reply '<link>'
gone request list             # see which requests have a reply
gone request open 3f9a        # print the reply, then delete it
```

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
- **Exit codes** are listed in [Exit codes](cli-reference.md#exit-codes); with `--json`, errors are JSON on standard error.
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

## Quoting links

Always put links in **single quotes**:

```sh
gone get 'https://app.getgone.dev/secret/AbC#v1:XyZ'
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

## Next

- [CLI reference](cli-reference.md): every command and flag, which servers a link may reach, exit codes and JSON output.
- [Verify a release](verify.md): check that a `gone` download was built by this repository.
- [Security model](security.md): what Gone protects, and what it can't.
