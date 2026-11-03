# Three composition studies · 02/11/2026

Owner request: implement the next step with three different compositions under the
agreed cream/olive direction. These are alternatives for review, not an approved
golden screen. The normal app entry is retained.

## Open locally

- A: http://127.0.0.1:8098/?design=a — compact change record.
- B: http://127.0.0.1:8098/?design=b — source with inline consequence.
- C: http://127.0.0.1:8098/?design=c — change-group list with focused detail.

The selector changes layout without rerunning analysis or changing evidence. Direct
loads analyze the existing ssh-drift synthetic sample through useReview. No verdict,
witness or change group is fabricated. All three share the same tokens and typefaces.

| Study | Reading order | Interaction | Tradeoff |
|---|---|---|---|
| A | Outcome, example route, deciding rule, next action | Expand before/after rules; open conditions | Direct for one change; further groups need detail |
| B | Source, deciding line, adjacent consequence | Toggle source version; keyboard-scroll code | Strong source context; more reading on mobile |
| C | Select group, affected region, deciding rules | Select group; expand witness; open conditions | Scales to several groups; overhead for one group |

Author recommendation for quick entry on this sample: A. B suits reviewers starting
from source. C suits reviews where selection/grouping is a major task. This is not
owner approval or a user study.

## Implementation boundaries

The primary action opens the existing manual ConditionsStep and canonical policy
preview; acceptance remains explicit. The preview does not introduce another repair
workflow. The actual app retains complete repair/replay. No backend contract,
dependency, model request, device access or publication is introduced.

UseReview binds sources and results; presentational components read props. A witness
is labeled an example. C calls a region complete only for one untruncated piece;
otherwise it labels it an example. Incomplete, truncated and unmodeled coverage
notices remain visible. Source lines map to original line numbers. The independent
code scroll region is keyboard focusable.

## Verification

- Production build PASS; existing main-chunk size warning remains.
- Frontend: 17 files, 138 tests PASS; existing jsdom scrollTo notices remain.
- Go formatting, vet, ordinary tests, race tests and build PASS; Go tests cached.
- Browser: 26 viewport/state checks PASS, VI/EN, A/B/C, 320/390/768/1440. No document
  horizontal overflow; zero serious/critical axe findings after fixing code-scroll
  keyboard access.
- Browser assertions cover the same evidence digest and no analyze/compare rerun on
  switches, actual deciding rules, actual group count, condition preview through the
  local server, initially unchecked acceptance, no external or AI-session requests,
  and the normal app entry.
- Skill UI lint on the new folder: zero errors/warnings. Components under 100 lines.
  Dynamic annotation alignment uses a CSS custom property; visual values use tokens.

Local artifacts: out/compositions/results.json, lint.json, reading-metrics.json,
full-page VI/EN captures and *-composition.png crops. Author inspected all three
desktop layouts and B mobile. No independent visual critique, human trial, complete
keyboard journey, 200% zoom or screen-reader qualification is claimed.

Sample-specific word counts excluding selector/intro at 1440px: A 90, B 125, C 113.
These are reading-volume observations, not measured comprehension.

Owner selected A, with remaining visual feel concerns. See [A refinement](qa/record-refinement.md).
Next gate: refine and approve the representative screen and difficult states before wider migration.

## Night integration, 2026-11-02

The owner authorized Codex, ZCode and OpenCode to continue implementation and make
team decisions through 07:00 on 3 November. A is now reused by the actual quick
entry through ChangeRecord rather than copied CSS. Multiple groups require an
explicit selection before showing an example; the single-group case remains
immediate. Conditions still open the existing explicit acceptance workflow.

Codex verification before the independent P2 gate: TypeScript PASS, 19 files / 141
tests PASS. ZCode owns production builds and independent browser/visual gates.
This authorization does not establish final owner taste approval or human
usability/screen-reader qualification. See coordination/UI-NIGHT-2026-11-02.md.
