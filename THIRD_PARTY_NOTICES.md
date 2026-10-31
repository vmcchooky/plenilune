# Third-party notices

Status: **prepared for the public source snapshot of 31 October 2026**. This is
an inventory and attribution aid, not legal advice. Each upstream license
governs its component; the lockfile is the authority for exact npm versions.

## License boundary

Plenilune's own source is GPL-3.0-only; see [LICENSE](LICENSE) and
[NOTICE](NOTICE). Third-party components retain their own copyrights and
licenses. Nothing in Plenilune's license or this inventory relicenses a
third-party component. Meta's model and API are a separate service, not code
distributed with Plenilune; see below.

## What the public snapshot distributes

The curated public snapshot contains Plenilune source, documentation, synthetic
fixtures and screenshots. It does **not** contain `web/dist`, `bin/`, installed
`node_modules`, local QA evidence or `Sơ khai/` research originals. A production
UI build and a Go binary do contain third-party runtime code; if you distribute
either, ship the matching texts in `third_party/licenses/` and comply with the
upstream licenses.

## Runtime components in a production UI build

`web/src` imports only `react` and `react-dom/client`; React DOM depends on
`scheduler`. No other runtime package is imported by the UI source. Vite,
esbuild, Babel, TypeScript and the test stack are build/test tools and are not
part of the browser bundle.

| Component | Version | License | Copyright | Full text |
|---|---|---|---|---|
| `react` | 19.3.1 | MIT | Meta Platforms, Inc. and affiliates | `third_party/licenses/react/LICENSE` |
| `react-dom` | 19.3.1 | MIT | Meta Platforms, Inc. and affiliates | `third_party/licenses/react-dom/LICENSE` |
| `scheduler` | 0.28.0 | MIT | Meta Platforms, Inc. and affiliates | `third_party/licenses/scheduler/LICENSE` |

The three MIT files are byte-for-byte copies from the installed packages. A
build that is distributed or served must include those notices (or the
equivalent upstream license text) together with Plenilune's LICENSE/NOTICE.

## Go binaries

The Go module uses only the Go standard library; there are no third-party Go
modules in `go.mod`. The Go standard library is statically linked into a Go
binary and is distributed under the Go project's BSD-3-Clause license
(copyright The Go Authors). A verbatim copy from Go 1.26.7 is at
`third_party/licenses/go/LICENSE`. This repository does not vendor Go source.
If you distribute a compiled binary, include that license text and the complete
corresponding Plenilune source under GPL-3.0-only.

## Build and test tools (not distributed)

These are direct declarations from `web/package.json`. Versions below are
resolved from `web/package-lock.json`.

| Package | Locked version | Declared license | Scope |
|---|---|---|---|
| `@vitejs/plugin-react` | 4.7.0 | MIT | Declared dependency; build tool |
| `react` | 19.3.1 | MIT | Runtime |
| `react-dom` | 19.3.1 | MIT | Runtime |
| `vite` | 7.3.7 | MIT | Declared dependency; build tool |
| `@axe-core/playwright` | 4.14.0 | MPL-2.0 | Dev/test |
| `@testing-library/react` | 16.3.3 | MIT | Dev/test |
| `@types/react` | 19.3.0 | MIT | Dev/test |
| `@types/react-dom` | 19.3.0 | MIT | Dev/test |
| `jsdom` | 29.1.1 | MIT | Dev/test |
| `playwright` | 1.64.1 | Apache-2.0 | Dev/test |
| `typescript` | 5.9.3 | Apache-2.0 | Dev/test |
| `vitest` | 5.0.5 | MIT | Dev/test |

`@vitejs/plugin-react` and `vite` are declared under `dependencies` for the
current demo but are build-time tools; only `react` and `react-dom` (plus
Scheduler) are runtime libraries. `@axe-core/playwright` is MPL-2.0 and
`playwright` is Apache-2.0; both are development/test dependencies. Full texts
are not copied for dev-only packages because they are not distributed with the
product. If you redistribute the test harness or `node_modules`, comply with
each package's own license.

## Assets

- **Fonts:** the UI uses system font stacks only (for example Iowan Old Style,
  Palatino, Aptos, Segoe UI, Cascadia Code). No font files are bundled or
  distributed, so no font license text is included.
