# Public repository catalog

Status: **owner-approved public snapshot** (31 October 2026). The owner
approved creating the public repository `vmcchooky/plenilune` and pushing this
snapshot under GPL-3.0-only. This catalog defines the snapshot; publication is
performed from a separate publish checkout created from it.

## Publication gate

1. **License:** GPL-3.0-only selected; [LICENSE](../LICENSE) contains the
   verbatim GPLv3 text and [NOTICE](../NOTICE) records the project copyright and
   license boundary. Done 31 October 2026.
2. **Security contact:** `chooky@quorix.io.vn` is assigned in
   [SECURITY.md](../SECURITY.md). Done 31 October 2026.
3. **Owner review:** the owner authorized the public repository and push on
   31 October 2026. Done.
4. **Secret scan:** `scripts/secret-scan.ps1` exits 0 on the tree and on the
   public snapshot. Binary/media files are skipped and must be reviewed
   separately.
5. **Local checks:** `gofmt`, `go vet`, Go tests, race tests, `go build`, frontend
   tests and the frontend build pass on the release snapshot.
6. **Snapshot:** `scripts/public-snapshot.ps1` builds the review copy under
   ignored `out/`, updates `docs/public-manifest.sha256`, and the snapshot passes
   the secret scan. No generated/local evidence, personal paths, credentials or
   device data are staged.
7. **Dependency/license review:** [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)
   and [third_party/licenses/](../third_party/licenses/) cover the runtime
   components that can be distributed. Dev-only packages remain under their own
   licenses.

Publication uses a separate Git checkout created from this snapshot. No
workflow or Actions are added; no release, tag or deployment is part of the
publication step.

## Publish

| Path | Notes |
|---|---|
| `README.md`, `README.vi.md` | Public entry points; license and security contact are explicit |
| `LICENSE` | Verbatim GNU GPLv3 text; project copyright is recorded separately in `NOTICE` |
| `NOTICE` | Project copyright, GPL-3.0-only boundary and trademark/service disclaimers |
| `SECURITY.md` | Local security model and private reporting contact |
| `THIRD_PARTY_NOTICES.md` | Dependency inventory, runtime/development split and Meta service boundary |
| `third_party/licenses/**` | Verbatim license texts for distributed runtime components (React family, Go) |
| `AGENTS.md`, `ARCHITECTURE.md`, `CONTRACTS.md` | Working contracts and ownership |
| `VERIFICATION_DESIGN.md`, `AI_AGENT_DESIGN.md` | Proof scope and AI boundary |
| `ENGINEERING_GUARDRAILS.md`, `ROADMAP.md`, `REVIEW_NOTES.md` | Change discipline and history |
| `.env.example`, `.gitignore` | Empty example and ignore rules |
| `go.mod` | Standard-library-only Go module |
| `cmd/**`, `internal/**` | Production code and tests |
| `testdata/**` | Synthetic ACL, packet and obligation fixtures |
| `scripts/demo.ps1`, `scripts/demo-exact.ps1`, `scripts/demo-repair.ps1` | Offline demos |
| `scripts/start-local.ps1` | Local UI launcher; key stays in process memory |
| `scripts/secret-scan.ps1` | Local publication gate |
| `scripts/public-snapshot.ps1` | Builds the review snapshot under ignored `out/` and regenerates the manifest |
| `web/src/**`, `web/tests/**` except the dated/historical browser harnesses | UI source and product tests |
| `web/index.html`, `web/package.json`, `web/package-lock.json` | UI entry/build metadata |
| `web/.gitignore` | Local UI build/dependency ignore rules |
| `web/tsconfig.json`, `web/vite.config.ts`, `web/vitest.config.ts` | UI configuration |
| `web/public/**` | Static public assets |
| `docs/design/product-ux.md` | Product design reference |
| `docs/semantics/**` | Declared prefix-list semantics profile |
| `docs/handoff/**` except the internal prompt/QA briefs | Implementation notes/history; review claims before publishing |
| `docs/research/**` | Measured engine data and the short research note |
| `docs/walkthrough-vi.md`, `docs/walkthrough-en.md` | Two synthetic offline walkthroughs with replay commands |
| `docs/screenshots/**` | Reviewed synthetic UI screenshots; no personal data or local paths |
| `examples/moonstorm-client/**` | Independent subprocess JSON client (std-lib only) |
| `docs/usage-vi.md` | Vietnamese walkthrough of the configuration-level flow |
| `docs/public-catalog.md` | This catalog |
| `docs/public-manifest.sha256` | SHA-256 inventory of the files above, generated for the review snapshot; the manifest file itself is not listed |

## Internal — do not publish

| Path | Why |
|---|---|
| `Sơ khai/**` | Preserved research originals; not working copy/public material |
| `out/**` | Local runs, receipts, recorded sessions and demo output |
| `bin/**` | Local binaries and build products |
| `web/node_modules/**` | Installed dependencies |
| `web/dist/**` | Generated production build |
| `web/tsconfig.tsbuildinfo` | Generated TypeScript state |
| `web/docs/qa/**` | QA evidence, screenshots, harnesses and reports |
| `web/tests/qa-ui-2026-10-31.mjs` | Dated browser QA harness tied to the local Brave/server layout |
| `docs/handoff/PROMPT_*.vi.md`, `docs/handoff/QA_NEXT.vi.md` | Internal agent/QA briefs; not product documentation |
| `web/tests/browser-qa.mjs` | Historical QA harness tied to an older UI snapshot |
| `.env`, `.env.*` except `.env.example` | Local secrets/configuration |
| `*.log`, `test-results/`, `playwright-report/`, `traces/`, `coverage.out` | Local output |

Handoff notes under `docs/handoff/` are technical history, not a live guarantee.
Review statements that reference local runs or prior QA before publishing them.

## Secret scan

From the repository root:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/secret-scan.ps1
```

The scan excludes the internal paths above, binary/media files and installed
dependencies. It checks for provider key formats, private-key headers, bearer
tokens, non-empty `META_MODEL_API_KEY` assignments, generic secret assignments,
and local Windows user paths. It prints `file:line [pattern]` only, never a
matched value. A non-zero exit blocks publication.

A dedicated `.env` file is reported as a blocking finding even if its contents
are not printed. Because binary and media files are skipped, review screenshots
and assets manually for personal data, local paths and real network information.

`docs/research/*.json` is intentionally published measured data; it contains
synthetic measurements, not customer data.

## License

Plenilune is licensed **GPL-3.0-only**. [LICENSE](../LICENSE) is the verbatim
GNU GPLv3 text; [NOTICE](../NOTICE) records the project copyright (Võ Mạnh Cường
(Chooky), Quorix Việt Nam brand) and the license boundary. The GPL permits
commercial use and does not prohibit competition; distribution of a derivative
must comply with GPL-3.0, including corresponding source and notices.

The GPL covers Plenilune's own source only. Third-party components retain their
licenses; see [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) and
`third_party/licenses/`. Meta's model and API are a service under Meta's terms,
not code licensed under Plenilune's GPL.
