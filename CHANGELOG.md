# Changelog

All notable changes to Plenilune are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
release versions follow [Semantic Versioning](https://semver.org/).
Schema versions (evidence, analysis, capabilities, bundle, signature)
are versioned separately under the schema policy recorded in
`docs/plan/STATUS.md` (P0-D1) and are pinned in the capabilities
document, not by this file.

## [1.0.0] — 2026-11-03

First official release.

### Added

- Semantic diff and intent verification for Cisco IOS named extended
  IPv4 ACLs and IPv4 prefix-lists, computed exactly over a five-field
  packet algebra (engines `acl-exact/v0.4`, `prefix-list-exact/v0.2`):
  verdicts EQUIVALENT / DIFFERENT / INCONCLUSIVE with witnesses and
  per-rule-pair change classes. Budget exhaustion never upgrades to a
  verdict; budget-stopped results are recorded as partial
  INCONCLUSIVE with the decided prefix preserved.
- Obligations (`must_permit`, `must_deny`, `must_remain`) evaluated
  independently of the global verdict, each with its own outcome and
  witness.
- Verified in-memory repair with exact collateral checking, and a
  capped enumeration surface for external repair proposals.
- Exact ACL audit (shadowing and redundancy from cumulative effective
  regions; audit never deletes anything).
- Evidence artifacts with dual digests — `content_sha256` for
  semantic identity, `sha256` for the exact sealed artifact — and
  replay as the only verification primitive (evidence schema v2,
  analysis schema v3).
- A capabilities gating document (`plenilune.capabilities/v1`), a
  75-code error taxonomy across 7 stages with explicit retryability,
  review bundles (`plenilune.bundle/v1`), and Ed25519 detached
  signatures binding both digests of a sealed artifact.
- Verdict inbox in the local server: watches on named objects with
  verdict-change, any-different and obligation-failure triggers.
- Offline local web UI: a verdict-first quick flow, the full review
  flow, and an interrogation chat. The AI session is opt-in and
  proposal-only — the deterministic engine owns every verdict,
  acceptance and replay.
- MoonStorm integration contract (subprocess + versioned JSON, no
  library import), specified in `docs/integration/moonstorm.md` with
  a working sample client.

### Replay support

- This release replays artifacts sealed by tool version `1.0.0` and
  by the pre-release development line (`0.2.0-dev`, never an official
  release). Artifacts sealed by any other tool version are rejected.
  See the supported-versions policy in `SECURITY.md`.

### Known limitations

- IPv4 only; named extended ACLs and prefix-lists only. Standard
  ACLs, object-groups and IPv6 are future work.
- No device connection: Plenilune reads configuration text only and
  never pushes configuration anywhere.
- Verdict-inbox state lives in server memory and resets on restart.
