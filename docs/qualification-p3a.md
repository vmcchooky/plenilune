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
