# Frequently asked questions

Short answers about Gone, each linked to the page that explains it in full.

## Will a chat app's link preview use up my secret?

No. A preview fetches the page, but the secret is only downloaded when the recipient presses Open, and the key after the `#` is never sent to a server anyway. ([Security model](security.md#opening-it))

## What happens if nobody opens it?

It's deleted when it expires. You pick the expiry when you send it, from five minutes to a day by default. ([Configuration](configuration.md))

## Can whoever runs the server read my secret?

No. The server only ever has the encrypted secret; the key is in the link, after the `#`, which browsers never send. ([Security model](security.md))

## What if someone else gets the link first?

They can open it, and then it's gone: your recipient will find nothing, which tells you the link leaked. Add a passphrase and send it another way, and the link alone isn't enough. ([Threat model](security.md#threat-model))

## Can I take a secret back?

Yes, until it's opened. The manage link you get when you send it can delete it.

## Do I need an account?

No. There are no accounts. An operator can require sign-in through their own sign-in proxy to create secrets, but Gone never records who. ([Sign-in for private instances](sign-in.md))

## Can I use it from a terminal or a script?

Yes. The `gone` CLI sends and opens secrets, works with links made in the browser, and lets scripts handle secrets without printing them. ([The gone CLI](cli.md))

## Do I have to run it in Docker?

No. Gone is one static binary. Run it in Docker, on Kubernetes, or from a Debian or RPM package with a systemd service. ([Self-hosting Gone](deploy/README.md))

## Is it really free?

Yes. Every feature works on every instance, under the AGPL-3.0 licence. There's no paid tier and none planned. ([Rejected features](rejected.md))

## Can I try it without installing anything?

Yes: the public instance at gone.hauken.us is open to anyone. It's run by the maintainer with no uptime guarantee, so for anything important, run your own.
