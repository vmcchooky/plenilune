# Plenilune review desk

The 2 November 2026 redesign request supersedes Pastel Flow / Neo 1+.
Design ownership and rationale: [art direction](../docs/design/art-direction.md),
[decisions](../docs/design/design-decisions.md), [state matrix](../docs/design/states-and-edge-cases.md).

`src/styles/tokens.css` is canonical. `design-system/tokens.css` forwards to it for
standalone specimens. `app.css` owns global/shared controls, `components.css` owns
existing feature styling, `review-desk.css` owns the review composition/disclosures.
Light is the owner-directed primary mode, independent of OS appearance. The warm
cream/beige neutral scale separates canvas, cards, insets and selected rows through
tonal steps; muted olive is the single accent. Semantic warning/error hues appear
only in labeled feedback. See the current [style recipe](../docs/design/style-recipe.md).
Sources, API data, policy acceptance and receipt binding still belong to the existing
review hook and backend. Collapsing a section never accepts a policy or human decision.

The owner-requested craft revision removes colored panel rails and content entrance
animations, aligns spacing to 8/16/24/32px, uses 6px control corners and 8px panels.
The quick screen prioritizes available results; blank state prioritizes sources.
Native details/summary provide keyboard-operated optional watches and conditions.
Existing deep review uses the same tokens and retains the five stages.
Owner visual sign-off remains pending; see the golden-screen candidate and QA report.

Quick-path update: `QuickOutcome.tsx` presents the example packet/action change and
proposed deciding rule. `QuickScreen` keeps optional tools collapsed after analysis;
blank/unsupported/multi-object cases expose the relevant inputs/selection. Critical
incomplete/coverage/truncation notices remain visible on the outcome. See updated
UX flows for the difference between the quick decision view and deep review.

Composition alternatives are available at `?design=a`, `?design=b`, `?design=c`.
The preview is lazy-loaded from src/design/compositions; normal App entry is retained.
See [studies and checks](../docs/design/composition-studies.md). The owner selected
A (change record) as the quick-entry basis on 02/11; the quick screen now renders it
through the shared `ChangeRecord` component rather than copied CSS. B/C remain
layout studies. Switching layouts reuses the same review state.

Night additions (03/11, P3): warning banners share the `--warn-soft` family;
the quick record is capped at 720px on the page's left axis; trust indicators
stay visible on small screens; partial results render outside the collapsed
details; touch controls keep a 44px floor on coarse pointers (quick mode fully; deep mode partially — small buttons and summaries), with 24px checkbox controls recorded as a deliberate exception.
