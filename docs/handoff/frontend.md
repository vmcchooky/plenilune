# Frontend VI/EN và runtime wiring — handoff

> Historical snapshot (30/10). Current model guard/transport:
> [contributor-runtime](contributor-runtime.md); current UI QA:
> [demo-ui-public](demo-ui-public.md). The model is fixed to
> `muse-spark-1.3-contributor`.

Ngày 30/10/2026. Phạm vi: `web/`, `internal/demohttp/session.go` (model guard),
`docs/design/product-ux.md` và tài liệu chủ sở hữu. Không sửa core semantics,
`Sơ khai/` hay MoonStorm.

## Trạng thái

Frontend được dựng lại thành workspace 5 bước, mặc định tiếng Việt, có chuyển
VI/EN và gọi API runtime hiện hữu. Comparison, obligation, repair, collateral và
replay hiển thị trong UI là artifact Go thật. Không còn fixture comparison/repair.
Input sample là synthetic có nhãn; AI chỉ chạy khi người dùng bấm và vẫn chưa có
live Meta trong môi trường này.

## Cấu trúc

| Đường dẫn | Vai trò |
|---|---|
| `web/src/i18n/` | Dictionary EN/VI, locale mặc định VI, lưu `localStorage`, test key/placeholder |
| `web/src/hooks/useReview.ts` | State machine của luồng, generation guard, cancel/retry, invalidate |
| `web/src/lib/api.ts` | Client cho parse/normalize/compare/session/repair/replay, giữ raw text |
| `web/src/lib/conditions.ts` | Structured condition/region draft; không bắt người dùng nhập JSON |
| `web/src/lib/report.ts` | Báo cáo Markdown theo ngôn ngữ hiện tại; không đổi raw evidence |
| `web/src/components/` | Source, conditions, result, repair, evidence, report view, primitives |
| `web/src/styles/` | Design tokens và CSS riêng của Plenilune; light/dark; responsive |
| `web/tests/browser-qa.mjs` | Brave QA end-to-end, axe, screenshot và download |
| `internal/demohttp/session.go` | Guard server: chỉ `muse-spark-1.3` |

Luồng: **Nguồn & ý định** → **Điều kiện & phạm vi** → **Kết quả** → **Bản sửa** →
**Bằng chứng**. Mỗi bước có một primary action. Kết luận đứng trước; packet, rule,
hash và JSON nằm sau, mở khi cần.

## Runtime, sample và AI

| Phần | Nguồn |
|---|---|
| Parse hai ACL, rule count/line, digest | Go `/api/parse` |
| Canonical conditions + digest | Go `/api/obligations/normalize` |
| Global comparison, witness, obligation outcome | Go `/api/compare` |
| AI proposal/session (khi enabled) | Go `/api/agent/session`, provider chỉ khi có key server |
| Repair candidate, collateral, receipt | Go `/api/repair/verify` |
| Semantic replay | Go `/api/compare/replay`, `/api/repair/replay` |
| Sample input | Synthetic, gắn nhãn “Dữ liệu mẫu”; có SSH drift, budget hạn chế, unsupported |
| AI live | Chưa qualified; không có key trong QA |
| So sánh/repair fixture | Không còn |

- Phạm vi được đổi có ba trạng thái tách biệt: **chưa khai báo** (`null`),
  **không cho phép thay đổi hành vi nào** (`[]`), và **vùng khai báo**. Chưa khai
  báo khiến repair trả `UNVERIFIED`; UI nói rõ, không nâng thành “đã kiểm chứng”.
- AI mặc định off. Mỗi lần gọi có consent, mô tả dữ liệu gửi và nút riêng. Browser
  không đọc/gửi khóa. Không có key server: session trả `not_configured`; UI giữ
  đường tự soạn điều kiện/bản sửa. Server từ chối model khác `muse-spark-1.3`.
- Không hiển thị “verified” từ câu AI nói. Status/witness/collateral/replay do
  engine tạo; AI summary/claims/rationale có nhãn AI riêng.

## Bảo vệ state

