# Contributing to Gone

Thanks for helping. Gone is small on purpose, and its security depends on that, so this page explains how changes get in and what they need to meet.

## Reporting bugs and requesting features

- **Bugs and feature requests:** open an issue at <https://github.com/haukened/gone/issues/new/choose>. The templates ask for what we need: the version (`gone version`, or the image tag), what you did, what you expected, and what happened.
- **Security problems:** do **not** open a public issue. Report them privately as described in [SECURITY.md](SECURITY.md).
- **Questions and ideas:** an issue is fine. Search the [existing issues](https://github.com/haukened/gone/issues?q=is%3Aissue) first; they're public and searchable.

We aim to acknowledge new issues within 14 days. Not every request will be accepted. The [non-goals in the roadmap](docs/ROADMAP.md#non-goals) (accounts, push notifications, secrets that open more than once, front-end frameworks, WebAssembly) are deliberate.

## How a change gets in

1. **Talk first for anything big.** For a new feature, a protocol change, or a new dependency, open an issue before writing code so we can agree on the approach.
2. **Fork and branch.** Work on a branch in your fork, named for the change (for example `fix/claim-timeout`).
3. **Commit with signed commits.** `main` only accepts signed commits ([GitHub's guide](https://docs.github.com/authentication/managing-commit-signature-verification/signing-commits)). Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/): `fix(cli): ...`, `feat: ...`, `docs: ...`, `ci: ...`, `test: ...`.
4. **Open a pull request** against `main` and fill in the template. Keep each PR to one change.
5. **Pass the checks.** Every PR runs lint, tests with the race detector, CodeQL, Codacy (static analysis and coverage), and the latency check. All required checks must pass, and the branch must be up to date with `main`.
6. **Merge.** A maintainer merges with a merge commit.

## Requirements for acceptable contributions

### Code
- **Go:** idiomatic Go, formatted with `go fmt`. `task lint` (golangci-lint with `.golangci.yml`) must report no issues.
- **GoDoc on every function**, including what the parameters and return values mean.
- **Errors:** wrap with `%w`; no panics in HTTP handlers.
- **JavaScript:** vanilla JS only. No frameworks, no build step, no `innerHTML`, nothing loaded from another origin; the strict CSP must keep working. `task lint-web` (ESLint and stylelint) must pass with no warnings.
- **Dependencies:** prefer the Go standard library and WebCrypto. A new dependency needs a written reason in the PR.
- **Never** log or store plaintext, keys, passphrases, or tokens, and never give the server anything that would let it decrypt a secret.

The full style rules are in [.github/instructions/default.instructions.md](.github/instructions/default.instructions.md) (Go) and [.github/instructions/js.instructions.md](.github/instructions/js.instructions.md) (JavaScript).

### Tests
**New functionality needs tests in the same PR.** That's our test policy, and reviewers will ask for them.
- Go unit tests go next to the code (`foo.go` → `foo_test.go`, same package), table-driven where it fits. Integration tests go in `test/`.
- JavaScript tests go in `test/js/` and use `node:test` with no dependencies.
- Go and JS line coverage are kept at or near 100%; don't lower it.
- Changes to the encryption format must update the shared vectors in `test/vectors/`, which both the Go and JS suites check.
- Bug fixes should include a test that fails without the fix.

Run everything before pushing:

```sh
go fmt ./...
task test        # Go and JavaScript tests
task lint        # golangci-lint
task lint-web    # ESLint and stylelint
```

### Text in the web pages
Never write user-visible text straight into a template or script. Add a message to [web/messages/en.json](web/messages/en.json) and to every other catalog, then use its key: `{{ t "key" }}` with a matching `data-i18n="key"` in templates, and `goneI18n.set(node, 'js.key', args)` in scripts. The tests fail on a key that is missing or unused, or on a catalog whose placeholders differ from English. See [Translations](#translations).

### Documentation
Update the docs in the same PR when behavior changes: the message catalogs (any wording), the README (configuration, metrics), [docs/README.md](docs/README.md) and [docs/openapi.yaml](docs/openapi.yaml) (HTTP API), [docs/protocol.md](docs/protocol.md) (wire format), and [docs/cli.md](docs/cli.md) (CLI).

## Translations

The web pages are translated through one catalog per language in [web/messages/](web/messages/): `en.json` (the source), `es.json`, `fr.json`, `de.json` and `pt-BR.json`. They use the [inlang](https://inlang.com) message format, so inlang tools such as Fink can edit them. The translations other than English were made by machine, so native speakers' fixes are very welcome.

**Fix a translation:** edit the message in that language's file and open a PR. [web/messages/README.md](web/messages/README.md) has the conventions and each language's glossary. Keep:
- every `{placeholder}` exactly as in English, such as `{count}` or `{max}`;
- every `{#slot}…{/slot}` pair, which wraps a link, emphasis or code. Translate the words inside it, and move it to wherever it reads naturally;
- the plural cases: `one` and `*` (other), plus `many` where the language uses it.

**Add a language:** add the locale (BCP 47 tag, its own name, text direction, number separators and plural rule) to `known` in [internal/i18n/locale.go](internal/i18n/locale.go), then add `<tag>.json` with every key from `en.json`. Run `task test`: it checks every catalog against English and renders every page in every language.

**Check the layout:** development builds (`task run`) have a pseudo-language, `en-XA`, in the language picker. It shows every message accented, bracketed and about a third longer, so untranslated text stands out and layouts that would clip a longer language show it.

## Releases

Releases use [Semantic Versioning](https://semver.org/) and are tagged `vMAJOR.MINOR.PATCH`. Every release has notes on its [GitHub release page](https://github.com/haukened/gone/releases) that summarize the changes and any upgrade steps. If a release fixes a publicly known vulnerability, the notes name it (with its CVE or GHSA ID).

## License

Gone is licensed under the [GNU AGPL v3](LICENSE). By contributing, you agree that your contribution is licensed under the same terms. The name and logo are covered separately by [TRADEMARKS.md](TRADEMARKS.md).
