---
title: Choosing a secret-sharing tool
description: How to tell whether a secret-sharing tool really encrypts, and whether it's looked after.
---

<!-- Draft for review. -->

# Most tools say they're secure. Here's how to check.

A secret-sharing tool asks for a lot of trust: you paste a password into it and hope. You shouldn't have to hope. These are the questions worth asking of any tool, Gone included, and how to answer them yourself.

## Is it really encrypted?

1. **Look at the link.** Is there a long random key after a `#`? Browsers never send that part to the server, so a key there stays between the sender and the recipient.
2. **Watch it send.** Your browser's developer tools show what leaves the page. You should see only scrambled data, never your secret.
3. **Read what the server stores.** In an open-source tool, check whether the server's code could decrypt what it keeps. If the server holds the key, it can read every secret.

## "The database is encrypted"

It's a common answer about tools that encrypt on the server, and it answers a different question. Encryption at rest, like a database's transparent data encryption, protects the disk: someone who steals a drive or a backup tape can't read it. But the database decrypts the data as it reads it, for whatever asks: the application, the server, its administrators, and anyone who breaks into the app.

Keeping data from the people who run the system means encrypting it before it ever reaches them. Microsoft's documentation describes exactly that: encrypting in the client application, to keep data from "those who manage the data but should have no access". That's what Gone does: the secret is encrypted in your browser, and the server never has the key.

## Is it looked after?

1. **When was the last release?** Months without one is a warning sign.
2. **Are its dependencies kept current,** and does it have tests?
3. **Does it have a security policy,** and does it publish its fixes?
4. **Can you check how a release was built?** Signed build provenance proves a download came from the project's own source.

## One thing every browser tool shares

In any tool that encrypts in the browser, Gone included, the code that does the encrypting comes from the server. A malicious or compromised server could send different code. If that matters to you, run your own instance, or use a command-line client, which encrypts with code you installed yourself.

## How Gone answers these

- **The key is in the link,** after the `#`, and the server only ever stores ciphertext. [How it works](/docs/security/)
- **The protocol is written down** in a [specification](/docs/protocol/), with test vectors the browser and CLI are tested against.
- **It's maintained in the open,** with a [security policy](/docs/security-policy/) and every release signed with build provenance you can verify. [Security and trust](/trust/)
- **The CLI encrypts on your machine,** and works with links made in the browser.

## Other good tools

Gone does one thing: hand over a secret exactly once. If you need something different, these are well-regarded open-source projects that also encrypt in your browser:

- **[PrivateBin](https://privatebin.info/)**, an encrypted pastebin with discussions, syntax highlighting and a large community of public instances.
- **[Yopass](https://yopass.se/)**, a long-established one-time secret tool built on the OpenPGP standard.

## Sources

- Microsoft Learn: [Transparent Data Encryption](https://learn.microsoft.com/en-us/sql/relational-databases/security/encryption/transparent-data-encryption) and [Always Encrypted](https://learn.microsoft.com/en-us/sql/relational-databases/security/encryption/always-encrypted-database-engine).
- AWS: [Encrypting Amazon RDS resources](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Overview.Encryption.html).
