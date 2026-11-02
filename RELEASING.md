# Releasing Plenilune

How a release is cut. Publication itself (tags, GitHub Releases,
pushes) remains an owner action; this document is the checklist that
precedes it.

## Versioning

- Release versions are semantic (MAJOR.MINOR.PATCH) and live in
  exactly one place: `evidence.ToolVersion` in
  `internal/evidence/comparison.go`. The CLI version command and the
  capabilities document derive from it; there is no second copy to
  update.
- Schema versions are a separate axis (P0-D1: bump a schema version
  when a field is removed or changes meaning; additive changes do not
  bump).
- A release that changes `ToolVersion` must state in its changelog
  entry which previously sealed tool versions remain replayable.
  Widening or narrowing `evidence.SupportedToolVersion` is a release
  decision and is recorded in the same change.

## Cutting a release

1. Start from a clean checkout of `main` at the intended release
   commit; the working tree must be clean (`git status` empty).
2. Prepare the release change like any other change (branch, gate,
   merge — never a direct push to `main`): set
   `evidence.ToolVersion`, add the `CHANGELOG.md` entry with the
   release date and the replay-support statement, update the
   supported-versions table in `SECURITY.md`, and regenerate the
   capabilities golden (`go test ./internal/capabilities/ -update`).
3. Pre-release verification matrix — all of it must pass on the
   release commit:
   - Full gate: gofmt clean, `go vet ./...`, `go test ./...`,
     `go test -race ./...`, `go build ./...`; the web test suite and
     build when the web tree changed.
   - Short fuzz: `scripts/fuzz.sh short` with no new crashers.
   - Offline browser QA of the demo UI on the release build.
   - Replay of every supported receipt version: the frozen replay
     tests in the suite (`testdata/receipts/` and package testdata)
     cover this — confirm they actually ran and passed; never skip
     them.
4. Regenerate the public manifest on an LF checkout
   (`scripts/public-snapshot.ps1`), run the secret scan, and merge
   the manifest PR. The manifest must match the release commit's
   tree.
5. Build the release binaries from the clean checkout of the tagged
   commit, so Go embeds the VCS stamp:
   `go build -trimpath -o plenilune ./cmd/plenilune` and
   `go build -trimpath -o plenilune-demo ./cmd/plenilune-demo`.
   Verify `plenilune version` reports the release version, the
   tagged commit and `modified=false`.
6. Tag `vX.Y.Z` on the release commit and create the GitHub Release
   (owner action): attach the Windows and Linux binaries plus a
   `SHA256SUMS` file, and use the changelog entry as the release
   notes. An Ed25519 detached signature over the sums may be
   attached, following the signing design recorded for P2-5.
7. Smoke-test the published artifacts from a fresh directory:
   version output, one synthetic compare, and replay of one bundled
   receipt.
