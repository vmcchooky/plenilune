# Review desk QA · 02/11/2026

Implemented in the primary checkout at D:/Quorix/plenilune-publish-20261031.
User explicitly requested this visual redesign separately from the sequential
production plan. Existing uncommitted repair/report work was retained.

## Checks actually run

- `gofmt -l cmd internal`: no output. `go vet ./...`, `go test ./...`,
  `go test -race ./...`, `go build ./...`: PASS (many Go results cached).
- `npm test`: 17 files, 136 tests PASS. Existing jsdom scrollTo notice remains.
- `npm run build`: PASS. Existing approximately 523 kB bundle warning remains.
- `node tests/review-desk-browser.mjs`: 35 checks PASS using Brave/Playwright,
  local Go runtime at 127.0.0.1:8098, AI key removed from that server environment.
- VI and EN, light and dark, 320/390/768/1440 widths: quick result screenshots,
  no horizontal document overflow; open conditions/watches; deep source/result.
- Mobile Vietnamese: canonical policy preview and explicit acceptance → verify
  candidate with the real local engine → human review decision. Keyboard Enter
  expands the native conditions disclosure. Empty and malformed ACL input checked;
  malformed source remains unsupported and does not produce a verdict plate.
- Axe serious/critical findings: zero in all 35 sampled states. Contrast sampling
  waits for the existing 240ms stage transition; this does not alter app budgets.
- No external requests observed in the VI/EN light/dark review journeys.
- Author visually inspected full quick desktop light, desktop dark, mobile light
  captures. Other dimensions were checked automatically; no independent critic.

## Craft audit and limits

The skill UI lint reports 15 errors and 15 warnings. A separate HEAD source snapshot
produces the same counts by rule: 13 large existing components, 1 outline-none,
14 important declarations (including reduced-motion/print), 2 mask-color literals.
No new rule count was introduced. This is a retained debt baseline, not clean lint.
New review-desk CSS is 57 lines and uses runtime tokens.

Subtraction: remove the decorative wash/shadows, collapse optional watch/policy forms,
make the verdict/overview the first substantial content after analysis. This cuts the
sample quick page from about 4000px to about 1700px at 1440px width. The material board
includes an input-first composition considered before choosing results-first.
Author audit found no decorative KPI cards, fake technical stamps, gradient text,
stock assets or color-only verdicts. Generic browser-native controls remain deliberate.

Not claimed: calibrated rubric >=8.8, independent screenshot red-team, complete
keyboard-only journey, screen-reader qualification, device or live AI qualification,
or owner visual approval. Full phase-7 skill completion is pending those criteria.
Golden screen remains a review candidate, not a frozen owner-approved standard.

## Local artifacts

`out/review-desk/results.json`, screenshots, lint before/after JSON and `preview.png`.
These are local QA evidence, not publication inputs. `docs/design/moodboard.html`
provides the reusable token specimen and composition studies. No contract changed.

## Craft refinement after owner feedback · 02/11/2026

Owner feedback: effects, colored borders and uneven detail did not feel coherent.
Changed the shared primitives at their owners instead of adding another visual skin:
neutral gray line/surface scales in both schemes; neutral panel edges throughout
result, condition, witness, candidate and evidence views; semantic filled chips for
verdicts in both quick/deep screens; shared control corners and button hierarchy.
Removed content entrance animations; kept busy indication and chevron state changes.
Aligned sections to 32px, panel insets to 24/16px, internal gaps to 8/16px, and
corrected the extra 16px mobile mode-control inset discovered during visual review.
Verdict metadata now uses explicit grid positions rather than incidental line wraps.

Verification rerun: production build PASS; frontend 136/136 PASS; all Go gates PASS
(cached tests where applicable). Browser journey rerun twice with 35/35 checks PASS,
including VI/EN, four widths, both schemes and real local repair/human review.
After the final mobile inset correction, four additional light/dark 320/390 checks
assert equal navigation/content gutters, no document overflow, and zero serious or
critical axe findings. Lint remains at the same 30 inherited findings by rule.
The existing bundle size warning remains. No live model or device access occurred.

Visual inspection: refined desktop quick/deep light and mobile dark screenshots.
Local captures: out/review-desk/refined-light-preview.png,
refined-dark-preview.png, refined-deep.png, refined-light-320/390.png,
refined-dark-320/390.png. This revision is still a candidate for owner taste review;
no calibrated numeric score or independent reviewer approval is claimed.

## Cream palette requested by owner · 02/11/2026

Current UI is explicitly light, under both light and dark OS preferences. Uses
cream canvas #f5f1e8, ivory cards #fffdf8, beige insets/selection, warm brown-gray
text/lines, and one muted olive accent family. Success shares olive; ochre/clay
remain only for labeled warning/error feedback. Browser metadata/favicon match.
No new behavior, spacing or proof contract is introduced by this palette change.

Production build PASS (existing bundle warning retained). Browser suite 35/35 PASS
with explicit assertions for cream body background and light-only color scheme,
VI/EN, 320/390/768/1440 widths and both OS preferences. Axe serious/critical findings
zero in sampled states. An initial contrast finding on the active rail description
was corrected by darkening the muted text token to #625c50; assertions unchanged.
The suite still exercises local engine repair and human review, no model/device.
Dark-labeled capture names in this run refer to OS preference, not a dark UI.
Author inspected cream desktop quick and deep screenshots. Current preview is
out/review-desk/cream-preview.png; full-page mobile captures are in the same folder.
Earlier gray/dark theme screenshots and palette comments in this report are history.

