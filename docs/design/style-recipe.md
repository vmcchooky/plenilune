# Cream review desk · owner-directed palette

Owner direction, 02/11/2026: primary mode is light; beige/cream background; gentle
accents; minimize hue count and distinguish components through tonal variation.

Material: warm neutral review desk, using Editorial Warm's cream surface recipe
with system sans typography retained for dense configuration comparison. No serif
marketing treatment is introduced. This supersedes the prior cool gray/teal palette.

| Role | Token / value |
|---|---|
| Canvas | --bg #f5f1e8 |
| Raised content | --surface #fffdf8 |
| Inset / secondary | --surface-2 #eee8dc |
| Selected control | --surface-3 #e4dccb |
| Code editor | --code-bg #f8f4ec |
| Text | --ink #302e28, --ink-2 #504c43, --muted #625c50 |
| Quiet line | --line #dfd7c8 |
| Single accent family | olive --accent #526247, strong #414e38, soft #e7ebdf |

Success shares the olive family. Ochre and clay are limited to labeled warning/error
feedback. Panel borders remain warm neutral, with no semantic-colored rails.
Light is explicit and independent of OS preference; no automatic dark scheme.
Browser native controls and surrounding window theme metadata request light mode.

Type/geometry unchanged: 12/13/14/16/20/24/32px, 8/16/24/32px spacing, controls 6px,
panels 8px, pill status chips. No gradients, decorative motion or drop shadows.
Canonical tokens: web/src/styles/tokens.css. Owner direction is explicit; final
visual approval remains pending. Current local preview: out/review-desk/cream-preview.png.