- **Icons:** icons are inline SVG paths in `web/src/components/Icon.tsx`; no
  third-party icon library is imported. Provenance beyond repository inspection
  has not been independently verified; if an icon was adapted from an external
  set, add that set's license before distribution.
- **Favicon:** `web/public/favicon.svg` is an SVG asset in this repository; no
  external asset source was identified.
- **Images:** `docs/screenshots/*.png` are captures of the Plenilune UI using
  synthetic data. No third-party photographs or illustrations were identified.
  Image metadata was checked for text chunks; no local paths or personal data
  were found. Visual review is required again if screenshots are replaced.
- **Vendor documentation:** Cisco/IOS references in `docs/semantics/` are links
  used to describe an independent model. No Cisco document text is bundled.

## Meta model and API (service, not distributed)

Plenilune contains an optional HTTP adapter for the Meta Model API. It does not
include, distribute, sublicense or relicense any Meta model, weights, API, SDK,
service or output. `muse-spark-1.3-contributor` is a fixed service model
identifier, not a license grant. The adapter code is Plenilune source under
GPL-3.0-only; Meta's service remains under Meta's terms.

As reviewed on 31 October 2026:

- [Meta Model API Terms of Service](https://dev.meta.ai/legal/terms-of-service)
  (page dated 9 October 2026) describes the API as a service, prohibits resale
  of the service on a standalone basis, and restricts using outputs to train
  competing models.
- [Meta Acceptable Use Policy](https://dev.meta.ai/legal/acceptable-use-policy)
  contains security, high-risk and end-user restrictions.

Those are service terms, not additional restrictions on Plenilune's source.
This summary is not legal advice and the terms may change; review the current
documents before enabling AI. Do not represent Meta as endorsing Plenilune, and
do not describe Meta's model or service as licensed under Plenilune's GPL.

## All locked npm packages

The build and test tree contains the following packages. Many are build- or
test-only and are not part of the distributed UI bundle. The lockfile is the
authority for exact versions and integrity hashes. The table was checked against
`web/package-lock.json` (189 entries, including the nested
`jsdom/node_modules/lru-cache`) on 31 October 2026.

| Package | Version | Declared license |
|---|---|---|
| `@asamuzakjp/css-color` | 5.1.11 | MIT |
| `@asamuzakjp/dom-selector` | 7.1.1 | MIT |
| `@asamuzakjp/generational-cache` | 1.0.1 | MIT |
| `@asamuzakjp/nwsapi` | 2.3.9 | MIT |
| `@axe-core/playwright` | 4.14.0 | MPL-2.0 |
| `@babel/code-frame` | 7.29.7 | MIT |
| `@babel/compat-data` | 7.29.7 | MIT |
| `@babel/core` | 7.29.7 | MIT |
| `@babel/generator` | 7.29.12 | MIT |
| `@babel/helper-compilation-targets` | 7.29.7 | MIT |
| `@babel/helper-globals` | 7.29.7 | MIT |
| `@babel/helper-module-imports` | 7.29.7 | MIT |
| `@babel/helper-module-transforms` | 7.29.7 | MIT |
| `@babel/helper-plugin-utils` | 7.29.7 | MIT |
| `@babel/helper-string-parser` | 7.29.11 | MIT |
| `@babel/helper-validator-identifier` | 7.29.7 | MIT |
| `@babel/helper-validator-option` | 7.29.7 | MIT |
| `@babel/helpers` | 7.29.10 | MIT |
| `@babel/parser` | 7.29.12 | MIT |
| `@babel/plugin-transform-react-jsx-self` | 7.29.7 | MIT |
| `@babel/plugin-transform-react-jsx-source` | 7.29.7 | MIT |
| `@babel/runtime` | 7.29.10 | MIT |
| `@babel/template` | 7.29.7 | MIT |
| `@babel/traverse` | 7.29.12 | MIT |
| `@babel/types` | 7.29.11 | MIT |
| `@bramus/specificity` | 2.4.3 | MIT |
| `@csstools/color-helpers` | 6.1.2 | MIT-0 |
| `@csstools/css-calc` | 3.4.3 | MIT |
| `@csstools/css-color-parser` | 4.2.7 | MIT |
| `@csstools/css-parser-algorithms` | 4.0.2 | MIT |
| `@csstools/css-syntax-patches-for-csstree` | 1.1.16 | MIT-0 |
| `@csstools/css-tokenizer` | 4.0.2 | MIT |
| `@esbuild/aix-ppc64` | 0.28.2 | MIT |
| `@esbuild/android-arm` | 0.28.2 | MIT |
| `@esbuild/android-arm64` | 0.28.2 | MIT |
| `@esbuild/android-x64` | 0.28.2 | MIT |
| `@esbuild/darwin-arm64` | 0.28.2 | MIT |
| `@esbuild/darwin-x64` | 0.28.2 | MIT |
| `@esbuild/freebsd-arm64` | 0.28.2 | MIT |
| `@esbuild/freebsd-x64` | 0.28.2 | MIT |
| `@esbuild/linux-arm` | 0.28.2 | MIT |
| `@esbuild/linux-arm64` | 0.28.2 | MIT |
| `@esbuild/linux-ia32` | 0.28.2 | MIT |
| `@esbuild/linux-loong64` | 0.28.2 | MIT |
| `@esbuild/linux-mips64el` | 0.28.2 | MIT |
| `@esbuild/linux-ppc64` | 0.28.2 | MIT |
| `@esbuild/linux-riscv64` | 0.28.2 | MIT |
| `@esbuild/linux-s390x` | 0.28.2 | MIT |
| `@esbuild/linux-x64` | 0.28.2 | MIT |
| `@esbuild/netbsd-arm64` | 0.28.2 | MIT |
| `@esbuild/netbsd-x64` | 0.28.2 | MIT |
| `@esbuild/openbsd-arm64` | 0.28.2 | MIT |
| `@esbuild/openbsd-x64` | 0.28.2 | MIT |
| `@esbuild/openharmony-arm64` | 0.28.2 | MIT |
| `@esbuild/sunos-x64` | 0.28.2 | MIT |
| `@esbuild/win32-arm64` | 0.28.2 | MIT |
| `@esbuild/win32-ia32` | 0.28.2 | MIT |
| `@esbuild/win32-x64` | 0.28.2 | MIT |
| `@exodus/bytes` | 1.16.0 | MIT |
| `@jridgewell/gen-mapping` | 0.3.13 | MIT |
| `@jridgewell/remapping` | 2.3.5 | MIT |
| `@jridgewell/resolve-uri` | 3.1.2 | MIT |
| `@jridgewell/sourcemap-codec` | 1.6.0 | MIT |
| `@jridgewell/trace-mapping` | 0.3.31 | MIT |
| `@napi-rs/lzma-linux-x64-gnu` | 1.5.1 | MIT |
| `@rolldown/pluginutils` | 1.0.0-beta.27 | MIT |
| `@rollup/rollup-android-arm-eabi` | 4.66.0 | MIT |
| `@rollup/rollup-android-arm64` | 4.66.0 | MIT |
| `@rollup/rollup-darwin-arm64` | 4.66.0 | MIT |
| `@rollup/rollup-darwin-x64` | 4.66.0 | MIT |
| `@rollup/rollup-freebsd-arm64` | 4.66.0 | MIT |
| `@rollup/rollup-freebsd-x64` | 4.66.0 | MIT |
| `@rollup/rollup-linux-arm-gnueabihf` | 4.66.0 | MIT |
| `@rollup/rollup-linux-arm-musleabihf` | 4.66.0 | MIT |
| `@rollup/rollup-linux-arm64-gnu` | 4.66.0 | MIT |
| `@rollup/rollup-linux-arm64-musl` | 4.66.0 | MIT |
| `@rollup/rollup-linux-loong64-gnu` | 4.66.0 | MIT |
| `@rollup/rollup-linux-loong64-musl` | 4.66.0 | MIT |
| `@rollup/rollup-linux-ppc64-gnu` | 4.66.0 | MIT |
| `@rollup/rollup-linux-ppc64-musl` | 4.66.0 | MIT |
| `@rollup/rollup-linux-riscv64-gnu` | 4.66.0 | MIT |
| `@rollup/rollup-linux-riscv64-musl` | 4.66.0 | MIT |
| `@rollup/rollup-linux-s390x-gnu` | 4.66.0 | MIT |
| `@rollup/rollup-linux-x64-gnu` | 4.66.0 | MIT |
| `@rollup/rollup-linux-x64-musl` | 4.66.0 | MIT |
| `@rollup/rollup-openbsd-x64` | 4.66.0 | MIT |
| `@rollup/rollup-openharmony-arm64` | 4.66.0 | MIT |
| `@rollup/rollup-win32-arm64-msvc` | 4.66.0 | MIT |
| `@rollup/rollup-win32-ia32-msvc` | 4.66.0 | MIT |
| `@rollup/rollup-win32-x64-gnu` | 4.66.0 | MIT |
| `@rollup/rollup-win32-x64-msvc` | 4.66.0 | MIT |
| `@testing-library/dom` | 10.4.2 | MIT |
| `@testing-library/react` | 16.3.3 | MIT |
| `@types/aria-query` | 5.0.4 | MIT |
| `@types/babel__core` | 7.20.5 | MIT |
| `@types/babel__generator` | 7.27.0 | MIT |
| `@types/babel__template` | 7.4.4 | MIT |
| `@types/babel__traverse` | 7.28.0 | MIT |
| `@types/chai` | 5.2.3 | MIT |
| `@types/deep-eql` | 4.0.2 | MIT |
| `@types/estree` | 1.0.9 | MIT |
| `@types/react` | 19.3.0 | MIT |
| `@types/react-dom` | 19.3.0 | MIT |
| `@vitejs/plugin-react` | 4.7.0 | MIT |
| `@vitest/mocker` | 5.0.5 | MIT |
| `@vitest/spy` | 5.0.5 | MIT |
| `ansi-regex` | 5.0.1 | MIT |
| `ansi-styles` | 5.2.0 | MIT |
| `aria-query` | 5.3.0 | Apache-2.0 |
| `assertion-error` | 2.0.1 | MIT |
| `axe-core` | 4.14.0 | MPL-2.0 |
| `baseline-browser-mapping` | 2.11.34 | Apache-2.0 |
| `bidi-js` | 1.1.0 | MIT |
| `browserslist` | 4.29.5 | MIT |
| `caniuse-lite` | 1.0.30001820 | CC-BY-4.0 |
| `chai` | 6.3.0 | MIT |
| `convert-source-map` | 2.0.0 | MIT |
| `css-tree` | 3.2.1 | MIT |
| `csstype` | 3.2.3 | MIT |
| `data-urls` | 7.0.0 | MIT |
| `debug` | 4.4.3 | MIT |
| `decimal.js` | 10.6.0 | MIT |
| `dequal` | 2.0.3 | MIT |
| `dom-accessibility-api` | 0.5.16 | MIT |
| `electron-to-chromium` | 1.5.468 | ISC |
| `entities` | 8.1.0 | BSD-2-Clause |
| `es-module-lexer` | 2.3.2 | MIT |
| `esbuild` | 0.28.2 | MIT |
| `escalade` | 3.2.0 | MIT |
| `estree-walker` | 3.0.3 | MIT |
| `expect-type` | 1.4.0 | Apache-2.0 |
| `fdir` | 6.5.0 | MIT |
| `fsevents` | 2.3.3 | MIT |
| `gensync` | 1.0.0-beta.2 | MIT |
| `html-encoding-sniffer` | 6.0.0 | MIT |
| `is-potential-custom-element-name` | 1.0.1 | MIT |
| `js-tokens` | 4.0.0 | MIT |
| `jsdom` | 29.1.1 | MIT |
| `jsdom/node_modules/lru-cache` | 11.5.3 | BlueOak-1.0.0 |
| `jsesc` | 3.1.0 | MIT |
| `json5` | 2.2.3 | MIT |
| `lru-cache` | 5.1.1 | ISC |
| `lz-string` | 1.5.0 | MIT |
| `magic-string` | 1.4.5 | MIT |
| `mdn-data` | 2.27.1 | CC0-1.0 |
| `ms` | 2.1.3 | MIT |
| `nanoid` | 3.3.20 | MIT |
| `node-releases` | 2.0.61 | MIT |
| `obug` | 3.0.0 | MIT |
| `parse5` | 8.0.1 | MIT |
| `picocolors` | 1.1.1 | ISC |
| `picomatch` | 4.0.7 | MIT |
| `playwright` | 1.64.1 | Apache-2.0 |
| `playwright-core` | 1.64.1 | Apache-2.0 |
| `postcss` | 8.5.32 | MIT |
| `pretty-format` | 27.5.1 | MIT |
| `punycode` | 2.3.1 | MIT |
| `react` | 19.3.1 | MIT |
| `react-dom` | 19.3.1 | MIT |
| `react-is` | 17.0.2 | MIT |
| `react-refresh` | 0.17.0 | MIT |
| `require-from-string` | 2.0.2 | MIT |
| `rollup` | 4.66.0 | MIT |
| `saxes` | 6.0.0 | ISC |
| `scheduler` | 0.28.0 | MIT |
| `semver` | 6.3.1 | ISC |
| `source-map-js` | 1.2.2 | BSD-3-Clause |
| `std-env` | 4.3.0 | MIT |
| `symbol-tree` | 3.2.4 | MIT |
| `tinybench` | 6.2.2 | MIT |
| `tinyexec` | 1.3.1 | MIT |
| `tinyglobby` | 0.2.17 | MIT |
| `tldts` | 7.4.19 | MIT |
| `tldts-core` | 7.4.20 | MIT |
| `tough-cookie` | 6.0.2 | BSD-3-Clause |
| `tr46` | 6.0.0 | MIT |
| `typescript` | 5.9.3 | Apache-2.0 |
| `undici` | 7.30.1 | MIT |
| `update-browserslist-db` | 1.3.4 | MIT |
| `vite` | 7.3.7 | MIT |
| `vitest` | 5.0.5 | MIT |
| `w3c-xmlserializer` | 5.0.0 | MIT |
| `webidl-conversions` | 8.0.1 | BSD-2-Clause |
| `whatwg-mimetype` | 5.0.0 | MIT |
| `whatwg-url` | 16.0.1 | MIT |
| `why-is-node-running` | 3.2.1 | MIT |
| `xml-name-validator` | 5.0.0 | Apache-2.0 |
| `xmlchars` | 2.2.0 | MIT |
| `yallist` | 3.1.1 | ISC |

## Inventory notes

- The npm table is an inventory, not a distribution list. Platform-specific
  optional binaries (esbuild, rollup, lzma) may not be installed on every
  machine and are not part of the browser bundle.
- Declared licenses come from package metadata in the lockfile. They were not
  independently audited against every upstream LICENSE file. Full texts were
  copied only for components that can be distributed at runtime (React family
  and Go).
- `caniuse-lite` is CC-BY-4.0 and `mdn-data` is CC0-1.0; they are build-time
  data, not runtime code. If you redistribute `node_modules` or build metadata,
  check their attribution requirements separately.
- No third-party source is copied into Plenilune's own source tree, and no
  MoonStorm source is included. The independent subprocess client under
  `examples/moonstorm-client/` is Plenilune source.
- This inventory was checked against the lockfile on 31 October 2026. Re-run the
  inventory after any dependency change.

## Unverified or open items

- Asset provenance is based on repository inspection only. No external icon,
  font or image license file was found because none is bundled; if an asset was
  adapted, add its license before distribution.
- The production build was inspected locally; no runtime package other than
  React/React DOM/Scheduler was identified. A dependency or build-config change
  requires a fresh bundle check.
- The Meta terms summary is a review note, not legal advice, and Meta may update
  the terms. The project does not claim any right to Meta's model, service,
  marks or outputs.
- Dev/test packages are listed but their full license texts are not bundled.
  They are not distributed with the product; redistribute them only with the
  corresponding upstream notices.
- This is not a legal review of every dependency combination. The project owner
  remains responsible for the final publication decision.

## Verification

```sh
cd web
npm ci
npm ls --all
npm run build
```

Check that `web/src` still imports only the runtime packages listed above, and
that `web/dist` exists only as an ignored build artifact. If shipping a build,
include `LICENSE`, `NOTICE`, this file and
`third_party/licenses/` with it. See [SECURITY.md](SECURITY.md) for the local
security model and [docs/public-catalog.md](docs/public-catalog.md) for the
publication gate.
