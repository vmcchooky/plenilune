# Plenilune

Read README.md for implemented status, CONTRACTS.md for wire contracts, and
ARCHITECTURE.md for package ownership. Sơ khai/ is preserved research input;
edit the working documents at root, never those originals. MoonStorm is outside
this workspace's implementation scope.

- Build small complete paths; update existing owner docs when contracts change.
- Go standard library core. Parser/verifier have no network, device or file I/O.
- AI proposes conditions and in-memory edits; deterministic checks own verdicts.
- A packet trace is not an equivalence proof. Unsupported/budget exhaustion never
  becomes EQUIVALENT or HOLDS. Intent translation is not proof of intent fidelity.
- Hash accepted predicate definitions, scope and results together in future full
  evidence. AI provenance and elapsed time are separate metadata.
- Use synthetic inputs. No credentials/configuration from real devices in Git.
- Keep algorithms covered by independent reduced-domain checks; review changes to
  semantics/evidence independently. Do not weaken assertions to pass.
- Check gofmt, go vet ./..., go test ./..., go test -race ./..., go build ./....
- No hosted workflow, publication or device access is needed for local
  development; the license is recorded in LICENSE (GPL-3.0-only). Do not copy proprietary MoonStorm source.
