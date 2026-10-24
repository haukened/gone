---
title: How to share a password securely
seoTitle: "How to share a password securely (without chat or email)"
description: "Don't paste a password into chat or email, where it stays. Send a one-time link instead: encrypted in your browser, opened once, then deleted."
date: 2026-10-24
---

<!-- Draft for review. -->

# How to share a password securely

**Don't paste it into chat or email, where it stays. Send a one-time link instead:** the password is encrypted in your browser, the link opens once, and then the password is deleted. If you add a passphrase, send it a different way.

## Why chat and email aren't safe for passwords

A password you paste into chat or email doesn't go away when the other person has used it. It stays:

- in the message history, on every device that synced it;
- in search indexes and backups;
- in front of anyone who later gets into either account, or picks up an unlocked laptop.

You can't take it back, and you can't tell whether anyone else has read it.

## What a one-time link does instead

- **Only the recipient can read it.** The password is encrypted in your browser before it's sent. The key travels in the link, after the `#`, which browsers never send to a server, so the server only ever stores something it can't read.
- **It opens once.** The first person to open the link sees the password. The server deletes it as soon as their browser confirms it arrived. Anyone after them finds nothing.
- **Nothing is kept.** If nobody opens it, it's deleted when it expires. There's no copy, no history and no record that it was ever opened.
- **You'll know if it leaked.** If someone else opens the link first, your recipient finds it gone, and you know to change the password.

## Share a password with Gone, step by step

1. Open [app.getgone.dev](https://app.getgone.dev), the free public instance, or your own server.
2. Paste the password into **Message**. You can add a note, such as the username, and attach files too.
3. Under **Delete unopened after**, choose the shortest time that works. The public instance offers five minutes to a day.
4. For anything valuable, open **Add a passphrase**. Type one, or press **Generate** for five random words.
5. Press **Create one-time link**.
6. Copy the link and send it to the person who needs the password. If you set a passphrase, tell them by a different route, such as a phone call or another app, never in the same message as the link.
7. Keep the **manage link** for yourself. It shows whether the secret has been opened, and lets you delete it early. It can never open the secret.

{{< cta >}}

## From a terminal

The [gone CLI](/docs/cli/) does the same from a terminal, and its links open in a browser:

```sh
printf 'hunter2' | gone send --ttl 30m --passphrase-generate
```

The password comes from standard input, never from an argument, so it doesn't end up in your shell history. `--passphrase-generate` prints a five-word passphrase to send separately.

## Good habits

- **Use the shortest expiry that works.** A link that expires in 30 minutes is a much smaller risk than one that lasts a day.
- **Add a passphrase for anything that matters,** and send it another way. A leaked link alone then isn't enough.
- **If your recipient finds the link already opened, assume the password leaked** and change it.
- **Change shared passwords when people leave.** A one-time link keeps the handover safe; it can't make an old password private again.

Read more in the [security model](/docs/security/) and the [FAQ](/docs/faq/).
