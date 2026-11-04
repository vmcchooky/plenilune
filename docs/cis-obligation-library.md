# CIS obligation library (P3-8)

A small library of ready-made obligation sets for the ACL-expressible
part of the CIS Cisco IOS Benchmark's data-plane guidance, shipped as
data under `examples/cis/` in the obligations wire format
(`plenilune.obligations/v1`). Each set plugs into an analysis policy
unchanged (`conditions` of a `plenilune.policy/v1` document).

## Method and limits — read this first

- **No CIS text is redistributed.** The benchmark is CIS-licensed; this
  library encodes the *substance* of its ACL-related recommendations as
  Plenilune obligations and cites the benchmark by name and theme. The
  benchmark's decimal numbering differs across its versions (IOS 16,
  IOS 17.x, IOS XE) and the licensed text is not available to this
  project, so entries carry stable library IDs (`CIS-ACL-nn`) plus the
  benchmark theme they encode. Sites holding a CIS license map the
  library IDs against their own benchmark copy.
- **Obligations judge an ACL's decision function, not a device.** A set
  marked for an internet-facing ingress ACL will (correctly) "violate"
  an internal ACL that legitimately permits RFC 1918 sources. Choosing
  which set applies to which ACL is the operator's scoping decision,
  exactly like choosing the ACL's interface and direction on a device.
- The underlying packet facts are public standards, cited per entry:
  BCP 38 / RFC 2827 (ingress source validation), RFC 1918 (private
  addressing), RFC 8190 and the IANA special-purpose registry (bogon
  ranges), RFC 6598 (shared address space).

## Encodable — shipped sets

| Library ID | Set file | Theme (CIS Cisco IOS Benchmark, data plane) | Obligations | Applies to |
|---|---|---|---|---|
| CIS-ACL-01 | `examples/cis/ingress-bogons-core.json` | Ingress anti-spoofing: a packet arriving from an untrusted network must not claim a this-network, private, loopback, or link-local source | 6 × `must_deny` (protocol `ip`): sources 0.0.0.0/8, 10.0.0.0/8, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.168.0.0/16 → any | Internet-facing **ingress** ACLs |
| CIS-ACL-02 | `examples/cis/ingress-bogons-special.json` | Ingress anti-spoofing, special-purpose ranges: shared address space, IETF protocol assignments, documentation prefixes, benchmarking range, multicast and reserved space must not appear as sources | 8 × `must_deny`: 100.64.0.0/10, 192.0.0.0/24, 192.0.2.0/24, 198.18.0.0/15, 198.51.100.0/24, 203.0.113.0/24, 224.0.0.0/4, 240.0.0.0/4 → any | Internet-facing **ingress** ACLs |
| CIS-ACL-03 | `examples/cis/cleartext-services.json` | Plaintext and legacy services must not be reachable: Telnet, TFTP, rlogin, rsh, and the RPC portmapper are denied rather than merely unused | 6 × `must_deny`: tcp/23, udp/69, tcp/513, tcp/514, tcp/111, udp/111 (any → any) | Transit and infrastructure ACLs; narrow the destination CIDR per site before enforcing on device-bound ACLs |
| CIS-ACL-04 | `examples/cis/egress-template.json` | Egress source validation (BCP 38): only the site's own prefixes may leave | 1 × `must_deny`: source any **except** the site prefix → any | **Template.** The shipped exclusion is the documentation prefix 198.51.100.0/24 — replace it with the site's real allocated prefixes before use |

Every set is validated in CI: `internal/verify/cis_library_test.go`
normalizes all four files through the obligations validator and checks
the core bogon set against a permissive ACL (all obligations VIOLATED)
and a filtered ACL (all HOLDS).

## Not encodable — and why

The rest of the benchmark's ACL-adjacent guidance is deliberately not
encoded, because the obligation model (protocol + CIDRs + port
intervals over a transit ACL's decision function) cannot express it
without lying:

| Benchmark theme | Why it is not an ACL obligation |
|---|---|
| VTY / access-class restrictions, SSH version and crypto settings | Management-plane configuration and ACL *binding*, not a transit ACL's decision function |
| SNMP community strings, SNMP ACL binding | Binding and credential configuration; the ACL alone does not carry the semantics |
| AAA / TACACS+ / RADIUS, login banners, password policy | Not packet filtering at all |
| Logging requirements (`log` / `log-input`, syslog targets) | A rule action modifier, not a permit/deny region; the grammar models it as metadata, not as a match dimension |
| ICMP message-type filtering on IPv4 (redirects, mask requests, …) | The IPv4 obligation region has no ICMP type dimension (the IPv6 engine's type dimension arrived with P3-7, but obligations remain IPv4-scoped in this version) |
| Fragment and IP-option matching (`fragments`, `option …`) | Outside the modeled grammar; such rules fail closed at parse |
| Time-based rules (`time-range`) | Outside the modeled grammar and the static decision function |
| Reflexive ACLs / stateful inspection | Stateful; the engine is a static decision-function model |
| Control-plane policing (CoPP), uRPF | Different mechanisms (control-plane policy, RPF check), not interface ACL content |
| "An ACL is applied to every external interface" | A binding/coverage property of the device configuration, not of any single ACL |

## Versioning

Sets are plain obligations documents: they version with the
`plenilune.obligations/v1` schema and digest like any operator-written
set. Corrections to a set's ranges land as ordinary changes to the JSON
files; the library IDs above are stable names for the themes, not
versions.
