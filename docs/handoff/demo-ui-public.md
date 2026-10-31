# UI/demo, offline QA and public prep — 31/10/2026

> Ghi chú lịch sử: handoff này viết trước khi license được chốt. License hiện
> hành là GPL-3.0-only; xem `LICENSE`, `NOTICE` và
> [license-public-prep](license-public-prep.md). Nội dung dưới đây giữ nguyên
> trạng thái tại thời điểm đó.

Tiếp quản từ [backend-prefix-v02](backend-prefix-v02.md) sau khi QA
backend-prefix đã đóng blocker. Không mở miền phân tích mới, không sửa
semantics/proof/core, không sửa `Sơ khai/` hoặc MoonStorm.

## Phạm vi đã làm

- Chốt hai walkthrough synthetic VI/EN:
  [ACL drift](../walkthrough-vi.md) và
  [prefix-list routing](../walkthrough-en.md).
- Tinh chỉnh phần nhìn chính của kết quả: nêu **đổi gì**, **vì sao** (quy tắc
  quyết định ở mỗi bản) và **làm gì tiếp** ngay trên phiếu; chi tiết packet,
  source, hash, budget và JSON vẫn ở phần mở rộng.
- Chuyển khối **Quyết định review** xuống sau biên bản sửa để người review đọc
  bằng chứng trước; thêm lý do khi nút nhận bị chặn.
- Giữ nguyên engine, schema, proof và API. Chỉ sửa trình bày/UI và tài liệu.
- QA UI offline trong Brave, server riêng không key; ảnh đã soát và đưa vào
  `docs/screenshots/`.
- Chuẩn bị public reviewable: README EN/VI, offline no-key guide, SECURITY,
  dependency notices, walkthroughs, public catalog và manifest. **Không** chọn
  license, không push, không deploy.

## Finding và sửa

### F1 — Phiếu kết quả chưa trả lời “vì sao” và “làm gì tiếp” ở phần đầu

- **Trước:** phiếu chỉ có headline packet + `DIFFERENT`, body chung và các con
  số điều kiện/witness. Quy tắc quyết định và bước tiếp theo nằm dưới (change
  class/action bar), nên người đọc phải cuộn mới trả lời được hai câu hỏi chính.
- **Sau:** thêm dòng **Vì sao** nêu đúng rule trước/sau (kèm dòng nguồn và text
  rule), và dòng **Tiếp theo** theo trạng thái (policy, repair, retry hoặc
  evidence). Prefix-list có dòng why theo seq và next evidence riêng.
- **File:** `web/src/components/ResultStep.tsx`,
  `web/src/components/PrefixListDetail.tsx`, `web/src/lib/labels.ts`,
  `web/src/i18n/messages.{vi,en}.ts`, `web/src/styles/components.css`.
- **Regression:** `web/tests/resultPriority.test.ts`; browser checks
  `ACL verdict plate names the deciding rules in both versions`,
  `ACL verdict plate names the next action`,
  `prefix verdict plate explains the deciding sequence entries`,
  `prefix verdict plate names the next evidence step`.
- **Evidence thất bại trước sửa:** `out/qa-ui-2026-10-31/browser-pre-fix/browser-results.json`
  (harness dừng ở `locator('.verdict-plate__why')`; ba assertion prefix/English
  cũng FAIL trên UI cũ). Sau sửa: toàn bộ PASS.

### F2 — Quyết định review đứng trước biên bản

- **Trước:** khối “Quyết định review của bạn” (gồm **Nhận để review**) nằm trên
  AI/manual builder và receipt. Người review có thể bấm nhận trước khi đọc
  trạng thái `VERIFIED`, collateral và replay.
- **Sau:** khối quyết định nằm ngay sau receipt; có câu “Đọc biên bản bên trên
  trước khi quyết định”; nếu receipt chưa `VERIFIED`/`HOLDS`/collateral
  `VERIFIED`, nút nhận bị khóa và hiện lý do. Quyết định vẫn là metadata riêng,
  không áp dụng thiết bị.
