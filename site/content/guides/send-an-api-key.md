---
title: How to send an API key or SSH key to a teammate
seoTitle: "How to send an API key or SSH key to a teammate"
description: "Send an API key, token or key file as a one-time link that's encrypted in your browser, then revoke it if plans change. From the web or the terminal."
date: 2026-10-24
---

<!-- Draft for review. -->

# How to send an API key or SSH key to a teammate

**Paste the key, or attach the key file, on Gone, pick a short expiry, and send your teammate the one-time link.** It's encrypted before it leaves your machine and deleted once they've opened it. From a terminal, `gone send` does the same with files.

## Before you send a private key

An API key or token often has to be shared. A private SSH key usually doesn't: if you can, ask your teammate to make their own key pair and send you the **public** key, which isn't secret, and add that where it's needed. Share a private key only when nothing else will do, and replace it afterwards.

## In the browser

1. Open [app.getgone.dev](https://app.getgone.dev), or your own server.
2. Paste the key or token into **Message**, with a line saying what it's for. To send key files, press **Add files**: up to ten files, encrypted with the message (the public instance allows 10 MB in all).
3. Choose a short time under **Delete unopened after**, such as 30 minutes or an hour.
4. Press **Create one-time link** and send the link to your teammate.
5. Keep the **manage link**. If you sent it to the wrong person, or plans change, open it and press **Delete now**: the link stops working at once.

{{< cta >}}

## From a terminal

Send a key from a file, so it never passes through your clipboard or shell history:

```sh
gone send --message-file ./api-key.txt --ttl 1h </dev/null
```

Send key files as attachments, protected with a generated passphrase you pass on separately:

```sh
gone send -f id_ed25519 -f id_ed25519.pub --passphrase-generate --ttl 2h </dev/null
```

Each command prints the link to send, and a manage link for you. To take the secret back before it's opened:

```sh
gone revoke '<manage-link>'
```

Your teammate can open the link in a browser, or with the CLI:

```sh
gone get '<link>'
```

Attachments are saved in the current directory. An existing file is never overwritten: a second `id_ed25519` is saved as `id_ed25519 (1)`.

## After it's sent

- **Check it was your teammate who opened it.** If they say the link was already used, someone else may have it: revoke the key and make a new one.
- **Rotate keys that more than one person has held,** once the work that needed sharing is done.

More options, such as JSON output for scripts, are in the [CLI reference](/docs/cli-reference/).
