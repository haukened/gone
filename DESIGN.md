---
name: Gone
description: One-time secret sharing behind frosted glass. Moonstone: calm, private, premium.
colors:
  pearl: "#eeecf1"
  dusk: "#121019"
  ink-12: "#1c1a26"
  ink-11: "#4a4658"
  ink-10: "#625e73"
  moon-12: "#efedf6"
  moon-11: "#c3bed4"
  moon-10: "#a7a2ba"
  plum-12: "#211e30"
  plum-11: "#36324c"
  moonlight: "#dcd6f4"
  violet-11: "#4b3f8a"
  violet-4: "#cfc6f7"
  light-lilac: "#beb2e8"
  light-peach: "#fac8b2"
  light-aqua: "#aad6d6"
  danger-light: "#a1283b"
  danger-dark: "#ff9eac"
  warn-light: "#7d5200"
  warn-dark: "#f2c46e"
typography:
  display:
    fontFamily: "Geist, system-ui, sans-serif"
    fontSize: "clamp(1.875rem, 1.4rem + 2vw, 2.5rem)"
    fontWeight: 400
    lineHeight: 1.08
    letterSpacing: "-0.035em"
  wordmark:
    fontFamily: "Geist, system-ui, sans-serif"
    fontSize: "1.75rem"
    fontWeight: 500
    letterSpacing: "-0.045em"
  lead:
    fontFamily: "Geist, system-ui, sans-serif"
    fontSize: "1.0625rem"
    fontWeight: 400
    lineHeight: 1.55
  body:
    fontFamily: "Geist, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "Geist, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 500
  hint:
    fontFamily: "Geist, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 400
  data:
    fontFamily: "Geist Mono, ui-monospace, monospace"
    fontSize: "0.9375rem"
    fontWeight: 400
    lineHeight: 1.6
rounded:
  pane: "22px"
  well: "13px"
  pill: "999px"
spacing:
  "1": "0.25rem"
  "2": "0.5rem"
  "3": "0.75rem"
  "4": "1rem"
  "5": "1.5rem"
  "6": "2rem"
  "7": "3rem"
components:
  pane:
    backgroundColor: "linear-gradient(160deg, rgb(255 255 255 / 68%), rgb(255 255 255 / 42%))"
    rounded: "{rounded.pane}"
    padding: "clamp(1.375rem, 5vw, 2.5rem)"
  well:
    backgroundColor: "rgb(250 249 252 / 66%)"
    rounded: "{rounded.well}"
    padding: "0.8rem 0.95rem"
  button-primary:
    backgroundColor: "{colors.plum-12}"
    textColor: "#f6f4fb"
    rounded: "{rounded.pill}"
    height: "3rem"
    padding: "0 1.25rem"
  button-primary-dark:
    backgroundColor: "{colors.moonlight}"
    textColor: "#17142a"
  button-secondary:
    backgroundColor: "rgb(250 249 252 / 66%)"
    textColor: "{colors.ink-12}"
    rounded: "{rounded.pill}"
    height: "3rem"
  button-small:
    rounded: "{rounded.pill}"
    height: "2.5rem"
    padding: "0 0.9rem"
  segment-selected:
    backgroundColor: "#ffffff"
    textColor: "{colors.ink-12}"
    rounded: "{rounded.pill}"
    height: "2.625rem"
---

# Design System: Gone

## Overview

**Creative North Star: "Moonstone"**

Pearl daylight, or dusk, seen through frosted glass. Gone should feel calm, private, and quietly expensive: the opposite of a security dashboard. Trust comes from clarity and restraint. Nothing is hidden except the secret, and the secret is the only thing behind frost.

Each view is **one sheet of glass** floating over a soft light field. Inputs are recessed wells cut into that sheet, never cards stacked on cards. The header and footer sit directly on the backdrop.

The signature is the **frosted reveal**. On the receive page the secret sits behind frosted glass (a decorative, aria-hidden vault). When it opens, the message panel defrosts from blur to sharp. When it is gone, the eroding **G** of the brand mark stands alone on the pane.

Mode is **Operate** for send, receive and manage, and **Read** for About.

**Key characteristics:**
- Frosted panes (36px backdrop blur, 130% saturation) with a bright top edge and a deep, soft shadow.
- A backdrop of three low-saturation light fields (lilac, peach, aqua) plus fine film grain. It drifts slowly on wide screens only.
- One dark (light mode) or moonlight (dark mode) primary button per view. Everything else is glass.
- One accent, moonstone violet, rationed to the accent word in headings, focus, progress, and the current lifecycle step.
- Geist for the interface, Geist Mono for anything a person might retype: messages, links, passphrases.

## Colors

Semantic tokens (`--color-*`, `--pane-*`, `--well*`, `--primary*`, `--danger*`, `--warn*` in `web/css/tokens.css`) resolve per theme through `light-dark()`. Components use only the semantic names.

### Backdrop
- **Pearl** `#eeecf1` (light) and **Dusk** `#121019` (dark) are the base.
- Light fields: lilac, peach and aqua radial gradients at 46 to 68% (light) or 26 to 46% (dark) alpha.

### Text
- **Ink** `#1c1a26` / **Moon** `#efedf6` for headings and body.
- **Ink 2** `#4a4658` / `#c3bed4` for leads and secondary text.
- **Ink 3** `#625e73` / `#a7a2ba` for hints and labels. Both pass 4.5:1 on the pane.

