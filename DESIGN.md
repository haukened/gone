---
name: Gone
description: One-time secret sharing, set like public-service signage.
colors:
  navy-12: "#14213d"
  navy-11: "#3d4a63"
  navy-10: "#5b6780"
  navy-7: "#b8c3d1"
  navy-6: "#d7dee7"
  navy-2: "#f6f8fa"
  white: "#ffffff"
  marigold-9: "#f2a900"
  marigold-11: "#b07800"
  amber-2: "#fff8e6"
  amber-7: "#e9c46a"
  night-2: "#0b1324"
  night-3: "#111b30"
  night-6: "#22304a"
  night-7: "#34445f"
  mist-12: "#e9eef6"
  mist-11: "#b9c4d6"
  mist-10: "#8d99af"
  umber-2: "#2a2210"
  umber-7: "#6b5214"
typography:
  display:
    fontFamily: "Atkinson Hyperlegible Next, system-ui, sans-serif"
    fontSize: "2.488rem"
    fontWeight: 700
    lineHeight: 1.1
    letterSpacing: "-0.02em"
  numeral:
    fontFamily: "Atkinson Hyperlegible Next, system-ui, sans-serif"
    fontSize: "4.3rem"
    fontWeight: 700
    lineHeight: 0.85
    letterSpacing: "-0.04em"
    fontFeature: "tnum"
  headline:
    fontFamily: "Atkinson Hyperlegible Next, system-ui, sans-serif"
    fontSize: "1.44rem"
    fontWeight: 700
  title:
    fontFamily: "Atkinson Hyperlegible Next, system-ui, sans-serif"
    fontSize: "1.2rem"
    fontWeight: 700
  body:
    fontFamily: "Atkinson Hyperlegible Next, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "Atkinson Hyperlegible Next, system-ui, sans-serif"
    fontSize: "0.833rem"
    fontWeight: 700
  data:
    fontFamily: "ui-monospace, SF Mono, Cascadia Mono, Menlo, Consolas, monospace"
    fontSize: "0.9rem"
    fontWeight: 400
rounded:
  sm: "2px"
  md: "4px"
spacing:
  "1": "0.25rem"
  "2": "0.5rem"
  "3": "0.75rem"
  "4": "1rem"
  "5": "1.5rem"
  "6": "2rem"
  "7": "3rem"
  "8": "4rem"
components:
  button-primary:
    backgroundColor: "{colors.navy-12}"
    textColor: "{colors.white}"
    rounded: "{rounded.md}"
    padding: "0 1.35rem"
    height: "3rem"
  button-primary-hover:
    backgroundColor: "{colors.navy-11}"
  button-secondary:
    backgroundColor: "{colors.white}"
    textColor: "{colors.navy-12}"
    rounded: "{rounded.md}"
    padding: "0 1.35rem"
    height: "3rem"
  button-small:
    rounded: "{rounded.md}"
    padding: "0 0.9rem"
    height: "2.75rem"
    typography: "{typography.label}"
  input:
    backgroundColor: "{colors.white}"
    textColor: "{colors.navy-12}"
    rounded: "{rounded.md}"
    padding: "0.85rem 1rem"
  segment:
    textColor: "{colors.navy-11}"
    rounded: "{rounded.sm}"
    height: "2.75rem"
    padding: "0 0.75rem"
  segment-selected:
    backgroundColor: "{colors.navy-12}"
    textColor: "{colors.white}"
  band:
    backgroundColor: "{colors.navy-12}"
    textColor: "{colors.white}"
    height: "4rem"
  alert:
    backgroundColor: "{colors.amber-2}"
    textColor: "{colors.navy-12}"
    rounded: "{rounded.md}"
    padding: "1rem"
---

# Design System: Gone

## Overview

**Creative North Star: "The Public-Service Instrument"**

Gone is set like civic wayfinding, in the lineage of Otl Aicher's Munich '72 and Lufthansa systems: a navy band, one marigold mark, big numerals, hairlines, and a strict 8-column grid. Trust comes from the system rather than decoration. Nothing glows, floats, or sells. The interface is calm, precise, and literal about what happens to a secret.

The signature is the **open ring**. It is the brand mark and also the secret's lifecycle: sealed, waiting, opened once, gone. Every surface repeats that story with the same four pictograms.

