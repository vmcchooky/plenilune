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

## Slice 2 — primitives + motion language

Component styling now consolidates onto shared primitives. Markup
changes were className-only; no behavior, copy, structure, or test
changed. All 109 web tests pass unmodified; no test depended on a
renamed class (tests reference `.partial-result`, `.chat-panel`,
`.change-class-list`, `.quick-obligations__listing`, `#sample-select` —
none renamed).

### Primitive inventory

- **Button — `.button`** (app.css): the existing primitive, kept as the
  canonical name (used across every step and panel). Variants `--primary`, `--secondary`,
  `--quiet`, `--small`; new `--danger` modifier (bad text, bad-soft
  hover) for destructive actions, combined with `--quiet --small` on
  the condition/scope-region/rule remove buttons. Its transition moved
  from a literal `120ms ease` to the motion tokens.
- **Field — `.field`** (app.css): label/hint/error wrapper as before;
  the control treatment remains the global `input, select, textarea`
  rule (now with a token border-color transition). Wrapping bare
  controls in `.field` would need DOM changes, so it was not forced.
- **Card — `.card`** (app.css): `surface` background, 1px `--line`
  border, `--radius-md`. Adopted by `.verdict-plate`,
  `.decision-block`, `.source-pane`, `.source-spread`, `.change-class`;
  their own rules now keep only extras (accent left edge, padding,
  layout). `.source-pane` gained `overflow: hidden` so its head strip
  follows the card radius.
- **Badge — `.badge`** (app.css): pill label (bordered, `--text-xs`,
  650 weight) with semantic variants `--info / --warn / --good / --bad`;
  the `button.badge` form adds pointer, hover, and motion. Adopted by
  the verdict code chip (was `.verdict-plate__code`), the report
  verdict code (was a `.report-verdict code` element rule), the
  rule-builder draft tag (was `.rule-builder__tag--draft`), and the
  chat suggestion chips (was the undefined `.chip` class — previously
  rendered unstyled). `.status-mark` and `.provenance` remain the
  icon-bearing label components.
- **Banner — `.banner`** (app.css): replaces the duplicated
  `.notice` / `.notice-line` pair with one definition and tone variants
  `--neutral / --info / --warn` (suffixes match the old `--warn`
  naming). All 38 markup usages migrated. Specialized banners keep
  their own classes on purpose: `.partial-result` (stacked fact lists;
  also test-referenced), `.accepted-stamp`, `.error-panel`.
- **Segmented control** (app.css): the language switch, mode switch,
  and evidence toolbar (`.segmented`) were three near-identical
  implementations; their container, item, divider, hover, and active
  declarations are now one grouped definition. Each control keeps only
  sizing/typography extras (`.segmented` extras stay in components.css;
  the toolbar also gained the shared radius/overflow clipping).
- **Row button pattern** (CSS-only): `.rail-step`,
  `.condition-result`, `.witness-index__item`, `.candidate-button`,
  `.download-item`, `.scope-option` share hover/selected behavior but
  have incompatible layouts, so they were not forced onto one class;
  they now share the token transition for background/border instead.

### Motion rules applied

- **One language:** every transition in app.css/components.css uses
  `--dur-1` (160ms) with `--ease`; entrances use `--dur-2` (240ms)
  with `--ease`. Census: before — 3 transition declarations, all
  literal (`120ms ease`, `140ms ease`, `0.15s ease`); after — 13
  transition declarations, all token-based, zero literal durations.
- **Transitions added/replaced:** button (all variants), badge
  buttons, segmented items, rail steps, inputs/selects/textareas
  (border-color), interactive rows (above), the quick-dropzone
  drag-over state, the evidence toggle + its chevron, and the
  technical-details chevron (140ms → tokens).
- **Entrances (keyframes in app.css, all `--dur-2` + `--ease`):**
  `rise-in` (opacity + 4px rise) on `.step` (mode/stage changes) and
  `.verdict-plate`; `fold-in` (opacity + −4px) on the quick evidence
  drawer panel; `fade-in` on `.chat-panel`, `.chat-exchange`, and
  `.replay-tools` (offline review). Entrances are mount animations —
  the surfaces are conditionally rendered, so there is no exit motion
  without JS changes. The spinner keeps its linear `spin` keyframes
  (rotation is not part of the easing language).
- **Reduced motion:** the global guard in tokens.css sets
  `transition-duration` and `animation-duration` to `0.01ms !important`
  (plus iteration count 1) on `*, *::before, *::after`, which covers
  every transition and animation added here; app.css carries the same
  guard. No addition escapes it (no inline styles, no WAAPI).

## Slice 3 — quick-screen hierarchy (de-clutter)

The quick verdict screen (and the shared verdict surfaces in
ResultStep) now follow a calm three-tier hierarchy. CSS-only: no
markup, behavior, copy, or test changed; all 109 web tests pass
unmodified.

- **One measure:** `.quick-screen` narrows the shared `.step` column
  from 1000px to 880px. Section separation is the `.step` gap alone
  (`--space-8`, a multiple of 8); the `border-top` + `padding-top`
  rule lines the quick sections used to stack on top of that gap are
  removed.
- **Tier 1 — the verdict.** `.verdict-plate` is the single dominant
  element: title at `--text-xl` (the largest type on the screen — the
  page `h1` steps down to `--text-lg` on this screen), `--space-6`
  padding, `--space-4` internal gap, and the `--shadow-1` lift that
  flat tier-2 cards do not get. Its semantic left edge and colors are
  unchanged.
- **Tier 2 — objects + obligations.** Groups live on flat card-grade
  surfaces: the obligations section and the config overview carry the
  `.card` treatment (white `--surface`, 1px `--line`, `--radius-md`,
  `--space-6` padding, no shadow). Section titles sit at `--text-md`,
  body at `--text-body`, metadata muted. The repair section stays
  transparent — its decision blocks are already cards, and wrapping
  them would re-nest. Inside the obligations card, separation is
  spacing and surface tiers, not borders: the obligations results
  read as ONE grouped list (a `--surface-2` tinted panel holds the
  rows; the list's top rule and the rows' bottom rules are gone),
  the accepted stamp drops its left edge (its tint already
  separates), and the intent block and scope disclosure lose their
  rule lines. Object rows keep their hairlines — on the white card
  the border is the only separator, so it stays; the list's own top
  rule and the last row's bottom rule go.
- **Tier 3 — evidence + chat.** The quietest tier: section rule lines
  gone, titles one step below tier 2 (chat title `--text-body`,
  evidence toggle `--text-body` in `--muted`, ink on hover), help
  text at `--text-sm`. The open evidence drawer becomes a
  `--surface-2` inset panel; chat exchanges become borderless
  `--surface-2` blocks instead of left-railed text.
- **Chrome stays chrome:** the mode switch keeps the shared segmented
  primitive but with a softer `--line` border and a `--surface-3`
  tinted active state instead of the solid ink block, so it never
  rivals the verdict.
- **Redundant-border rule (the general principle):** where a
  background tint and a border were both doing the separation, the
  border goes (partial notice, accepted stamp, chat exchanges,
  grouped obligation rows). Where a border is the only separator
  (object rows on white, control chrome), it stays.

Bordered-container count on the quick screen, canonical results
state (verdict + partial notice, 3 objects, 2 accepted obligations,
repair shown, evidence closed, chat empty; layout containers only,
excluding button/input/badge control borders): **19 → 7**. The
survivors are the mode switch, the source card and its column
divider, the verdict plate, the obligations card, and two object-row
hairlines.
