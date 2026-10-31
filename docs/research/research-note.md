# Config semantic diff — research note (31/10/2026)

Scope: short decision record for the configuration-level review slice. Evidence is
marked **FACT** (primary source or measured), **HYPOTHESIS** (product reasoning),
or **WALKTHROUGH** (synthetic scenario run by the agent, not user research).

## Sources confirmed

- **FACT** Cisco IOS ACLs are first-match and end with an unwritten deny:
  "The first match determines whether ... accepts or rejects ... stops the test
  of conditions after the first match" and "an implicit deny all clause at the
  end of every ACL" — Cisco, *Configure Commonly Used IP ACLs*.
- **FACT** Prefix-lists are evaluated in sequence order, first match wins, with
  an implicit deny; `ge`/`le` ranges are defined as "length < ge < le <= 32",
  exact match with neither, `ge` alone = ge..32, `le` alone = length..le; omitted
  sequence numbers start at 5 and increment by 5 — Cisco IOS XE BGP command
  reference and Catalyst 9200 routing command reference.
- **FACT** Batfish `compareFilters` returns pairs of lines from two filters that
  match the same flow but treat it differently, with columns Node, Filter_Name,
  Line_Index, Line_Content, Line_Action, Reference_Line_Index,
  Reference_Line_Content. It does not return an example flow. `searchFilters`
  returns Flow and Trace. Batfish's safe-refactoring analysis is exhaustive over
  all flows.
- **NOT CONFIRMED** by Cisco docs: whether `ge == le` is accepted; matching on
  the first `len` bits; host bits in the network address; empty/missing list
  behavior. Implementation choices therefore: accept `ge == le` (common device
  usage), reject host bits instead of normalizing, reject an entries-less list as
  unsupported, and document those as assumptions requiring device confirmation.

## Measured engine cost (FACT, synthetic corpus, Go 1.26.7, Windows/amd64, 8 CPU)

Fixed corpus of generated management/server ACLs with host/subnet sources, TCP/UDP
services, ranges, `neq`, mixed protocols, explicit final deny; five runs per case,
median; default limits 2s / 2M operations / 50k boxes; engine v0.1.

| rules | case | v0.1 result | ops | peak boxes | median |
|---|---|---|---|---|---|
| 100 | identical | EQUIVALENT | 1,831,170 | 5,239 | 533 ms |
| 100 | insert top deny | DIFFERENT | 2,000,000 (budget) | 5,716 | 545 ms |
| 250 | any case | INCONCLUSIVE | 2,000,000 | 12,475+ | 510–732 ms |
| 512 | any case | INCONCLUSIVE | 2,000,000 | 38,482 | 606–662 ms |

The bottleneck was not the cube algebra itself but rescanning/subtracting
fragmented permit sets (quadratic fragment×fragment work). v0.1 needed a budget
of roughly 2M operations even for 100 rules, so realistic 250+ rule ACLs could not
complete. Raw data: `docs/research/engine-baseline.json`.

## Three product bottlenecks (evidence + walkthrough)

1. **Intake was ACL-only (FACT from code, WALKTHROUGH).** Reviewers hold
   running-config backups, but the tool accepted a single ACL file. Pasting a full
   configuration failed with "only one ACL block is accepted". Manual work: extract
   the ACL, preserve byte identity, and re-check which parts were never reviewed.
2. **Results were witness-first, not change-first (WALKTHROUGH).** The result screen
   showed a verdict and up to N packets, but not which before/after rule pair owns
   each behavior change, nor how much was newly allowed vs newly blocked. The
   reviewer had to infer rule pairs from individual witnesses.
3. **The engine was the real completion bottleneck (FACT, measured above).** 100
   rules nearly exhausted the operation cap; 250 rules were INCONCLUSIVE. Raising
   limits or caching would not help; the work itself had to shrink.

A fourth observation (HYPOTHESIS, not selected): the tool is ACL-shaped. The user
direction requires semantic diff for network configuration generally. This was
addressed at the architecture level (domain-neutral intake/artifact and a second
domain) rather than as a separate feature.

## Chosen improvements and acceptance criteria

1. **Configuration intake and object inventory** (domain-neutral). Accept a full
   configuration, isolate each supported object with exact byte and line mapping,
   pair by domain/name, and explicitly inventory unsupported and unanalyzed parts.
   Accept: standalone ACL digests unchanged; CRLF block bytes exact; object-group,
   standard ACL, IPv6 and unanalyzed sections reported, never silently dropped; no
   verdict for unsupported objects.
2. **Change classes** (ACL) and **exact route-filter classes** (prefix-list).
   Group the exact difference by deciding before/after rule pair; each class has an
   independently re-evaluated witness, a packet/prefix example, and a scope
   classification only when an operator-accepted allowed scope is supplied.
   Accept: exhaustive reduced-domain oracle for both domains; class attribution
   matches the linear evaluator; scope outside/inside classification verified;
   no packet count or "percent safe" claim.
3. **Engine v0.2**: index candidate cuts and decide fragments against raw rules in
   first-match order; versioned so v0.1 receipts replay under v0.1 accounting.
   Accept: identical boxes/verdicts on the corpus, all old receipts replay, more
   cases complete under the same caps, measured before/after.

## Explicit non-claims

- `WALKTHROUGH` scenarios are synthetic, agent-run demonstrations, not interviews,
  customer requests or usability research with real operators.
- A `DIFFERENT` verdict is a behavior difference, not automatically "dangerous"; an
  `EQUIVALENT` verdict is not proof of intent fidelity; an error is not automatically
  unsafe. Only budgets, unsupported pieces and unanalyzed parts constrain claims.
- Prefix-list and ACL providers are not modeled (no interface/BGP binding), so the
  artifact states `behavior_evaluated_per_object_without_binding`.
