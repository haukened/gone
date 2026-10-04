# Gone brand mark

A heavy G whose right side erodes into particles. The solid letter is drawn on two exact circles; the erosion and particles come from the original hand-made artwork, traced to smooth curves.

| File | Use |
|---|---|
| `gone-mark.svg` | Master mark, any size above 64px. Ink (`#10161c`). |
| `gone-mark-brass.svg` | Master mark on dark grounds: bone letter (`#e7e4dc`), brass particles (`#b8925a`). |
| `gone-wordmark.svg` | The mark with "one" in matching heavy lowercase. |
| `gone-mark-64.svg` | Drawn for 64px: only particles big enough to see, firmer eroded edge. |
| `gone-mark-32.svg` | Drawn for 32px: the seven largest particles. |
| `gone-mark-16.svg` | 16px pixel master, drawn on the pixel grid. Render it at exactly 16px. |

Each mark file has two paths: the letter first, then the particles. Recolour or animate the particles on their own by targeting the second path.

**Sizes.** Use the master at 65px and above, and the sized drawings below that; do not scale the master down to icon sizes, because the fine particles turn to grey noise.

**Clear space.** Keep at least a quarter of the mark's height clear on every side.

**Site icons** (`web/img/`) are built from these files: `favicon.svg` (the 32px drawing on a vault-ink tile), `favicon.ico` (16px pixel master and 32px), and `apple-touch-icon.png` (180px).
