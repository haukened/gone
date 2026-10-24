---
title: How to give secrets to scripts and AI agents
seoTitle: "How to give secrets to scripts and AI agents safely"
description: "Pass secrets to scripts and AI agents by reference: gone turns a file into a one-time link and a link back into a file, so the plaintext never hits logs."
date: 2026-10-24
---

<!-- Draft for review. -->

# How to give secrets to scripts and AI agents

**Pass secrets by reference, not by value.** `gone send --message-file` turns a file into a one-time link, and `gone get - --message-out` turns a link back into a file, so the secret never appears in a script's output, a CI log or an AI agent's context.

## Why plaintext in output is the problem

Scripts and agents record what they see: terminal output, CI logs, chat transcripts and an agent's context window. A secret printed once can be copied into all of them, and kept long after it's used. The [gone CLI](/docs/cli/) is built to keep the secret in files you control and pass only links around.

## Send a file's contents

```sh
gone send --message-file /root/.restic-password --ttl 1h --json </dev/null | jq -r .link
```

The message comes from the file, not from `cat`, so nothing secret flows through a pipe. Only the link is printed.

## Receive into a file

```sh
gone get - --message-out ./restic.pw --json < link.txt
```

The link is read from standard input, so it needs no quoting and stays out of the process list. The secret is written to a new file readable only by you, and never printed. The JSON result says only where it went and how big it is.

## Ask a person and wait

A script or agent that needs a credential it doesn't have can ask for it and wait:

```sh
gone request --label "prod DB for migration" --wait --message-out ./db.pw --json </dev/null > events.json &
head -1 events.json | jq -r .link   # hand this link to the person who has the secret
wait                                # returns once they've answered
```

The secret lands in `./db.pw` and never passes through the output. If the wait is interrupted, `gone request open ID --wait --message-out ./db.pw` picks it up again.

## Guidelines

- **Don't use `gone get --json` without `--message-out`.** Its `message` field is the plaintext.
- **No prompts.** With `--message-file` and `--passphrase-file`, `gone` never reads the terminal. Redirect standard input from `/dev/null` anyway, so nothing can wait on a terminal. On Windows, use `<NUL` in cmd, and in PowerShell, which has no `<`, pipe the link in: `Get-Content link.txt | gone get - …`.
- **Mistakes don't burn the secret.** Everything `gone get` can check locally (the link, the passphrase file and the output paths) is checked before the link is opened.
- **Only your own server by default.** A link for another server is refused before anything is sent, unless you pass `--allow-origin`.
- **Delete the file once it's been used.**

{{< cta >}}

The full list of flags, exit codes and JSON fields is in the [CLI reference](/docs/cli-reference/).