### Accent
- **Violet** `#4b3f8a` (light) / `#cfc6f7` (dark). The accent word in each `h1` (`.accent`), focus rings, progress fills, the current lifecycle step.

### Actions and status
- **Primary:** plum `#211e30` gradient in light mode, moonlight `#dcd6f4` in dark.
- **Danger:** muted rose `#a1283b` / `#ff9eac`. Only for deleting.
- **Warn:** amber `#7d5200` / `#f2c46e`, on a 10% tint. For the insecure-connection banner and all `.alert` callouts.

## Typography

**Geist** and **Geist Mono** are self-hosted as Latin variable woff2 files (weights 100 to 900, OFL) because the CSP allows fonts only from this origin.

- **Display (`h1`):** Geist 400, `clamp(1.875rem → 2.5rem)`, tracking -0.035em, balanced wrapping. The last word is set in the accent colour (`<span class="accent">`), never italic. Geist has no italic, and a synthesised one looks cheap.
- **Wordmark:** the eroding G (`web/img/gone-g-32.svg`, applied as a CSS mask so it follows the text colour) stands in for the capital. It is cap-height tall, sits on the baseline with the same 0.016em overshoot as the "o", and is followed by "one" in Geist 500 at -0.045em.
- **Data:** Geist Mono for the message textarea, the revealed secret, links and passphrases. It separates I, l, 1, O and 0, which matters when the content is a password.

## Layout

- The header and footer use `.wrap` (72rem max). Content is one centred `.view` (36.25rem; About is 47.5rem) holding one `.pane`.
- Inside a pane: eyebrow chip, `h1`, lead, then a `.form` grid with 1.25rem gaps, then the lifecycle `.rail`.
- Mobile (≤40rem): the lifecycle rail becomes 2×2, passphrase buttons drop below the input, and action buttons stretch.

## Elevation & Depth

Depth comes from glass only:
- **Pane:** blur, a 1px light edge, an inset top highlight, and two soft shadows.
- **Wells:** recessed, with an inset shadow.
- **Lens:** the chosen segment is raised, with a top highlight and a small shadow.

`prefers-reduced-transparency` swaps every pane for a solid `--pane-solid`. Forced-colors mode draws pane borders and the mark in system colours.

## Shapes

- **Panes:** 22px corners (20px on phones).
- **Wells and callouts:** 13px.
- **Buttons, segmented tracks, link boxes and chips:** fully round (pills).

## Components

- **Buttons:**
  - `.btn-primary` is the one strong action per view.
  - `.btn-secondary` is glass.
  - `.btn-quiet` is text on hover tint.
  - `.btn-danger-ghost` starts a delete; `.btn-danger` confirms it.
  - `.btn-block` is the full-width main action. `.btn-small` is 2.5rem tall.
- **Wells:** `textarea`, `.input`, `.drop`, `.files li`, `.secret-panel` and `.linkbox` share the recessed well treatment. Focus turns the edge violet and adds a 4px soft ring.
- **Segmented control (expiry):** a pill track. The checked radio's label becomes a raised lens.
- **Link box:** the read-only link in a recessed pill, with its copy button inside the right end.
- **Lifecycle rail:** four steps (Sealed, Waiting, Opened once, Gone) in a row along the foot of the pane.
  - Each step has a 2px top rule: hairline when upcoming, faint violet when done (`.is-done`), solid violet when current (`.is-now`).
- **Callouts:** `.alert` is amber, for warnings and errors; `.banner` is an alert floating above the pane. `.callout` is violet, for notes like "Passphrase protected".
- **Vault:** decorative frosted placeholder lines with a lock, shown before opening.
- **Covered message:** after Open, the message starts behind frost in case the recipient is sharing their screen. On the frost: blurred placeholder dots, a size pill ("4 lines", or characters for one line), one line of explanation, and **Show message** under it, so the reason and the action sit together. **Copy message** stays top-right in the panel header and works while covered. Once shown, the frost goes and a quiet **Hide** joins Copy in the header. The real text is not in the page while covered, and only the button reveals it, never a click on the frost. "Always show secrets on this device" is a per-browser localStorage setting at the foot of the panel.
- **Gone hero:** the 64px drawing of the mark (`gone-g-64.svg`, masked), `h1`, lead, a `.next` well with what to do, one button.
- **Manage:**
  - Status details: a `.pill-status`, a `.facts` key/value list, and Check again.
  - Deleting: a `.zone` with Delete now, which expands into an inline rose `.confirm`.
- **Navigation:** About link and an icon-only theme toggle (moon in light mode, sun in dark) with an sr-only "Dark mode" label and `aria-pressed`.

## Do's and Don'ts

### Do
- Put each view on exactly one pane. Use wells for anything inside it.
- Keep the accent to one word per heading plus focus and progress.
- Use Geist Mono for anything a person might copy or retype.
- Keep the backdrop soft and low-saturation. It is light behind glass, not decoration.

### Don't
- Don't nest cards or add extra panes for grouping. Use hairlines, wells, or spacing.
- Don't use neon, purple-to-pink gradients, or glows on text.
- Don't italicise Geist.
- Don't add inline styles or scripts; the CSP forbids them.
- Don't scale the master mark down to header size. Use the 32px drawing (header) or 64px drawing (hero).