Mode is **Operate** for send and receive, and **Read** for About.

**Key Characteristics:**
- Navy band header in both themes, with the marigold ring mark.
- Numbered steps (1 Write, 2 Set expiry, 3 Share) with large tabular numerals.
- Flat surfaces separated by 1px hairlines; no shadows.
- One accent (marigold), rationed to the mark, the current step, focus, and status icons.
- A single hyperlegible typeface; monospace only for data (the link).

## Colors

The palette is borrowed from Radix-style 12-step scales and aliased into semantic tokens (`--color-*` in `web/css/tokens.css`). Components consume semantic tokens only; each one resolves per theme through `light-dark()`.

### Primary
- **Signage Navy** (navy-12): the band, primary buttons, the selected segment, and body ink in light theme. In dark theme ink flips to Mist (mist-12), but the band stays navy.

### Secondary
- **Marigold** (marigold-9): the mark colour. Used for the ring logo, focus rings, the current-step underline, and the pressed theme toggle. Never used for text on light surfaces.
- **Deep Marigold** (marigold-11): the light-theme signal colour for status and alert icons. It reaches 3:1, so it is for icons and large UI only, never body text.

### Neutral
- **Paper / Night** (navy-2 / night-2): the page background.
- **Surface** (white / night-3): fields, panels, and secondary buttons.
- **Ink ramp** (navy-12, 11, 10 / mist-12, 11, 10): primary, secondary, and tertiary text. All three meet 4.5:1 on paper and surface in both themes.
- **Hairlines** (navy-6, 7 / night-6, 7): `line` for section rules, `line-strong` for control borders.
- **Warning** (amber-2 with amber-7 / umber-2 with umber-7): alert background and border.

**The One Mark Rule.** Marigold marks *where you are* or *what has focus*. If it appears in more than three places on one screen, one of them is wrong.

**The Band Holds Rule.** The band is navy in both themes. Theme switching changes the paper beneath it, never the identity above it.

## Typography

**Atkinson Hyperlegible Next** is self-hosted as one Latin variable woff2 (weights 200 to 800, OFL). It is used for everything except the share link. It was chosen because it separates I, l, 1, O, and 0, which matters when the content is a password.

The root is 17px (106.25%) on a 1.2 modular scale, `--step--1` to `--step-5`.

### Hierarchy
- **Display** (step-5, 700, 1.1, -0.02em): one h1 per view, balanced wrapping. 2rem on narrow screens.
- **Numeral** (4.3rem, 700, tabular figures): step numbers only. 2.6rem on narrow screens, inline with the step name.
- **Headline** (step-2, 700): About section headings.
- **Title** (step-1, 700): step names, view subheadings, and the revealed secret text.
- **Body** (1rem, 1.55): running text, at most 40ch for leads and 66ch for prose.
- **Label** (step--1, 700): field labels, nav, status lines, and small buttons.
- **Data** (system mono, 0.9rem): the share link only.

**The Numerals Lead Rule.** On task screens the step numeral is the largest thing after the h1. Never put decorative type above it.

## Layout

- `.wrap` caps content at 72rem with fluid side padding (`clamp(1rem, 4vw, 2.5rem)`).
- An **8-column grid** with 1.5rem gutters. Task content spans columns 1 to 6; the "What happens next" rail spans 7 to 8.
- Each **step** is a 6-column subgrid: the numeral and name take 2 columns, the body takes 4. A hairline separates steps.
- About uses columns 1 to 2 for a sticky table of contents and columns 3 to 7 for prose.
- There is one breakpoint, at **52rem**. Below it everything is one column, step heads go inline, the lifecycle becomes a 2×2 grid, action buttons go full width, and the table of contents hides.
- Spacing follows the 0.25 to 4rem scale (`--space-1` to `--space-8`). Section rhythm comes from steps 6 to 8; spacing inside controls comes from steps 2 to 4.

**The Grid Is The Brand Rule.** Align to the 8 columns. A one-off offset breaks the signage feel faster than a wrong colour.

## Elevation & Depth

The system is flat. There are no box shadows for depth: layering is tonal (paper, then surface) and edges are hairlines. The only `box-shadow` in the system is the focus ring.

