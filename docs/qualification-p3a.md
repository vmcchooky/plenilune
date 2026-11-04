# P3A qualification ledger

Assessment date: 2026-11-02. Only synthetic inputs may be used. An automated
accessibility scan or viewport emulation does not qualify a screen reader or
a physical mobile device. A backend model receipt does not qualify live UI.

| Required evidence | Current status | Completion procedure |
|---|---|---|
| Model live UI | Not run: no `META_MODEL_API_KEY` in this task's process environment; owner authorized using `META_API_KEY`, but it is absent in process, User and Machine scopes too | Start local runtime with the key held only by the server. In VI and EN, explicitly consent, propose conditions, inspect and accept canonical policy, propose/verify repair, record decision, export, then replay in a fresh AI-off session. Record model ID, build commit, statuses, actual latency and failures without key/config contents. |
| Screen reader | Not run; previous axe/keyboard checks are insufficient | With Narrator or NVDA, record OS/browser/reader versions and manually check source labels, quick verdict, disclosure order, policy acceptance, asynchronous errors/status, repair decision, stale warning and evidence replay in VI and EN. Record announced text and findings. |
| Physical mobile UI | Not run: no physical mobile browser/session identified | Record device, OS, browser, build and input method. Run synthetic source → verdict → conditions → repair → export/replay; verify focus, scrolling, touch targets, zoom and file import/download. Desktop viewport emulation is supplemental only. |

For each run preserve a dated result with build commit, exact synthetic case,
steps, expected/observed behavior, findings and their disposition. Never record
credentials. P3-2 remains open until these real runs have evidence; this ledger
is a reproducible handoff, not a qualification pass. Optional operator research
is outside the release gate.

## Disposition 04/11/2026 - P3-2 closed PARTIAL (owner decision, not a full qualification)

Owner authorization 03/11/2026: P3-2 closes as PARTIAL, with the limits
recorded verbatim below. This disposition supersedes the "remains open"
sentence above for the purpose of closing P3-2 in `docs/plan/STATUS.md`;
it does NOT upgrade any item below to a full qualification, and it does
not authorize claiming live-UI, screen-reader, or physical-device
qualification beyond the scope actually evidenced.

Verbatim limits (owner authorization 03/11/2026): live UI + NVDA achieved
within the recorded scope; AI-proposed conditions 0/3 usable and
real-device not qualified are known limits that do not block the rest of
Phase 3.

| P3-2 item | Disposition | Evidence (as reported by the local Windows agent, 02/11/2026) |
|---|---|---|
| Live UI with model (synthetic end-to-end) | PARTIAL - not full | Build 1.0.0 (`bcb0646`), model `muse-spark-1.3-contributor`, key entered by the owner into the server process only. Engine/compare, repair, evidence+replay, and inbox flows evidenced (compare 217 ms; repair candidate VERIFIED; replay conclusion unchanged; inbox DIFFERENT to EQUIVALENT). AI-proposed conditions: 0 usable proposals in 3/3 runs across 2 builds (one `partial` run, two `clarification_needed` runs) - that flow has no end-to-end pass evidence. The UI correctly did not adopt proposals or change verdicts on its own. |
| Screen reader | ACHIEVED within the recorded scope only | NVDA 2026.3 (portable), 5 screens (quick verdict, conditions and scope, result, repair, evidence) + verdict inbox, Tab/heading/landmark navigation; no blocking issue found in that scope. Evidence runs were taken on `4b58ae7`; they apply to 1.0.0 only because `web/` is byte-identical between `4b58ae7` and `bcb0646`. Not extended beyond the 5 screens + inbox covered. Known non-blocking observations: evidence screen heading order (h1 + h3, no h2) and step rail `<nav>` preceding `main`. |
| Physical mobile device | NOT QUALIFIED | No reliable recorded result: the owner opened the demo URL on a phone over the same Wi-Fi (build `4b58ae7`) but answered "not sure" about errors and chose not to retest on 1.0.0. Desktop viewport emulation (390x844, no page-level horizontal overflow) is supplemental evidence only and does not qualify a physical device. Touch-target note (non-blocking): several controls measured 40-42 px, below the 44 px recommendation. |
| Operator research | Not done; outside the release gate | Only if the owner wants it. |

Follow-up (not a condition for this PARTIAL closure): re-run the
AI-proposed-conditions flow and the physical-device run when the owner
chooses, and record them as new dated evidence here; a future full
qualification, if ever claimed, must be a fresh owner decision on that new
evidence, not an extension of this PARTIAL disposition.
