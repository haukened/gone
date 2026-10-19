# Gone website

The source of the Gone website: the pages people read before they install
Gone. The plan is in the pull request; in short:

- **Pages:** Home, Why Gone, Choosing a tool, Install, Security & trust,
  Privacy, and the full documentation, which the build renders from
  `docs/` at the latest release.
- **Look:** the app's own design system (`DESIGN.md`, `web/css/tokens.css`).
- **Hosting:** static files on GitHub Pages, with no cookies, analytics or
  third-party requests.

**Status: copy draft.** `content/` holds the English copy for review. The
generator (`tools/site`), templates and deploy workflow come after the copy
is agreed. Nothing here is built or published yet.