- Đổi source/intent/conditions/scope → xóa acceptance, comparison, AI session,
  repair, replay và export cũ; UI quay về bước phù hợp.
- Generation guard chặn response cũ ghi đè phiên mới. Unit test mô phỏng hai
  comparison deferred: response cũ resolve sau response mới bị bỏ.
- Cancel giữ acceptance/conditions và đưa nút retry; lỗi transport/unsupported/
  inconclusive có đường tiếp tục. Replay timing-sensitive không được gọi là PASS.
- Language switch chỉ đổi view; không chạy lại analysis, không gọi AI, không đổi
  input/acceptance/result. QA đếm request `/api/` trước/sau khi chuyển ngôn ngữ.

## Ngôn ngữ và accessibility

- Default VI; lựa chọn lưu trên máy; `html lang`, title, description đổi theo locale.
- Không dịch/sửa ACL, IP, nội dung người dùng nhập, ID, enum, schema hoặc raw
  evidence. Báo cáo dịch là view riêng; JSON tải xuống là response gốc.
- Không dùng LLM để dịch. Dictionary có test đủ key và placeholder.
- Keyboard đi hết luồng; focus visible; live/alert cho trạng thái; status có icon
  và chữ, không chỉ màu; code pane cuộn bằng phím; reduced motion được tôn trọng.
- Responsive 320/390/768/1440; dark mode; không tràn ngang. Primary action ≥44px.

## Chạy

```sh
cd web
npm ci
npm test
npm run build
cd ..
go run ./cmd/plenilune-demo
```

Mở `http://127.0.0.1:8080`. Development: `npm run dev` (Vite 5173, proxy `/api`).
QA script mặc định `QA_BASE_URL=http://127.0.0.1:8091`; truyền URL khác nếu cần:

```sh
cd web
QA_BASE_URL=http://127.0.0.1:8091 node tests/browser-qa.mjs
```

## Validation đã chạy

| Lệnh / kiểm tra | Kết quả |
|---|---|
| `gofmt -l cmd internal` | PASS — không file lệch |
| `go vet ./...` | PASS |
| `go test -count=1 ./...` | PASS — gồm guard model |
| `go test -race -count=1 ./...` | PASS |
| `go build ./...` | PASS |
| `npm test` | PASS — 5 file, 34 test |
| `npm run build` | PASS — tsc (gồm test) + Vite |
| `npm audit --audit-level=high` | PASS — 0 vulnerability |
| Brave `156.0.8078.74` browser QA | PASS — 82 check |
| Axe | PASS — 10 lượt, 0 serious/critical |
| Screenshot | 26 ảnh desktop/mobile/VI/EN/dark/lỗi |
| Download | Comparison, repair, hai AI session, report VI/EN |

Browser QA dùng server thật tại `127.0.0.1:8091`, **không có** `META_MODEL_API_KEY`;
không có request external từ browser. Các path AI session trả `not_configured`.
Stale-response được chứng minh bằng unit test; invalidation qua UI được kiểm trong
browser. Không dùng kết quả QA cũ.

## Artifact

- QA report: `web/docs/qa/redesign-2026-10-30/REPORT.md` (local QA evidence,
  not published)
- Browser results: `web/docs/qa/redesign-2026-10-30/browser-results.json`
  (local QA evidence, not published)
- Axe report: `web/docs/qa/redesign-2026-10-30/axe-report.json` (local QA
  evidence, not published)
- [Design contract](../design/product-ux.md)
- [Wire contracts](../../CONTRACTS.md)

## Chưa chứng minh / giới hạn

- Chưa có live Meta inference, cost/latency hay model-quality evaluation. UI path
  với một session `completed` (proposal/repair) chỉ được test bằng fixture unit,
  không phải Meta runtime.
- Chưa test screen reader thật, real mobile, browser khác Brave/Chromium hoặc
  soak/performance.
- Một packet trace không phải equivalence proof; kiểm chứng điều kiện không chứng
  minh điều kiện diễn đạt đúng ý định. Không kết nối/áp dụng thiết bị.
- Không publish, deploy, push hoặc chọn license trong phiên này.
