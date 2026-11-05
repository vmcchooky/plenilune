# Standard IPv4 ACL profile — parser v0.3

Status: implemented for P3-5 (parser, independent oracle, intake, repair
and frozen replay gates passed; merged via PR #54). Production status
lives in docs/plan/STATUS.md. A runnable pair lives in
`examples/standard-acl/`.

## Model and source authority

This profile targets the documented Cisco IOS **router** standard ACL model.
It does not claim ASA, NX-OS or universal Catalyst semantics. Source-only matching,
numbered ranges and address syntax follow Cisco's [configuration guidance](https://www.cisco.com/c/en/us/support/docs/security/ios-firewall/23602-confaccesslists.html).
The [IOS XE 16.12 sequence chapter](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/sec_data_acl/configuration/xe-16-12/sec-data-acl-xe-16-12-book/sec-acl-seq-num.html)
describes exact-host lookup preceding the sequence-ordered subnet list, automatic
sequence assignment, duplicate-sequence rejection and the 2147483647 bound.
The [IOS XE 17 router chapter](https://www.cisco.com/c/en/us/td/docs/routers/ios/config/17-x/sec-vpn/b-security-vpn/m_sec-acl-seq-num.html)
provides the corresponding router profile. No physical device has been tested.

Lowering source lines directly as an extended ACL would therefore be wrong when
a network deny appears before an exact-host permit. The canonical IR must reflect
the documented effective order, while retaining each original physical line.

## Accepted forms and fail-closed boundary

- Named form: `ip access-list standard NAME`, including a standard-range numeric
  name, with permit/deny rules and remarks.
- Old numbered form: `access-list N ...`, N in 1–99 or 1300–1999; every executable
  line in a standalone document must belong to the same identity.
- Source operands: `any`, `host IPv4`, bare IPv4 (exact host), or IPv4 plus Cisco
  wildcard. Noncontiguous wildcards retain bit-mask semantics.
- A rule constrains only the source address; destination, protocol and both port
  dimensions span the existing modeled IPv4 packet universe. Final implicit deny
  remains a filter decision, never a statement about device reachability.
- Sequence allocation includes remarks. Explicit network sequences determine
  evaluation order; missing sequences receive deterministic append positions.
- Unsupported tails, dynamic/time constraints, object groups, hostnames, invalid
  identities, duplicate/overflow sequences and malformed later rules fail closed.
- Repeated exact-host predicates are explicitly unsupported in this profile,
  including identical duplicates. IOS duplicate-entry no-op behavior is not
  silently guessed or used to discard source provenance.
- Existing input byte/line/rule limits and UTF-8/control-character checks apply.

The two forms normalize to kind `standard`; changing representation alone must
not manufacture a semantic identity mismatch for the same numeric ACL name.

## Canonical rules and source identity

`Rules` is in effective evaluation order: exact hosts first, then wildcard
networks by effective sequence. Stable ordering among distinct exact hosts is a
deterministic representation choice; their predicates do not overlap.

`Ordinal` is the zero-based index in that effective list. `Line` is the original
physical source line, and can decrease between adjacent standard rules. Each line
must remain valid and unique. `Sequence` records normalized effective sequence
for standard rules, including assigned values where the source omitted them; it
does not assert a literal source token or an observed device counter. Frozen
extended-parser output keeps its existing representation.

This distinction preserves verifier, audit and partial-result ordinal invariants.
Witnesses and patches bind the effective ordinal **and** physical line. Rendering
must not infer the last physical rule from the last evaluation ordinal, or
renumber standard network rules in physical order and thereby change semantics.

## Version and replay requirements

Fresh support arrived in `cisco-acl/v0.3` and continues in the current
`cisco-acl/v0.4`; `cisco-acl/v0.2` remains explicit legacy
dispatch and must continue rejecting standard ACLs. Old trace, comparison,
analysis, audit, repair and enumeration records retain their recorded parser and
artifact identities. No frozen fixture is rewritten merely to match the new
default. Intake classification and repair rendering changes require compatible
version dispatch as well as parser dispatch.

## Required independent evidence

Goldens precede parser changes. The reduced-domain oracle must independently
perform exact-host lookup followed by a sequence-ordered network scan; it must
not call production canonicalization to obtain its expected answers. It covers
compare, obligations, changes, audit, budget-stopped partials and witnesses.

Integration covers both syntaxes, full-configuration source mapping, policy
acceptance, append/remove/replace at nonmonotone source positions, CRLF, replay,
and explicit capability/unsupported reporting. These gates passed for P3-5
(see docs/plan/STATUS.md).
