# GPL-3.0-only và chuẩn bị public — 31/10/2026

Trạng thái: license, copyright, security contact và ranh giới Meta đã hoàn tất;
public snapshot đã dựng, quét và được chủ dự án phê duyệt tạo repository public
`vmcchooky/plenilune` + push ngày 31/10/2026. Không deploy, tag hay release.
Không đổi hành vi engine/UI; chỉ metadata license, không gọi Meta, không sửa
`Sơ khai/` hoặc MoonStorm.

## File đã thay đổi

- `LICENSE`: nguyên văn GPLv3 từ nguồn GNU chính thức, 35.149 byte, SHA-256
  `3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986`.
- `NOTICE`: copyright Võ Mạnh Cường (Chooky), thương hiệu Quorix Việt Nam,
  SPDX `GPL-3.0-only`, ghi chú thương mại/tuân thủ, không có tùy chọn "or
  later", ranh giới third-party/Meta/trademark.
- `third_party/licenses/{react,react-dom,scheduler,go}/LICENSE` và `README.md`:
  bản sao nguyên văn. React family MIT (19.3.1/19.3.1/0.28.0); Go 1.26.7
  BSD-3-Clause. Dev-only packages chỉ nằm trong inventory, không được bundle.
- `README.md`, `README.vi.md`: GPL-3.0-only, giải thích GPL cho phép thương mại
  và không cấm cạnh tranh, nghĩa vụ khi phân phối bản phái sinh, ranh giới
  third-party/Meta, security contact. Sửa khoảng trắng "Backend update".
- `SECURITY.md`: trạng thái active, contact `chooky@quorix.io.vn`, phạm vi
  supported version, Meta terms boundary, publication gate mới.
- `THIRD_PARTY_NOTICES.md`: tách runtime/build-test, license text React/Go,
  kiểm tra font/icon/ảnh, Meta service terms, mục chưa xác minh.
- `docs/public-catalog.md`: gate/license/publish list cập nhật; bỏ dòng sai
  coi `docs/research` là internal.
- `ROADMAP.md`, `ARCHITECTURE.md`, `AGENTS.md`: chỉ đồng bộ trạng thái license.
- `AI_AGENT_DESIGN.md`: thêm ranh giới license với Meta.
- `web/package.json`: `"license": "GPL-3.0-only"` và author. `web/index.html`:
  comment SPDX, meta license/author.
- `scripts/public-snapshot.ps1`: thêm `LICENSE`, `NOTICE`,
  `third_party/licenses` vào allowlist.
- `docs/public-manifest.sha256`: tái tạo từ snapshot; manifest không tự liệt kê.

## Kết quả kiểm tra

- `cmp` xác nhận `LICENSE` giống byte bản tải từ
  `https://www.gnu.org/licenses/gpl-3.0.txt`; copyright FSF trong văn bản không
  bị sửa.
- Hash của các bản license third-party khớp `web/node_modules/*` và `GOROOT`.
- `gofmt -l .` sạch; `go vet ./...`, `go test ./...`, `go test -race ./...`,
  `go build ./...` PASS ngày 31/10.
- Frontend: `npm test` 83/83 PASS; `npm run build` PASS.
- Kiểm link Markdown: 134 link nội bộ trong 33 file đều resolve; snapshot cũng
  không còn link tới brief/QA evidence bị loại khỏi allowlist.
- Secret scan trên cây làm việc và trên snapshot: không có blocking pattern;
  scan chỉ in `file:line [pattern]`, không in giá trị. Binary/media được bỏ qua
  nên đã soát thủ công toàn bộ 16 ảnh screenshot: dữ liệu synthetic, dải địa chỉ
  tài liệu, không thấy đường dẫn máy hay dữ liệu cá nhân. PNG không có chunk
  text/EXIF/time.
- Workspace không có `.git`, nên không có staged hay history để quét.
- Snapshot dưới `out/` (ignored) chứa `LICENSE`, `NOTICE`, `third_party/licenses`
  và không chứa `web/dist`, `web/node_modules`, `bin/`, `out/`, `Sơ khai/`,
  QA evidence hay internal briefs. Manifest cuối có 240 entry (241 file gồm chính
  file manifest); mọi hash khớp và không có file thừa/thiếu.

## Thành phần bên thứ ba

- Runtime UI: React, React DOM, Scheduler — MIT; Go standard library — BSD-3-Clause.
- Build/test-only: Vite, `@vitejs/plugin-react`, esbuild, Babel, TypeScript,
  Vitest, jsdom, Playwright (Apache-2.0), axe-core (MPL-2.0), v.v. Không nằm
  trong bundle runtime và không được phân phối kèm snapshot.
- Font: chỉ dùng system font stack, không có file font. Icon/favicon là SVG
  trong repo; ảnh là screenshot của UI. Không xác định được asset bên thứ ba;
  provenance chỉ dựa trên kiểm tra repo, chưa audit độc lập.
- Meta: adapter trong repo là mã GPL của Plenilune. Model/API/service/output của
  Meta là của Meta, theo Meta Model API Terms (bản 9/10/2026) và Acceptable Use
  Policy; không được cấp phép lại theo GPL. Không có Meta SDK/weights trong repo.
- Tài liệu Cisco chỉ được tham chiếu bằng URL; không chép văn bản vendor.

## Điểm còn thiếu và bước public tiếp theo

- Chủ dự án đã approve public repo và push; snapshot này là nguồn publish. Việc
  push được thực hiện từ checkout xuất bản riêng; workspace gốc vẫn không có
  `.git`.
- Sau push: xác minh repo public, commit/tree, README/ảnh/license và bật private
  vulnerability reporting nếu khả dụng; giữ email bảo mật `chooky@quorix.io.vn`.
- Chốt version/tag/release và gắn `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`,
  `third_party/licenses/` cùng manifest khi phát hành.
- Live Meta/UI, screen-reader và real-device QA vẫn là qualification riêng.
- Nếu phân phối binary/UI build, phải kèm third-party license texts và
  corresponding source theo GPL-3.0-only; nếu asset thay đổi provenance, cập
  nhật notices trước khi public.
- Các handoff cũ vẫn có thể mô tả trạng thái trước license; coi file này,
  README và SECURITY là trạng thái hiện hành.
