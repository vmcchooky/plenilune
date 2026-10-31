# IPv4 prefix-list model v0.2

Profile: `ios-style-canonical-ipv4-prefix-list/v1`. Current parser
`cisco-prefix-list/v0.2` + engine `prefix-list-exact/v0.2` identify this profile
in the sealed analysis `versions` and each prefix-list result. This is an exact
model of a declared IOS-style subset, **not device/firmware qualification**.

## Normative model

- A route is a canonical IPv4 network and a length 0–32. The full domain contains
  `2^33 - 1 = 8,589,934,591` distinct route prefixes, not that many observed routes.
- A nonempty list is evaluated by increasing sequence; first matching entry wins.
  No match denies. Application at an interface, neighbor, route-map, redistribution
  or routing protocol is outside this model; filter difference is not proof of a
  route being advertised, installed or reachable.
- An entry matches a route inside its network with length in an inclusive interval:
  no qualifiers → `[len,len]`; ge only → `[ge,32]`; le only → `[len,le]`;
  both → `[ge,le]`. In this subset ge/le must exceed len and be at most32;
  both allow `ge == le`, meaning one length.
- Explicit sequences: 1–4,294,967,294. An omitted sequence is5 initially, then
  highest sequence **seen in source order** +5. Overflow, duplicates and unresolved
  history are rejected, not reordered into a different meaning.
- Canonical network addresses only; host bits are rejected rather than silently
  masked. Empty/description-only lists are unsupported, not modeled as deny-all
  or permit-all. Names use the documented local parser subset (1–63 characters;
  detail, summary and sequence-number are reserved). IPv6/eq/other syntax is not
  inferred from another Cisco family.

## Evidence and limits

Counts are exact canonical route-prefix counts only when report.complete=true.
Classes/ranges may be truncated independently; a reported class count remains
exact, but summing a truncated class list is not the total changed route space.
Budget exhaustion yields INCONCLUSIVE with classes withheld, never a partial
safe result. Current logical accounting charges partition/search/merge/sweep;
entry count and bounded source text also cap validation work. Cancellation is
checked within search/merge/sweep loops.

Public Compare/Evaluate validate exported IR, including semantic fields, derived
intervals, unique source refs and source-order sequence resolution. The interval
engine's witnesses are checked by a separate direct mask/first-match evaluator.
This does not make the parser or model independently device-verified.

## Source disposition

| Item | Basis / status |
|---|---|
| First match, implicit deny, exact/no qualifier, ge/le ranges, sequence bound/default | [Cisco IOS BGP command reference](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/iproute_bgp/command/irg-cr-book/bgp-c1.html), ip prefix-list section; [IOS XE data model](https://netascode.cisco.com/docs/data_models/iosxe/device/prefix_list/). |
| Containment plus inclusive ge/le meaning | [Cisco Live BRKCCIE-3000, slide47](https://www.ciscolive.com/c/dam/r/ciscolive/us/docs/2018/pdf/BRKCCIE-3000.pdf). |
| Equality and syntax acceptance on a particular release | Operator meanings imply inclusive matching; some command-reference formulas use stricter inequalities. The profile explicitly chooses inclusive equal endpoints. No target IOS/XE CLI lab has qualified acceptance. Do not claim all Cisco firmware accepts this syntax. |
| Host bits and empty definitions | Deliberate subset exclusions; no claim that all platforms reject them. |

No NX-OS, IOS-XR or FRR result substitutes for qualification of a claimed IOS/XE
release. A later platform profile must be separately named/versioned if its
semantics differ; do not change an existing proof's meaning in place.

## Compatibility and trust

Prefix parser/engine v0.1 remain frozen **for replay only**. New analysis always
uses v0.2, including stricter sequence/IR checks and new operation accounting.
Unsupported v0.2 input is never retried automatically with v0.1. Only exact
supported parser/engine pairs are accepted; unknown or mixed versions fail.

Original v0.1 analysis fixtures, generated before this change, live under
testdata/receipts. One deliberately contains the old parser's out-of-profile
maximum sequence; replay preserves historical computation without certifying
that syntax as valid on a Cisco device.

Analysis replay checks exact non-timing data even when one ACL is timing-limited:
sources, request/policy, versions, inventory/extents, coverage, prefix-list reports
and unaffected ACL components. Recorded ACL witnesses and necessary scope-point
conditions are rechecked. TIMING_SENSITIVE still withholds full region/proof
matching for the affected computation; it never authenticates engine provenance
or upgrades the stored verdict. Hash/Seal alone are content checks, not signatures.
