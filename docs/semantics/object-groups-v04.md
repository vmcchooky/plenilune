# Object-groups for named extended IPv4 ACLs — parser v0.4

Status: implemented for P3-6 (parser, point evaluation, region algebra,
analysis intake). Production status lives in docs/plan/STATUS.md.

## Design decision, measured (plan 05 requires the data)

Plan 05, item P3-6, required the representation to be chosen from prototype
data: unify group membership in the IR (union) instead of textual expansion
if expansion blows up combinatorially. It does, at sizes that are small in
production terms. Prototype (scratch harness over the real engine, a
self-pair whose only edit adds one host to the first source group):

| Logical ACL (5 rules unless noted) | Expanded physical rules | Result under expansion |
|---|---|---|
| groups 4×4 | 81 | DIFFERENT, complete, ~6 ms |
| groups 8×8 | 321 | DIFFERENT, complete, ~29 ms |
| groups 16×16, **2 rules** | 513 | **unsupported** — 1 rule past MaxRules (512) |
| groups 16×16 | 1281 | unsupported |
| source group 32 × dst 8 | 1281 | unsupported |

Raising MaxRules is not an option (owner guardrail). Decision: **union in
the IR**. A rule that references groups stays ONE rule; its address fields
become member sets and the region algebra works on the set directly. The
16×16 × 5-rule case above analyzes as 5 logical rules: DIFFERENT, complete.
Group form and expanded form were cross-checked to agree on verdicts at
every size where the expanded form is analyzable at all
(`internal/analysis/p36_groups_test.go`).

## Model and source authority

The profile targets Cisco IOS object-groups as used by named extended IPv4
ACLs on routers: `object-group network` (members: `host`, network + subnet
mask, nested `group-object`) and `object-group service` for tcp/udp
(members: `port-object` with the usual operators, nested `group-object`).
A rule may reference a network group in either address position and a
service group in either port position. As in IOS, an `object-group` token
in a port position is a service reference only when the name resolves to a
service group; a network group of that name is taken as the following
address operand. Definitions may appear before or after the ACL block;
references resolve after the whole document is read.

Network group members carry a **subnet mask** (converted to a wildcard
internally); ACL rule operands keep wildcard masks, as in IOS.

## IR representation

- `Rule.SrcSet` / `Rule.DstSet` (`src_set` / `dst_set`, additive DTO
  fields): the resolved member set of a referenced network group —
  normalized addresses, sorted by (value, wildcard), exact duplicates
  removed, **no subsumption pruning** (a /24 member and a host inside it
  both stay; membership is a set, not a minimization). When a set is
  non-empty it is authoritative and the single `Src` / `Dst` must be the
  zero address; `ACL.Evaluate` validates this contract.
- Service groups resolve at parse time into the rule's existing port
  interval sets, merged into canonical form (sorted, overlaps and
  adjacencies merged). No new DTO field is needed.
- Point evaluation (`ACL.Evaluate`) matches a packet against ANY member —
  it never expands rules, so the parser, the point evaluator and the box
  algebra are three views of the same semantics (guardrail: independent
  reduced-domain checks; the analysis cross-check above is one).
- The region builder crosses member cubes **inside one rule's region**;
  live-box budgets bound the growth exactly as they bound port sets.
- Semantic rule equality (LCP alignment) and the repair rule key both
  include the sets: a group-membership change is a rule change.

## Versioning and replay

- Parser version bumps to `cisco-acl/v0.4` (`GroupParserVersion`). v0.3 and
  v0.2 keep their exact prior behavior — group syntax under a v0.3 request
  fails exactly as it did before v0.4 existed, so frozen artifacts replay
  byte-identically. Group-free documents parse to identical DTOs under
  v0.2/v0.3/v0.4 (pinned by `TestParserVersionDispatch`).
- Standard ACLs are accepted under v0.3 and v0.4 (the kind predates the
  bump and must not lose support); groups do not apply to standard ACLs.
- Capabilities document: `parser_version` moves to v0.4 (golden regenerated
  with `-update`; the diff is that one line).
- Repair: `parserForRepair` for the current repair version deliberately
  stays at v0.3 — repair evidence semantics do not change until a repair
  version bump adopts group parsing explicitly.
- DTO validation: the port-interval cap per rule rises from 2 to 128
  (`MaxPortIntervals`), because a service group legitimately resolves to
  many disjoint intervals. Group-free parser output is unchanged (≤2), so
  older DTOs validate exactly as before.
- Analysis intake: when a document carries object-group sections, each
  named-extended ACL is re-parsed with its group sections spliced in
  (other lines blanked, line numbers absolute). The object's effective
  source — and its digest — becomes the spliced text, so a group-only
  edit digests differently from the previous version. Only artifacts
  recorded at parser v0.4 take this path.

## Accepted forms and fail-closed boundary

- Definitions: `object-group network NAME`, `object-group service NAME
  tcp|udp|tcp-udp`; members as above; nesting via `group-object`.
- Bounds: ≤64 group definitions per document, ≤1024 resolved members per
  group, nesting depth ≤8, ≤128 resolved port intervals per rule field.
- Fail closed (typed `PARSE_GROUP_*` / parse errors, never a silent
  fallback): undefined group, reference cycle, duplicate definition,
  empty group, service group used with a protocol it does not cover,
  service group on a non-TCP/UDP rule, malformed member lines.
- Error messages never embed source tokens (the group name included);
  the typed code and line locate the problem, per the redaction rules.
- Out of scope for v0.4: protocol object-groups (a rule whose protocol
  position is `object-group` fails closed), object-groups in standard
  ACLs, IPv6 object-groups, `range` address members, group rename/edit
  operations in repair proposals.
