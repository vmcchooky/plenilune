# Plenilune web — design system (slice 1: token foundations)

Owner-locked direction: **Pastel Flow / Neo 1+**. This slice rewrote
`src/styles/tokens.css` as the single source of truth and purged style
literals in `app.css` / `components.css` onto tokens. No behavior, logic,
markup, or test changed — visuals only.

## The anchor — one gradient is the whole color backbone

```css
linear-gradient(135deg, #E8F5FD 0%, #F2ECF5 50%, #FCEBE1 100%)
```

- Surface tints are sampled from this one ramp: cool end `#E8F5FD`,
  25% stop `#EDF3F6`, mid `#F2ECF5`, 75% stop `#F7EEE8`, warm end `#FCEBE1`.
  Cards stay white (`--surface: #ffffff`).
- **Honesty note:** the 50% stop `#F2ECF5` is a *hand-tuned* midpoint,
  chosen by eye to keep a whisper of lavender in the middle of the wash.
  It is **not** the linear RGB interpolation of the two ends — that is
  `#F2F0EF`. The 25% / 75% stops (`#EDF3F6`, `#F7EEE8`) *are* true linear
  stops. Never document `#F2ECF5` as an interpolated value.
- The gradient is applied **only** as the app-level background wash
  (`body { background-image: var(--wash) }`, fixed attachment), at low
  visual weight behind white cards. It is never applied to a component
  and never varies by state. The dark scheme sets `--wash: none`.
- Accent (interactive / primary) is deepened from the cool end:
  `--accent: #2E6484`, `--accent-strong: #245269`, `--accent-soft: #E8F5FD`.
- Neutrals are slate derived by darkening the anchor's cool family
  (`--ink #1b2732`, `--ink-2`, `--muted`, `--faint`, `--line`,
  `--line-strong`); text values keep at least the contrast of the warm
  palette they replaced.
- Semantic verdict colors (`--good`, `--bad`, `--warn`, `--rust` and
  their `-soft` pairs, `--focus`, `--stamp`) are **unchanged in meaning
  and value** — legibility of verdicts is a contract, not a tint.

## Scales

- **Type — 6 steps only:** 12 / 13 / 14 / 16 / 20 / 28 px
  (`--text-xs / --text-sm / --text-body / --text-base / --text-lg /
  --text-xl`). Weights 400 / 600. Mono stack unchanged for hashes/code.
  Legacy names kept so existing references resolve: `--text-md` = 16,
  `--text-display` = 28.
- **Spacing — 4/8 grid only:** 4 / 8 / 12 / 16 / 24 / 32 / 48 px
  (`--space-1 / -2 / -3 / -4 / -6 / -8 / -12`). Legacy `--space-5` = 16,
  `--space-10` = 32, `--space-16` = 48 (nearest grid step, ties down).
- **Radius:** 8 / 12 / 16 px (`--radius-sm / -md / -lg`) + `--radius-pill`;
  `--radius-none` = 0. Note `--radius-sm`/`--radius-md` kept their names;
  their values moved from 2 / 4 px onto the new scale.
- **Elevation:** two soft shadows, `--shadow-1` and `--shadow-2`
  (`--shadow` remains as the level-2 alias).
- **Motion (tokens only in slice 1; component application is slice 2):**
  `--ease: cubic-bezier(0.2, 0, 0, 1)`, `--dur-1: 160ms`,
  `--dur-2: 240ms`. A global `@media (prefers-reduced-motion: reduce)`
  guard in `tokens.css` neutralizes transitions and animations.

## Mapping rules applied in the purge

All replacements in `app.css` / `components.css` followed these
deterministic rules (540 declarations rewritten):

- `font-size` → nearest type step; ties round **down**; `clamp()` mapped
  by its maximum value; `em` treated at the 16 px base. Result: 50
  distinct values → only `var(--text-*)` references remain.
- `padding` / `margin` / `gap` (and longhands) → each component mapped to
  the nearest grid step, ties down; `0` and `auto` kept; px values inside
  `clamp()` mapped the same way, fluid `vw` parts kept; a negative value
  became `calc(-1 * var(--space-*))`. Result: 39 distinct component
  values → the 7 grid tokens (+ `auto`).
- Hex colors → nearest token. The app/components sheets were already
  almost fully tokenized; the only literals were in print rules
  (`#fff` → `--surface`, `#000` → `--ink`).
- `border-radius` → radius tokens: `50%` → pill, `0` → none, any nonzero
  sub-scale value → `--radius-sm` (intent-preserving: a slightly rounded
  corner stays rounded rather than snapping to square).

## Deliberately left literal (and why)

- `#000` inside the two `mask-image` gradients (app.css) — a mask stop is
  a functional opaque marker, not a palette color; any opaque value is
  equivalent, and a token whose dark-scheme value is near-white would
  read as a bug.
- `margin: -1px` (app.css) — a hairline overlap offset; mapping it to the
  grid (0) would break the overlap it exists to create.
- Border widths (`1px` / `2px`, via `--border-thin` / `--border-thick`
  or literal in `border` shorthands) — hairlines are not spacing.
- Transition durations in the 3 existing `transition` declarations —
  motion *application* is slice 2 by plan; slice 1 only defines the
  tokens and the reduced-motion guard.
- Non-spacing geometry (widths, heights, `top/left` offsets, grid track
  sizes, `line-height`, `letter-spacing`) — outside this slice's scope.