**The No-Lift Rule.** Hover changes border or fill colour; it never lifts, scales, or casts a shadow.

## Shapes

- **Corners:** 4px (`--radius`) on buttons, fields, panels, and alerts; 2px on segments inside their track. Nothing is pill-shaped.
- **Borders:** 1px solid. Dashed borders are reserved for the file drop zone and the "gone" ring.
- **Pictograms:** one inline SVG sprite, stroke-only, square caps, 2px stroke (3 to 3.2px for the brand mark and the current lifecycle step). Icons are always `aria-hidden` and paired with visible text.
- **Lifecycle glyphs:** ring (300° arc, waiting), sealed (circle with a dot), opened (120° arc), gone (dashed circle).

## Components

### Buttons
- **Primary:** an ink fill (navy, or mist in dark) with on-ink text, 3rem tall, weight 700. Hover steps down to ink-2. One per view.
- **Secondary:** a surface fill with a `line-strong` border; on hover the border becomes ink.
- **Small:** 2.75rem tall, label size. Used for Copy, Download, and Remove inside panels and rows.
- **Disabled and busy:** `aria-disabled="true"` at 55% opacity. The button stays focusable and its label says what is happening ("Opening…").
- Icon buttons always have a text label; accessible names include the file name where relevant.

### Inputs / Fields
- Flat surface, 1px `line-strong` border, 4px corners, 0.85rem by 1rem padding. On hover the border darkens to ink-3.
- The textarea auto-resizes from 11rem. Placeholders use ink-3 and never stand in for labels.
- Each field has a bold label above it and an optional hint below in ink-3. Errors use `aria-invalid` plus `aria-describedby`, and are never shown by colour alone.

### Segmented control (expiry)
Native radio inputs in a fieldset with a legend. The track is a grid that auto-fits columns of at least 6.5rem. The selected segment fills with ink; the focused segment gets an inset marigold ring.

### Steps
A large tabular numeral and a step name in the left two columns, with the content in the right four. A hairline sits above each step. This is the backbone of the send screen.

### Lifecycle (signature)
An ordered list of the four lifecycle glyphs.
- **Vertical** in the send rail, with short explanations.
- **Horizontal** on the result and revealed views, with connecting hairlines. Done segments thicken to a 2px ink line.
- The current step carries `aria-current="step"`, a marigold glyph, and a 3px marigold underline. Future steps drop to ink-3.

### Alerts
A tinted warning background with a hairline warning border, 4px corners, a signal-coloured icon, and a bold lead line. Never a thick left stripe. Errors use `role="alert"`.

### Link box and secret panel
- **Link box:** a read-only monospace field with ellipsis overflow next to Copy link. It stacks on narrow screens.
- **Secret panel:** a surface card with a header row (label and Copy message), and the copy status announced inside the header. The message is title size, `pre-wrap`, scrolls at 60vh, and breaks anywhere.

### Navigation
- The band holds the brand on the left and the nav on the right. Nav items are 2.75rem targets at label size.
- The theme toggle is outlined at 28% white. When pressed (`aria-pressed="true"`) it gets a marigold border, a 10% white fill, and a marigold icon.
- A skip link appears on focus.

### Focus
A double ring, 2px ink and then 5px marigold (inside the band, 2px white and then 5px marigold). It is never removed. In forced-colors mode it becomes a 3px `CanvasText` outline.

## Do's and Don'ts

### Do:
- Do use semantic `--color-*` tokens in components; primitives belong only in `tokens.css`.
- Do pair every icon with visible text and keep icons `aria-hidden`.
- Do keep targets at 44px (2.75rem) or larger and keep the double focus ring visible.
- Do move focus to the new view's heading after swapping views, and announce status in a live region that stays rendered.
- Do respect `prefers-reduced-motion`, `prefers-color-scheme`, and `forced-colors`.
- Do keep all CSS and JS external; the CSP forbids inline styles and scripts.

### Don't:
- Don't add shadows, gradients, glass effects, or pill shapes.
- Don't use marigold for body text, or deep marigold below large or UI sizes.
- Don't add a second accent colour or recolour the band per theme.
- Don't use placeholders as labels or build icon-only controls.
- Don't load fonts, icons, or scripts from a CDN.
- Don't use `innerHTML` to render secret content; use `textContent` only.
