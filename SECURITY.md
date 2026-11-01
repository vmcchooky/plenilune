# Security policy

Status: **active for the prepared public repository** (31 October 2026).
Plenilune is licensed GPL-3.0-only; see [LICENSE](LICENSE) and [NOTICE](NOTICE).
The curated public snapshot is described in
[docs/public-catalog.md](docs/public-catalog.md). Remote push, deployment and
publication remain owner actions; the local workflow does not perform them.

## Supported versions

There is no versioned release train. The current source tree is the only
supported state; older snapshots and local builds are not supported. This
section will be replaced when a release policy exists.

## Reporting a vulnerability

**Security contact: chooky@quorix.io.vn** (public by owner decision). Report
privately; do not open a public issue for an unpatched vulnerability. If the
hosting platform offers private vulnerability reporting, that channel may also
be used; email is the current reporting address.

A useful report includes the affected version/commit, a minimal synthetic
reproduction, expected and actual behavior, and whether the issue crosses the
loopback boundary, the parser/engine boundary, receipt binding or key handling.
Do not include real credentials or customer data. No PGP key is published yet;
do not send secrets. Reports are handled on a best-effort basis; no response or
fix SLA is promised.

## What this software is

Plenilune is a local Go binary with an optional localhost review UI. It parses
synthetic network configuration, compares modeled behavior, can build an
in-memory repair, and exports receipts that another local session can replay.
It does **not** contact a device, apply configuration, authenticate users, or
provide a hosted multi-tenant service.

The security model is a **single-user localhost trust model**:

- The demo server binds loopback only. There is no authentication, authorization,
  TLS, audit log or tenant isolation.
- Anyone who can reach the loopback port can call the local API while the server
  runs. Do not expose the port through a proxy, tunnel, port-forward or container
  publish.
- As defense in depth inside that single-user model, every `/api/*` route sits
  behind a local-request guard: the `Host` authority must be a loopback name or
  address carrying the server's listen port, a present `Origin` must be an
  http(s) loopback origin, and `Sec-Fetch-Site: cross-site` is rejected. This
  blunts DNS-rebinding and cross-site requests from other pages in the same
  browser; it is a localhost mitigation, **not** multi-user security and not a
  substitute for authentication. The server also bounds its header, read and
  write phases with timeouts, caps request bodies per route, and runs at most
  two heavy analyses (analyze/compare/repair/replay/agent session)
  concurrently — excess requests get a typed `429 SERVER_BUSY` instead of
  queueing without bound.
- Configuration files, obligations and receipts are handled in memory for the
  current session. The UI does not persist configuration or receipts in
  `localStorage`; it stores only the locale choice.
- The interface is not a substitute for a change-control system, a device
  authorization boundary or a security review of the modeled policy.

## AI key handling

- AI is off by default. A request is sent only after an explicit UI consent
  action.
- The server reads `META_MODEL_API_KEY` from its own process environment. The
  browser never receives the key, and the key is not serialized into receipts,
  session files, reports or logs.
- `scripts/start-local.ps1` shows a local WinForms box. The key stays in that
  PowerShell process and is removed when the server exits. Its **Paste** button
  reads the clipboard only when the user clicks it; Plenilune itself does not
  read the clipboard, process memory or an environment dump.
- The provider model is fixed to `muse-spark-1.3-contributor`; model override and
  fallback are rejected before a provider request.
- When AI is enabled, the selected ACL excerpt, intent, accepted conditions and
  change list are sent to the configured provider. Do not enable AI for
  configuration you are not allowed to share with that provider.
- A failed, partial, malformed, refused or timed-out provider response is never
  turned into a completed or verified engine result.

## License and third-party boundary

Plenilune's own source, including its Meta HTTP adapter and orchestration code,
is GPL-3.0-only. Third-party components retain their own licenses; see
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and `third_party/licenses/`.

Meta's model and API are a third-party service, not code distributed with
Plenilune. They are governed by Meta's terms, not by Plenilune's GPL. As
reviewed on 31 October 2026, the relevant documents were the
[Meta Model API Terms of Service](https://dev.meta.ai/legal/terms-of-service)
(page dated 9 October 2026) and the
[Meta Acceptable Use Policy](https://dev.meta.ai/legal/acceptable-use-policy).
The terms describe the API as a service and include restrictions on resale and
on using outputs to train competing models. This summary is not legal advice;
review the current terms before enabling AI. The GPL does not relicense Meta's
service, model, marks or outputs, and Plenilune is not endorsed by Meta.

## Data rules

- Use synthetic configuration, packets, obligations and receipts. Do not put real
  device configuration, credentials, API keys, packet captures, customer names or
  other personal data in the repository, screenshots, issues or test fixtures.
- The sample data uses documentation address ranges and is labelled as sample.
- Export only what the review needs. Receipts and reports can still reveal
  configuration details even when they contain no key.
- Hashes in receipts check content consistency. They are not signatures and do
  not authenticate the author, approver or build.

## Known limitations and non-goals

- No device, firmware, NAT, routing-table, TCP-state or real-connectivity
  validation. A packet witness is not an equivalence proof.
- No user authentication, authorization, encryption at rest, remote audit,
  tamper-proof ledger or signed approvals.
- No guarantee that the parser covers every vendor syntax; unsupported syntax is
  reported, not guessed.
- No security audit, penetration test, fuzzing campaign or screen-reader
  qualification has been completed.
- Live UI with Meta, real-device qualification and model-quality evaluation
  remain open; a successful mocked provider test is not live evidence.
- There is no supported release train. Only the current source tree is exercised
  by the local checks.

## Local checks before sharing

```sh
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...
cd web && npm ci && npm test && npm run build
cd ..
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/secret-scan.ps1
```

The secret scan prints `file:line [pattern]` only, never a matched value. A
non-zero exit blocks sharing. It is a targeted pattern scan, not proof that a
tree is free of secrets. It skips binary/media files; screenshots and other
assets must be reviewed separately.

## Publication gate

The owner has selected GPL-3.0-only, added [LICENSE](LICENSE)/[NOTICE](NOTICE)
and assigned the security contact. Before a remote push or deployment:

1. the owner reviews the final [public catalog](docs/public-catalog.md), public
   READMEs, walkthroughs and screenshots;
2. `scripts/public-snapshot.ps1` produces the snapshot and
   `scripts/secret-scan.ps1 -Root <snapshot>` exits 0;
3. the full local checks above pass on the release snapshot;
4. no credentials, local evidence, QA output, build output or `Sơ khai/`
   research originals are included;
5. the owner approves the publication and performs the remote action.

`docs/public-manifest.sha256` records the reviewed snapshot. This policy does
not authorize a push by itself.

See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for the dependency and
license boundary.
