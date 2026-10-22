---
title: Why Gone
description: What Gone promises, what it is, and why it was made.
---

<!-- Draft for review. -->

# Share a secret exactly once.

A password pasted into chat stays in chat. It sits in the history, the search index, the backups and every device that synced it, long after the person who needed it has used it. Gone gives that password a better route: a link that works once, to a secret only the recipient can read, which leaves nothing behind.

## What Gone promises

1. **Only the person you send it to can read it.** The server never sees the secret or its key. Not the server, not its operator, not us.
2. **It opens once.** A link opens its secret at most once.
3. **Once it's gone, nothing about it is kept.** No copy, no history, no record that it was opened.

These are promises in the strict sense: Gone is built so that breaking them would take a different design, not a different setting. [How Gone keeps them](/trust/)

## What it is

Gone is a small open-source service, and a command-line tool, for handing over passwords, tokens, keys and files.

- **Encrypted before it leaves your device.** Your browser or the CLI encrypts the secret with AES-256-GCM and a fresh random key. The key goes in the link, after the `#`, which browsers never send to a server.
- **The server holds only ciphertext.** It can tell how big a secret is and when it expires. It can't tell what it says.
- **Deleted once it arrives.** The recipient's browser decrypts the secret and confirms it, and only then does the server delete it. A secret nobody opens expires on its own.
- **You can ask for a secret too.** Send someone a link, and their reply is encrypted to a key that never leaves your browser.
- **One small program you run yourself:** a single static binary on SQLite, with no external services, in Docker, Kubernetes or a Linux package.

## Why we made it

<!-- DRAFT of the maintainer's own account, for them to rewrite in their voice. Two separate experiences: aging PHP apps, and home-grown corporate tools defended with "the database is encrypted". -->

Every time I came across an open-source password-sharing app, I ended up with more questions than answers.

Some were old PHP apps, written years ago and still running. I couldn't shake the feeling that their age showed in more than the code style: in how they handled secrets, in what had been tested, in whether anyone was still looking after them.

At work it was worse. IT teams used home-grown tools where there was no way to tell, from how they behaved, whether a secret was ever encrypted at all. When I asked, the answer was always some version of "well, the database is encrypted." That protects a stolen disk. It doesn't protect a secret from the server that stores it, from whoever runs that server, or from anyone who gets into the app.

So I built the tool I wanted to be able to trust. In Gone, the server never has the key, so it couldn't read a secret if it tried. That isn't a claim to take on faith: the encryption happens in your browser, the code is open, and every release can be traced back to the source it was built from.

And all of it is free. Requiring a passphrase, adding your own name and logo, requiring sign-in: everything Gone does works on every instance, with no paid tier.

**Because security shouldn't live behind paywalls.**

## What Gone will never do

- **No accounts and no history.** Gone doesn't know who you are, and keeps no list of what you've sent.
- **No read receipts and no "opened" state.** Keeping that record would mean keeping something after the secret is gone.
- **No paid plan.** Every feature is free on every instance.
- **No tracking.** No analytics, no third-party scripts or fonts, and server logs never record visitors' addresses or what they open.

Each of these is a decision, with its reasons, in [Rejected features](/docs/rejected/).

## Questions

### Will a chat app's link preview use up my secret?

No. A preview fetches the page, but the secret is only downloaded when the recipient presses Open, and the key after the `#` is never sent to a server anyway. ([Security model](/docs/security/#opening-it))

### What happens if nobody opens it?

It's deleted when it expires. You pick the expiry when you send it, from five minutes to a day by default. ([Configuration](/docs/configuration/))

### Can whoever runs the server read my secret?

No. The server only ever has the encrypted secret; the key is in the link, after the `#`, which browsers never send. ([Security model](/docs/security/))

### What if someone else gets the link first?

They can open it, and then it's gone: your recipient will find nothing, which tells you the link leaked. Add a passphrase and send it another way, and the link alone isn't enough. ([Threat model](/docs/security/#threat-model))

### Can I take a secret back?

Yes, until it's opened. The manage link you get when you send it can delete it.

### Do I need an account?

No. There are no accounts. An operator can require sign-in through their own sign-in proxy to create secrets, but Gone never records who. ([Sign-in for private instances](/docs/sign-in/))

### Can I use it from a terminal or a script?

Yes. The `gone` CLI sends and opens secrets, works with links made in the browser, and lets scripts handle secrets without printing them. ([The gone CLI](/docs/cli/))

### Do I have to run it in Docker?

No. Gone is one static binary. Run it in Docker, on Kubernetes, or from a Debian or RPM package with a systemd service. ([Self-hosting Gone](/docs/deploy/readme))

### Is it really free?

Yes. Every feature works on every instance, under the AGPL-3.0 licence. There's no paid tier and none planned. ([Rejected features](/docs/rejected/))

### Can I try it without installing anything?

Yes: the public instance at gone.hauken.us is open to anyone. It's run by the maintainer with no uptime guarantee, so for anything important, run your own.