- **File:** `web/src/components/RepairStep.tsx`, i18n.
- **Regression:** browser checks
  `review decision is not offered before a receipt exists`,
  `review decision follows the receipt it belongs to`,
  `receive is enabled only by a verified, bound receipt`,
  `unverified receipt blocks receiving with a reason`.
  Đây là finding đọc code + kiểm DOM sau sửa; harness cũ không có assertion thứ
  tự DOM nên không có artifact FAIL riêng trước sửa.

### F3 — Quick start dùng binary cục bộ không được build bởi hướng dẫn

- **Trước:** `README` hướng dẫn build `bin/plenilune.exe` nhưng
  `scripts/start-local.ps1` chạy `bin/plenilune-demo-contributor.exe`, tên chỉ
  tồn tại trên máy phát triển. Một bản clone sạch sẽ không mở được UI theo
  README.
- **Sau:** script dùng `bin/plenilune-demo.exe` và báo lỗi rõ nếu chưa build;
  README VI/EN thêm lệnh build demo binary và hướng dẫn offline không key.
- **File:** `scripts/start-local.ps1`, `README.md`, `README.vi.md`.
- **Regression:** build và chạy `bin/plenilune-demo.exe` trên server QA 8094;
  toàn bộ 142 browser checks chạy qua binary này.

### F4 — Ghi chú “chọn đối tượng” sai khi đã chọn object

- **Trước:** ActionBar của result analysis-only luôn ghi “Chọn một đối tượng để
  xem bằng chứng chi tiết” kể cả khi prefix-list đã mở.
- **Sau:** khi đã chọn object, ghi bước tiếp theo (xuất hồ sơ hoặc replay).
- **File:** `web/src/components/ResultStep.tsx`, i18n.
- **Regression:** browser check `prefix verdict plate names the next evidence
  step` và ảnh `vi-prefix-1440.png`.

### F5 — Câu tiếng Anh sai số ít/số nhiều ở phần đầu

- **Trước:** “1 objects change behavior”, “1 newly denied groups”, “1 route
  prefixes”. Đây là câu người đọc thấy ngay trên phiếu tổng quan/prefix.
- **Sau:** thêm dạng số ít cho headline/unanalyzed/prefix range; đổi summary
  change class sang nhãn đếm trung tính (“Newly denied groups: 1”).
- **File:** `web/src/components/ConfigOverview.tsx`,
  `web/src/components/PrefixListDetail.tsx`, i18n.
- **Regression:** browser checks `English overview uses singular grammar for one
  changed object`, `English prefix counts use singular grammar for one route`.

## QA UI offline — kết quả

- Ngày 31/10/2026; server `127.0.0.1:8094`, không key, không gọi Meta.
- Brave/Chromium `156.0.8078.80` (exe file version `156.1.98.63`).
- **142/142 PASS**, 0 FAIL. Không có console error ngoài các lỗi 400 cố ý
  (validation/replay/tamper) và không có request ra ngoài loopback.
- Phủ: VI/EN; 1440/390/320; light/dark; axe 0 serious/critical; keyboard skip
  link, mở technical details, focus sau đổi bước, đổi ngôn ngữ bằng bàn phím.
- Kịch bản: ACL drift → policy → repair → replay → decision → evidence; prefix
  overview → exact counts → limits → evidence/replay; fresh-session import,
  tamper, timing-sensitive; unsupported, budget exhaustion, cancel, invalidation,
  undeclared scope.
- Artifact (cục bộ, không public):
  `out/qa-ui-2026-10-31/browser/` (browser-results.json, axe-report.json,
  screenshots, downloaded receipts/reports), `environment.txt`,
  `source-manifest.sha256`, `qa-run.log`.
- Evidence trước sửa: `out/qa-ui-2026-10-31/browser-pre-fix/`.
- Ảnh đã soát và copy vào `docs/screenshots/`:
  VI result/repair/evidence/prefix/mobile/dark; EN result/repair/evidence/
  conditions/prefix. Không có key, đường dẫn cá nhân hay dữ liệu thật.

