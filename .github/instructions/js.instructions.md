---
applyTo: '**/*.js'
---
# JavaScript Authoring Guidelines for Gone

Purpose: Keep frontend code small, secure, maintainable, dependency‑light, and friendly to static analysis.
Target Runtime: Evergreen modern browsers (Chromium, Firefox, Safari) – ES2020+ features allowed when natively supported without polyfills.
ECMAScript Baseline: ES2022 (`.eslintrc.json` `ecmaVersion`). Later features may be used only when the ESLint ruleset and all target browsers support them without polyfills. Always `use strict` in non-module files. Forbid `eval`, `new Function`, and dynamic imports from untrusted sources.
Files: `web/js/` holds small single-responsibility scripts (no bundler). Each is an IIFE that exposes at most one `window.gone*` namespace; load order is set by the `<script>` tags in `web/*.tmpl.html` (e.g. `util.js` → `fileMeta.js` → `crypto.js` → `envelope.js` → flow modules → `submit.js` / `consume.js`).
Tests: zero-dependency `node:test` suites live in `test/js/` (run `task test-js`). Every new or changed module needs tests; keep coverage at 100% lines.

## Core Principles
1. Security First: Never send plaintext secrets or keys to the server; key only lives in the URL fragment. Avoid logging sensitive data. Keep crypto primitives minimal (AES‑256‑GCM via WebCrypto only).
2. Minimalism: No bundler or build-time module system. Split into a new file only when it materially reduces complexity or groups a single responsibility.
3. Stability: Avoid experimental APIs unless widely shipped. No external JS dependencies unless a critical security or standards need.
4. Deterministic Refactors: Any complexity or size reduction should preserve observable behavior and timing logging semantics.
5. Accessibility: Maintain focus management and semantic elements (e.g., `h2`, `button`, ARIA labels). Do not remove accessible names for brevity.

## Avoid XSS Risks
- Never manipulate the DOM with untrusted input via `innerHTML` or similar. Prefer `textContent` or explicit element creation.
- Do not use `innerHTML`, `outerHTML` or `insertAdjacentHTML`, even for static markup. Build icons with `window.goneIcons.make()` (DOM-built SVG) and set mixed content with `goneUtil.setContent()`.

## Language & Syntax
- Use `const` by default; use `let` only when reassignment is required. NEVER use `var`.
- Avoid global symbol leakage: wrap logical areas in IIFEs or module scopes when we migrate to ES modules.
- Prefer early returns over deep nesting.
- Destructure only when it increases clarity (e.g., options objects). Avoid over‑destructuring mid‑expression chains.
- Template literals for string assembly (especially URLs) unless trivial concatenation with `+` improves clarity.
- Use strict equality `===` / `!==`; no loose equality.
- Optional chaining only if it meaningfully reduces guard noise without hiding failures (currently minimized until remote tooling updated).
- Do not extend built‑ins or monkey‑patch globals.

## Functions & Complexity
- Soft caps: Function length < 60 lines, cyclomatic complexity < 8. Exceeding requires refactor or documented exception.
- Decompose by responsibility: preparation, crypto, network, UI update, cleanup.
- Keep small pure helpers stateless. UI helpers may close over minimal DOM refs.
- Name async functions with verbs reflecting intent (`encryptSecret`, `uploadCiphertext`, `performUpload`).

## Performance & Memory
- Zero sensitive plaintext as soon as no longer needed (`secureWipe`).
- Avoid retaining secret data in closures, attributes, or logs.
- Avoid unnecessary array copies; operate on `Uint8Array` directly.
- Prefer explicit loops over regex for performance‑sensitive or security‑sensitive transforms (base64 padding trim).

## Cryptography Handling
- AES‑GCM with 12‑byte nonce and 32‑byte key. AAD is `gone:v1` for v1; for v2 (passphrase) it is `gone:v2` plus the 21‑byte KDF header, and the key comes from HKDF over the link key and PBKDF2 of the passphrase (`cryptoV2*.js`, `docs/protocol.md` §4.2).
- Fresh nonce per encryption. Never reuse.
- Key exported as unpadded base64url into fragment `#v<version>:<key>`; never transmitted to server.
- Enforce key length on import; throw early on mismatch.
- Verify version header before decrypt.

## DOM & UI
- Query elements once per flow; cache references.
- Use `textContent` or DOM nodes; never parse HTML strings.
- Avoid constructing large interpolated HTML strings with dynamic user input.
- Manage focus after dynamic panel insertion.
- Keep imperative DOM creation for auditability (explicit `createElement` sequence).

## Error Handling & Logging
- Log with concise, greppable tags: `[gone]`, `[gone][timing]`.
- Always update user‑visible status on recoverable failure.
- Never log plaintext secret or key material.

## Network Calls
- Explicit headers: `X-Gone-Version`, `X-Gone-Nonce`, `X-Gone-TTL` on create; `X-Gone-Claim` on claim retry and acknowledge; `X-Gone-Manage` on status and revoke. Tokens only ever go in headers, never in URLs or bodies.
- Treat non‑2xx as failure; re‑enable UI with a slight delay for UX clarity.
- Ciphertext body is raw bytes; no JSON wrapping.

## Accessibility & UX
- Preserve accessible names and ARIA where required.
- Provide clear copy feedback and restore previous label after timeout.
- Use semantic elements (`button`, `time`, `p`, `h2`).

## Tooling & Static Analysis
- Rely on repo ESLint config; avoid inline disables. If required, add `// EXCEPTION:` with justification.
- Maintain timing log labels stable for simple performance regression tracking.

## Code Style Conventions
- Single quotes for JS literals; double quotes only inside embedded SVG/HTML attributes.
- camelCase for functions/variables; UPPER_CASE only for stable constants (currently minimal).
- Group related helpers and separate submit vs consume flows with comment banners.

## Refactor Playbook
1. Identify metric breach (length/complexity).
2. Extract helper(s) with clear name.
3. Insert calls; remove duplicated logic.
4. Re-run analysis; manual smoke (create, copy, consume).
5. Document exception if still above threshold.

## Migration Path (Optional Future)
- Phase 1: Split by responsibility into IIFE scripts (done; see `web/js/`). Move to ES modules if the script graph grows further.
- Phase 2: Add JSDoc typedefs for encrypted payload structure.
- Phase 3: Consider TypeScript if surface grows substantially (>3 modules or complex state machine).

## Anti‑Patterns to Avoid
- Reintroducing `var`.
- Global mutable state for timing or counters (prefer local logs).
- Storing secrets in `localStorage`, `sessionStorage`, or query params.
- Silent catch blocks without user feedback.
- Large HTML template strings with dynamic insertions.

## Pre-Commit JS Checklist
- [ ] No plaintext/key lingering after submit
- [ ] No `var` usages
- [ ] Function sizes & complexity within limits or justified
- [ ] Crypto invariants preserved (nonce fresh, key length 32, AAD unchanged)
- [ ] Accessibility unaffected (focus + labels)
- [ ] Timing logs intact
- [ ] ESLint & Codacy clean

