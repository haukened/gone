# Rejected features

Things people reasonably ask Gone for that we've decided not to build, and why. It exists so a request gets a clear answer instead of silence, and so the same idea isn't re-argued from scratch.

A "no" here isn't forever unless it says **Never**. If a revisit trigger has happened, or you have a reason the entry doesn't address, open an issue that names the entry and what changed.

**Never** means the feature would break one of Gone's promises:
- nobody, including the server, can read your secret but the person you send it to;
- a secret opens once;
- once it's gone, nothing about it is kept.

This page is about features: what you'd be able to do. How Gone is built, for example which algorithms or libraries it uses, is explained in [ROADMAP.md](ROADMAP.md) and [protocol.md](protocol.md).

## Sending and receiving

| Feature | Decided | Why not | Revisit when |
| ------- | ------- | ------- | ------------ |
| **Get notified when a secret is opened** (by email, push or webhook) | 2026-10-02 | Gone would have to keep your contact details and a record tying you to the secret. Your manage link tells you whether it's still waiting, whenever you check. | Never |
| **See that a secret was opened**, as opposed to expired or deleted | v3.0.0 | Telling those apart means keeping a record after the secret is gone. The manage link only ever says "still waiting" or "gone". | Never |
| **Let a secret be opened more than once**, by several people, or edited after sending | 2025-09 | Opening once is the point: it's how you know nobody else saw it first. Send one link per person instead. | Never |
| **Lock a secret after too many wrong passphrases** | v3.0.0 | Someone who has the link already has the encrypted copy and can keep guessing on their own computer, so a lock would only get in the way of the real recipient. Use a strong passphrase; the **Generate** button makes one. | Never |
| **Formatted messages** (Markdown, code highlighting) | 2026-10-05 | Gone delivers exactly what was sent, as plain text. Formatting tools already exist, such as PrivateBin. | Never |
| **A QR code for request links** | v3.8.0 | A QR code hides where it leads, so it's easy to swap for a fake page. That's riskiest for a link that asks someone to type in a secret. Send request links as text, where the address can be checked. | Never |
| **A QR code that matches dark mode** (light on dark) | v3.8.0 | Many phone cameras can't read inverted codes. It's always black on white. | Never |

## Accounts and history

| Feature | Decided | Why not | Revisit when |
| ------- | ------- | ------- | ------------ |
| **Accounts and sign-in**, including teams and an admin dashboard | 2026-10-02 | Anyone can send without signing up, and Gone keeps no record of who sent what. An account is that record. | Never |
| **A history of secrets** sent, opened or expired | 2026-10-05 | Even without the contents, a history proves a secret existed after it's gone. Operators can see overall counts in [metrics](metrics.md). | Never |
| **A paid plan** or premium features | 2026-10-05 | Security shouldn't live behind a paywall. Everything Gone does is free on every instance. | Never |

## Command line and AI tools

| Feature | Decided | Why not | Revisit when |
| ------- | ------- | ------- | ------------ |
| **Use Gone from AI chat apps** (an MCP server) | 2026-10-16 | Coding assistants can already run the `gone` CLI, which handles secrets by file so they never appear in the conversation. In a chat app, the secret would usually be pasted into the chat first, which defeats the point. | AI tools that can't run commands become common |
| **Type the secret on the command line** (`gone send "hunter2"`) | v3.0.0 | It would be saved in your shell history and visible to other programs on the machine. Pipe it in, or use `--message-file`. | Never |
| **Overwrite existing files** when saving attachments | v3.0.0 | `gone get` never replaces a file you have; it saves under a free name instead, which is just as easy to script. | Never |

## Running an instance

| Feature | Decided | Why not | Revisit when |
| ------- | ------- | ------- | ------------ |
| **Show visitors whether an instance is safe to use** (a security status page) | 2026-10-15 | A page that an instance serves about itself proves nothing: a compromised one would just say it's fine. Every page already shows the running version in its footer. | Dishonest instances become a real concern; then a CLI check, run on your own machine, that compares an instance against the signed release |
| **Store secrets in cloud storage** (S3 and similar) | 2026-10-05 | A single database file and folder handles the load with lots of room to spare, and makes deleting a secret simple and certain. | Instances outgrow it in practice |
