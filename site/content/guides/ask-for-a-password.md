---
title: How to ask someone for a password
seoTitle: "How to ask someone for a password, securely"
description: "Send a request link instead of asking for a password in chat. They type the secret into it, it's encrypted to your browser, and only you can open it, once."
date: 2026-10-24
---

<!-- Draft for review. -->

# How to ask someone for a password

**Make a request link on Gone and send it to the person who has the password.** They type the secret into that page, it's encrypted so that only your browser can open it, and you open it once. They don't need an account or anything installed.

## Why ask this way

Asking "can you send me the password?" in chat usually gets the password pasted into chat, where it stays. A request link gives them a better place to put it, without you having to explain encryption or a tool. Many secret-sharing tools can only send; with Gone you can ask too.

## In the browser

1. Open [app.getgone.dev/request](https://app.getgone.dev/request) (**Request a secret** in the app).
2. Add a **Label** if you like, such as "staging database password". Only you see it; it stays on your device.
3. Under **Wait for a reply**, choose how long the link should work.
4. Press **Create request link** and copy the **request link**.
5. Send the request link to the person who has the password. It's fine to send it in chat or email: the link can't open anything, it only lets them send you one reply. Make sure it reaches the right person.
6. They open the link, paste the password and press **Send securely**. It's encrypted in their browser, to a key that only exists in yours.
7. Come back to **Request a secret** in the same browser. When the reply has arrived, open it. It opens once, then it's gone.

The key that opens the reply never leaves the browser that made the request, so use a normal window (private windows often can't keep it), and open the reply in the same browser.

{{< cta request="true" >}}

## From a terminal

The [gone CLI](/docs/cli/) can ask, wait and open the reply in one command:

```sh
gone request --label "staging database" --wait
```

It prints the request link at once. Send it on, and when the reply arrives `gone` prints it and deletes it from the server. Without `--wait`, check later:

```sh
gone request list        # which requests have a reply
gone request open 3f9a   # open one (by ID or a short prefix), once
```

The person answering can use the browser, or the CLI:

```sh
printf 'hunter2' | gone reply '<request-link>'
```

## If you're the one asked

A request link is easy to make, so anyone can make one. Only answer a request that you know came from the person you expect.

Read more in the [FAQ](/docs/faq/) and the [security model](/docs/security/).
