# Reference notes · overnight product pass

Owner direction remains primary: light cream canvas, muted olive accent, tonal
surfaces, concise change review, deterministic evidence. References inform small
complete patterns rather than replacing the product's design language.

## Requested local collections

- [awesome-design-md](https://github.com/voltagent/awesome-design-md), verified at
  local D:/Collections/repo-github/awesome-design-md, commit fcc7028. A catalog of
  third-party design analyses, not an official specification for those brands.
  Read the Notion analysis for warm neutrals/hairline hierarchy; do not adopt its
  marketing hero, purple CTA, many brand hues or pricing composition in this tool.
- [OpenDesign](https://github.com/nexu-io/open-design), verified at local
  D:/Collections/repo-github/open-design, commit 5a0c477. Read craft/animation-discipline.md.
  Useful direction: motion should explain a state transition, navigation or progress;
  retain static feedback and honor reduced motion. No package, font, asset or runtime
  dependency is imported from either repository into Plenilune.

Both clones existed when Codex joined the three-agent coordination. Origin URLs and
commit IDs were checked; they were not overwritten or cloned again over active work.

## Primary repository references inspected

- [Radix Primitives](https://github.com/radix-ui/primitives): use accessible interaction
  boundaries as a review reference for focus, disclosure and component responsibilities.
- [GitHub Primer React](https://github.com/primer/react): use shared product primitives
  and state consistency as an architectural reference.
- [Material Web](https://github.com/material-components/material-web): use explicit
  component states as a review reference, while retaining Plenilune's existing palette.

These references are not a claim that Plenilune matches those products' quality.
No numeric performance or browser-compatibility claims from collection material are
used as measured facts for this app. New motion must be verified against the actual
local UI, including reduced-motion behavior and cancellation.

## Shared-control pass

Typography already had negative tracking; A already had tabular endpoint numbers.
The pass introduces shared tracking/leading/focus/chip tokens, applies tabular digits
globally and aligns existing chip geometry without turning every runtime/provenance
label into a colored badge. Cream colors and plain secondary metadata are retained.
Actual build/browser gate belongs to the ZCode coordinator to avoid concurrent dist
writes. See (local coordination notes, not published) for acknowledgements and locks.