### Giới hạn của QA này

- Offline UI + synthetic; không phải live UI với Meta.
- Không phải device qualification, screen reader audit hay model-quality
  assessment.
- Một packet witness vẫn chỉ chứng minh một điểm; QA không thay đổi nghĩa proof.
- 142 check là harness cục bộ, không phải coverage toàn bộ UI.

## Live UI

**Chưa chạy.** Không có server Plenilune đang chạy với key khi bắt đầu; server
QA 8094 chạy no-key. Không đọc clipboard/process memory/env, không đưa key vào
browser, không dùng receipt backend cũ thay cho UI-live.

Nếu chủ dự án muốn chạy một ca live synthetic duy nhất:

1. Build `bin/plenilune-demo.exe`.
2. Chạy `./scripts/start-local.ps1` trên một port trống và nhập key tại hộp
   setup cục bộ (key chỉ ở process đó).
3. Dùng UI, chọn mẫu SSH synthetic, bật AI bằng consent, gửi **một** yêu cầu
   `muse-spark-1.3-contributor`; không fallback/retry để săn PASS.
4. Ghi actual request/tool/usage/receipt + replay và so với caps hiện có
   (8 model requests, 12 tool calls, 2 proposals, 1200 output tokens/request,
   60 s).
5. Không dùng ca backend cũ (`out/live-contributor-*`) làm UI-live mới.

## Public readiness

Đã chuẩn bị:

- `README.md` / `README.vi.md`: hai walkthrough, offline no-key guide,
  screenshot đã soát, trạng thái live UI trung thực.
- `SECURITY.md`: mô hình localhost single-user, key handling, data rules, giới
  hạn, publication gate. **Security contact còn trống** — chủ dự án phải điền
  trước public.
- `THIRD_PARTY_NOTICES.md`: inventory 189 package từ `web/package-lock.json`,
  Go stdlib, ghi chú license; không thay thế license text.
- `docs/walkthrough-vi.md`, `docs/walkthrough-en.md`, `docs/screenshots/**`.
- `docs/public-catalog.md`: allowlist cập nhật; internal vẫn giữ `Sơ khai/`,
  `out/`, `bin/`, `web/dist`, `web/node_modules`, `web/docs/qa`, harness QA ngày
  31/10 và `web/tests/browser-qa.mjs`.
- `docs/public-manifest.sha256`: sinh từ allowlist cho snapshot review; 232 mục,
  tự kiểm 0 mismatch. Manifest không liệt kê chính nó.
- Snapshot review cục bộ: `out/public-review-<timestamp>/` (không public; đường
  dẫn lần chạy cuối ở `out/qa-ui-2026-10-31/public-snapshot.log`).
- Source manifest QA: `out/qa-ui-2026-10-31/source-manifest.sha256` (222 mục).
- Secret scan working tree: `237 scanned files; no blocking patterns` (exit 0),
  không in giá trị khớp. Snapshot: `217 scanned files; no blocking patterns`.
  Không có Git trong workspace này, nên không có staged/history scan; scan là
  working tree + snapshot allowlist.

Chưa làm (cần chủ dự án quyết định):

1. **License** — chưa chọn Apache-2.0 hay source-available; chưa có `LICENSE`.
2. **Owner/contact** — chưa có tên chủ dự án và kênh báo lỗi bảo mật.
3. **Public push/deploy** — không thực hiện; chưa có remote.
4. **Dependency license texts** — inventory đã có; cần bổ sung full text của các
   package thực sự phân phối trước public.
5. **Live UI** — chưa chạy; nếu muốn, cần key do chủ dự án nhập như trên.

## Trạng thái kỹ thuật cuối

- `gofmt` sạch; `go vet ./...`, `go test ./...`, `go test -race ./...`,
  `go build ./...` PASS.
- Frontend: 83 unit tests PASS; production build PASS.
- Browser QA: 142/142 PASS; axe serious/critical = 0.
- Không sửa engine/semantics/schema/proof; không deploy, push hay chọn license.
