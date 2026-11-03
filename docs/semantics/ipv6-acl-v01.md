# IPv6 ACL semantics — parser `cisco-acl6/v0.1`, engine `acl6-exact/v0.1` (P3-7)

Plan 05 scopes P3-7 as an algebra-width change, not new semantics: the
IPv6 engine is a parallel construction (`internal/packetset6`,
`internal/ciscoacl6`, `internal/verify6`) mirroring the IPv4 engine's
constructions and budget discipline, so the verified IPv4 path is never
touched. This note records the decisions and the evidence.

## Packet model

A packet is (next-header protocol, source, destination, source port,
destination port, ICMPv6 type). Address cubes are 128-bit value/wildcard
word pairs. The ICMPv6 type is a first-class interval dimension because
IPv6 ACL semantics turn on it (below); it is consulted only for protocol
58 and held at the full range elsewhere.

## The implicit tail — confirmed from Cisco documentation, not inferred

Per Cisco's IOS IPv6 command references (e.g. the IPv6 ACL chapter of the
IOS XE / IOS XR command references, "ipv6 access-list" entry):

> "Every IPv6 ACL has implicit `permit icmp any any nd-na`,
> `permit icmp any any nd-ns`, and `deny ipv6 any any` statements as its
> last match conditions."

Consequences the model honors:

- An ACL whose text never mentions ICMPv6 still permits neighbor
  solicitation (type 135) and neighbor advertisement (type 136).
- An explicit broad rule above the tail shadows it as usual (an explicit
  `deny ipv6 any any` denies ND too — the tail is last).
- The scoped cross-check `TestCompareImplicitTailEquivalence` pins the
  observable consequence: adding explicit ND permits to an ACL is a
  semantic no-op.

## Comparison construction

An ACL's decision function is its permit set (everything not permitted
is denied, explicitly or by the tail), so equivalence is permit-set
equality. The full permit set is `E ∪ (ND ∖ D)` with E the effective
explicit permit region and D the decided region. The difference of two
permit sets is computed in a layered decomposition whose minuends stay
explicit-sized — materializing `ND ∖ D` (a wide region minus narrow
holes, intrinsically ~250 boxes at 128 bits) and differencing two such
regions blows the shared operations budget on ACLs as small as two rules
(measured during development: 2,000,000 operations exhausted, 259 peak
boxes). Region accumulations are kept near their minimal cover by exact
cube/interval coalescing in `packetset6`.

## Grammar profile (v0.1)

- One `ipv6 access-list NAME` block; optional increasing sequence
  numbers; remarks; `log` / `log-input`.
- Protocols: `ipv6` (all), `tcp`, `udp`, `icmp` (= ICMPv6, 58), or a
  decimal 0–255.
- Address operands: `any`, `host <addr>`, `<addr>/<prefix-len>` (IPv6
  ACLs have no wildcard-mask form).
- Port operators for TCP/UDP: `eq`, `neq`, `lt`, `gt`, `range`.
- ICMPv6 types by IOS name: destination-unreachable (1), packet-too-big
  (2), time-exceeded (3), parameter-problem (4), echo-request (128),
  echo-reply (129), and the ND set nd-rs / router-solicitation (133),
  nd-ra / router-advertisement (134), nd-ns / neighbor-solicitation
  (135), nd-na / neighbor-advertisement (136). Other named types fail
  closed (`PARSE_UNSUPPORTED`).
- Not in v0.1: object-groups for IPv6, extension-header matching
  (`routing`, `hbh`, `dscp`, `flow-label`, `fragments`, `time-range`,
  `reflect`), ICMPv6 code matching. All fail closed.

## Analysis integration and limits

- IPv6 ACL objects are analyzed when the run's recorded versions carry
  the v6 parser identity (`versions.acl6_parser` / `acl6_engine`,
  additive with `omitempty`): artifacts sealed before P3-7 record no v6
  identity and replay with IPv6 unsupported, byte-for-byte.
- Object payload `acl6`: verdict (EQUIVALENT / DIFFERENT / INCONCLUSIVE)
  plus witnesses, each validated through the independent evaluator
  before it is recorded.
- Not covered for IPv6 in this version: change-class explanation,
  obligations and repair, and the global budget ledger (v6 objects carry
  no IPv4 budget payload; `objectCharge` reads zero). These are
  follow-up scope, not silent gaps: the artifact records exactly the
  verdict layer.

## Independent checks

- `packetset6`: randomized point-membership oracle over a reduced
  subspace (difference/intersection vs brute force) + pairwise
  disjointness of single-box differences.
- `verify6`: verdicts vs a brute-force packet-grid oracle over
  2001:db8::/124 with rule operands drawn from grid-aligned prefixes
  (120 random pairs), plus the implicit-tail equivalence case above.
