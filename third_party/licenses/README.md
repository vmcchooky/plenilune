# Bundled third-party license texts

These files are verbatim copies of the upstream license texts for components
that can be included in a Plenilune build. They are not relicensed by
Plenilune. Each upstream license governs its component.

| Directory | Component | Version | License | Copy source |
|---|---|---|---|---|
| `react/` | React | 19.3.1 | MIT | `web/node_modules/react/LICENSE` |
| `react-dom/` | React DOM | 19.3.1 | MIT | `web/node_modules/react-dom/LICENSE` |
| `scheduler/` | Scheduler (React DOM dependency) | 0.28.0 | MIT | `web/node_modules/scheduler/LICENSE` |
| `go/` | Go standard library/toolchain | 1.26.7 | BSD-3-Clause | `GOROOT/LICENSE` |

## Scope

- **React, React DOM, Scheduler** are included in a production UI build under
  `web/dist`. The public source snapshot does not contain `web/dist`; ship these
  texts with any built UI you distribute or serve.
- **Go** is statically linked into Go binaries. This repository does not include
  Go source; ship the Go license text with any binary you distribute.
- Other locked npm packages are build/test-only or are not present in the
  runtime bundle. See [THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md).

The files are byte-for-byte copies from the installed packages/toolchain. Do
not edit them. When a version changes, refresh the copy, update this table and
rerun the checks in THIRD_PARTY_NOTICES.md.