## Concise quick outcome · 02/11/2026

Owner identified excessive reading for a small SSH change. Quick entry now shows a
single example packet, its permit/deny transition and the actual deciding rule,
with explicit model/intent limits. Other review tools are grouped and collapsed.
Incomplete/truncated results, unmodeled coverage and additional change groups have
visible notices. Full changes remain accessible; editing source/conditions reveals
the right disclosure and focuses it without changing review acceptance.

Build PASS; frontend 138/138 PASS, including new disclosure-action and visible
INCONCLUSIVE tests. Existing MGMT assertion now targets outcome identity rather
than the hidden overview. No assertion was dropped. Browser 35/35 PASS, including
real-engine repair/human review, VI/EN and four widths. The suite checks that default
editors/chat/policy are closed, the sample deciding deny rule is shown and initial
main content stays below 200 visible words. Axe serious/critical findings zero.

Measured synthetic sample at 1440x900: 93 visible words in main, no page scroll.
Author inspected desktop/mobile captures: out/review-desk/quick-concise.png and
quick-concise-mobile.png. This is a sample-specific usability improvement, not a
user study. Existing Go code is unchanged; previous Go gate results still apply.
The existing build-size warning and inherited lint debt remain.

## Night pass P3 (+P2.1 audit fixes) · 03/11/2026

Implemented by ZCode under the night coordination protocol (Codex quota lapse):
partial results render outside the collapsed details when present (F1); skip
link focus lands on `main` via tabIndex=-1 (F13); the quick record sits on the
page left axis at 720px instead of a centered 519px card (F14); warning banners
use the shared `--warn-soft` family (F2); runtime/AI trust indicators stay
visible on small screens (F6); mode/skip controls keep a 44px floor on coarse
pointers (F4); the mode bar is a named region (F12). Verification: npm test
19 files / 141 PASS; production build PASS; browser suite 35/35 PASS; the
independent 10-point P3 verifier (F14@1440/1024, F13, F1, F2, F6/F4@390/320,
F12) PASS on all points; gate captures VI/EN 320/390/1440 overflowX=0. No
assertion changed, no backend file touched. Owner taste review still pending.

## Night pass P4 (+ audit P4 bucket) · 03/11/2026

Implemented by ZCode under the night coordination protocol: verdict chips use
localized badge nouns in quick and deep plates while the wire enum remains in
raw technical detail and inbox entries (F18); object error rows state an
owner-facing clause with the code and keep the raw parser message on hover and
in technical disclosure (F3); the retry action is wired into the error panel
and the runtime hint addresses the owner instead of the operator (F16);
witness detail prose is capped at a 68ch measure (F7); wildcard source ports
read "any" instead of ":0" (F8); the witness detail heading drops one level so
the section heading is not duplicated at rank 2 (F15); the inbox panel no
longer repeats the drawer summary as a visible heading while keeping the
unread badge visible (F5); the mobile rail gains scroll snap (F9); quiet
buttons carry a hairline edge (craft nit). Deferred with reason: F10 inbox
form grouping, F11 sample-run affordance, F17 row-level click (lower severity,
presentational only). Verification: npm test 19 files / 141 PASS (awaitVerdict
and three port/label assertions updated to the new user-visible wording, no
assertion dropped); production build PASS; browser suite 35/35 PASS; gate
captures VI/EN 320/390/1440 with overflowX=0. Owner taste review pending.

## Night pass P-M + P4.2 (F10/F11) · 03/11/2026

Motion audit (ZCode): every transition already resolves through `--dur-1` and
`--ease`; the three entrance-era keyframes (`rise-in`, `fold-in`, `fade-in`)
were dead code after the owner's craft decision and are removed; the spinner
and chevron state changes stay. F10: the watch form carries an accessible
group label. F11 root cause found and fixed: a budget scenario reuses the
same configuration text as the default sample, so the signature-keyed
auto-run attempt guard treated the switch as already-attempted and left the
result area empty; a sample switch now grants one fresh auto-run attempt
(verified in jsdom — analyze called after switch — and in the real browser —
outcome, warn banner and partial notice render). F17: row buttons already
carry per-object accessible names; row-level click remains deferred.
Verification: 19 files / 141 tests PASS; build PASS; browser suite 35/35;
captures VI/EN 320/390/1440 overflowX=0. Owner taste review pending.

### Morning fresh review, 03 November 2026

The night F8 substitution of numeric witness port 0 with `any` was incorrect.
CONTRACTS.md section 2 permits literal ports 0–65535. Packet examples now retain
both numeric values in quick/deep and the composition studies; actual region
wildcards remain labels for regions. Four new regression cases failed before the
fix; the frontend suite then passed 148 tests across 20 files. The browser suite
passed 35 checks. These are local synthetic checks, not device qualification.

The first rebuild also exposed an orphan keyframe tail (`to` rule and closing
brace) left by the night motion cleanup. Removed it; no additional animation or
palette change. Morning rebuild results and limitations are in the coordination
channel. Night screenshots showing `(any)` for a concrete packet are superseded.
