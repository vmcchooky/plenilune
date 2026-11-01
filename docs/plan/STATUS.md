# Plenilune — Trạng thái kế hoạch phát triển

File sống duy nhất về trạng thái. Agent cập nhật sau mỗi hạng mục theo
[protocol](01-agent-protocol.md). Giá trị trạng thái: `todo`, `in-progress`,
`done`, `blocked` (ghi rõ chờ gì), `already-done`, `invalid` (finding không còn
đúng — kèm bằng chứng), `deferred` (Chooky hoãn).

## Quyết định chờ Chooky

| ID | Câu hỏi | Trạng thái |
|---|---|---|
| P0-D1 | Schema cho field đã redact: bump version hay giữ field deprecated? (khuyến nghị: bump) | done — Chooky chốt 31/10/2026: bump schema version (`plenilune.analysis/v1` → `v2`) |
| Luồng nhanh | Thêm luồng sử dụng nhanh vào kế hoạch? (quy trình 5 ga hiện dài dòng, giảm khả năng dùng thực tế) | done — Chooky chốt 01/11/2026: duyệt luồng nhanh xếp tầng — tầng 0: trigger & hộp thư verdict (P2-8); tầng 1: luồng verdict nhanh + CLI tóm tắt (P1-8, P1-9); tầng 2: chat thẩm vấn + bằng chứng trên màn verdict (P1-10) |
| P2-6 | Rà soát pháp lý GPL trước phân phối MoonStorm kèm Plenilune | todo |
| P3-9 | Làm route-maps hay không (và với phạm vi diễn đạt nào)? | todo |
| P3-10 | Vendor thứ hai: làm hay không; nếu làm thì vendor nào? | todo |

## Phase 0 — Hardening

| ID | Hạng mục | Trạng thái | Bằng chứng / ghi chú |
|---|---|---|---|
| P0-1 | Redact secret khỏi artifact và UI | done | Nhánh `fix/p0-1-redact-secrets` từ main a1d3b65. Schema bump v1→v2 theo P0-D1: SectionChange bỏ `header` thô → `header_kind` (≤2 token) + `lines` + `section_sha256`; Reference bỏ `text` → `line`/`kind`/`name`; artifact v1 đóng băng vẫn replay được (fixture `internal/analysis/testdata/analysis-v1-artifact.json` + test P01). Repair receipt có candidate gắn nhãn `sensitive` (ngoài digest projection; receipt cũ giữ digest, vẫn replay). Test synthetic secret: `TestP01ArtifactCarriesNoRawSecretText`, `TestP01ReplayV2ArtifactAndTamperDetection`, `TestP01ReplayAcceptsFrozenV1Artifact`, `TestP01CandidateReceiptIsLabeledSensitive` PASS; cổng Go đầy đủ (gofmt/vet/test/race/build) + web (npm test/build) xanh trên Linux/Go 1.26.7 — cổng Windows do owner chạy lại khi merge. Không đổi verdict/engine/replay semantics. |
| P0-2 | Siết local API (Host/Origin/Fetch Metadata, timeout, semaphore) | done | Nhánh `fix/p0-2-local-api-hardening` từ main b523987. Guard `localOnly` dùng chung cho mọi `/api/*` (`internal/demohttp/guard.go`): Host phải là loopback (`localhost`/`127.0.0.1`/`[::1]`) và — qua `NewHandlerForAddr` mà cmd dùng — phải mang đúng port listen; `Origin` (nếu có) phải là origin http(s) loopback, port không bị khóa (giữ hành vi cũ: UI dev ở port loopback khác vẫn chạy, theo `TestAIOriginAndMissingProvider`); `Sec-Fetch-Site: cross-site` bị từ chối; từ chối = 403 typed `LOCAL_ORIGIN_REQUIRED`. Endpoint AI giữ check riêng `localAIRequest` làm lớp thứ hai. Server (`cmd/plenilune-demo` `newServer`): thêm `ReadTimeout` 60s, `WriteTimeout` 180s (phải vượt deadline phân tích tối đa 120s), `MaxHeaderBytes` 1MiB; giữ `ReadHeaderTimeout` 5s/`IdleTimeout` 60s. `decodeRequest` chuyển sang `http.MaxBytesReader` (body quá cỡ vẫn `JSON_INVALID` như trước). Semaphore 2 cho các route nặng (analyze, analyze/replay, compare, compare/replay, repair/verify, repair/replay, agent/session): vượt quá trả 429 `SERVER_BUSY` + `Retry-After`, không xếp hàng vô hạn; parse/trace/normalize nằm ngoài semaphore vì UI probe gọi 3 trace đồng thời (`web/src/hooks/useReview.ts`) và các route đó nhẹ. Test mới: `TestP02ForeignHostRejectedOnNonAIRoutes`, `TestP02CrossSiteOriginAndFetchMetadataRejected`, `TestP02LoopbackRequestsStillWork`, `TestP02HandlerForAddrRequiresListenPort`, `TestP02ConcurrencyCapRejectsBeyondLimit`, `TestNewServerAppliesHardeningTimeouts` PASS; cổng Go đầy đủ (gofmt/vet/test/race/build) + web (npm test 83/83, build) xanh trên Linux/Go 1.26.7 — cổng Windows do owner chạy lại khi merge. SECURITY.md đã ghi đây là giảm thiểu localhost, không phải bảo mật đa người dùng. Không đổi schema artifact/verdict/engine. |
| P0-3 | UX vi mô (references panel, bỏ framing tỉ lệ, ticket summary, nhãn equivalent) | done | Nhánh `fix/p0-3-ux-micro` từ main 30e2aeb. Chỉ thay đổi web + docs, không đụng engine/schema/backend. (1) Panel “Được dùng tại”: component `ObjectReferences` (`web/src/components/ConfigOverview.tsx`) render `references` dạng v2 (line/kind/name) trong cả hai view chi tiết đối tượng (`PrefixListDetail` + `ObjectDetail`), kèm ghi chú chỉ là tham chiếu cấu trúc — không mô hình hướng in/out hay binding thật — và trạng thái rỗng tường minh khi không có tham chiếu; không hiển thị raw config text. (2) Bỏ framing tỉ lệ: xóa key `result.conditionsSummary` (“Conditions passed: {holds}/{total}”) ở cả hai locale và cả ba nơi dùng (`ResultStep.tsx`, `ReportView.tsx`, `lib/report.ts`); thay bằng `obligationStatusList` (`web/src/lib/labels.ts`) liệt kê từng obligation + trạng thái (“OB-1 violated · OB-2 holds · OB-3 holds” / “OB-1 không đạt · OB-2 đạt · OB-3 đạt”) qua key mới `result.outcomeWord.*`. (3) Ticket summary deterministic: `buildTicketSummary` (`web/src/lib/report.ts`) sinh Markdown VI/EN chỉ từ artifact bằng chứng (verdict, witness + rule theo số dòng, obligations dạng liệt kê, repair, limitations, hash, lệnh replay) — không timestamp, không trạng thái phiên, không prose AI, không raw config text; JSON vẫn là authority (ghi rõ trong summary); tải xuống tại EvidenceStep (`plenilune-ticket-{locale}.md`). (4) Nhãn `overview.status.equivalent`: EN “Text changed; behavior equivalent in modeled scope” / VI “Đổi văn bản; hành vi tương đương trong phạm vi mô hình”, phân biệt với `unchanged`. Test mới `web/tests/p03ux.test.tsx` (8 test: panel có/không tham chiếu cả hai locale, listing hai locale, key tỉ lệ đã xóa, ticket đủ mục + deterministic theo thời gian sinh + không raw text) PASS; i18n/report test đã cập nhật sang dạng liệt kê. Cổng Go đầy đủ (gofmt/vet/test/race/build) + web (npm test 91/91, build) xanh trên Linux/Go 1.26.7 — cổng Windows do owner chạy lại khi merge. `docs/usage-vi.md` đã cập nhật hai chỗ mô tả nhãn tỉ lệ cũ; ảnh screenshots trong usage-vi vẫn chụp UI cũ, chờ chụp lại theo luồng docs của owner. Không phát sinh request AI; JSON artifact không đổi. |

Ghi chú Phase 0: **Phase 0 đã đóng.** Cả ba hạng mục P0-1, P0-2, P0-3 đều `done` và đã merge vào `main` (PR #5, #7, #8; manifest khép phase qua PR #6 và #9; `main` tại 50cdf48, manifest 271 entry đã kiểm chứng). Điều kiện ra của Phase 0 theo `00-master-plan.md` — cả ba hạng mục đạt nghiệm thu — đã thỏa.

## Phase 1 — Engine, repair, fuzz

| ID | Hạng mục | Trạng thái | Bằng chứng / ghi chú |
|---|---|---|---|
| P1-1 | Engine v0.3 LCP residual + cổng benchmark | todo | Cổng đã đăng ký trong file phase |
| P1-2 | INCONCLUSIVE một phần theo cặp rule | todo | |
| P1-3 | Liệt kê bản sửa deterministic | todo | |
| P1-4 | Chẩn đoán rule exact (audit) | todo | |
| P1-5 | Song song hóa object + budget toàn cục + cache comparison | todo | |
| P1-6 | Fuzzing vào CI | todo | |
| P1-7 | Nới ngân sách đề xuất AI | todo | Sau P1-3 |
| P1-8 | Luồng verdict nhanh (quick verdict path) | done | Màn "Kết luận nhanh" là entry mặc định: một vùng nạp/thả hai cấu hình (tái dùng SourceEditor + SampleBand của bước nguồn), đủ hai phía là tự chạy explore — verdict hiện với 0 click, kèm dòng phạm vi mô hình rút gọn luôn hiển thị. Obligations nằm inline trên cùng màn (ConditionEditorList/ScopeEditor trích chung với ConditionsStep; xem trước bản chuẩn rồi Chấp nhận & kiểm chứng; listing từng điều kiện theo P0-3 cập nhật tại chỗ; không có điều kiện vẫn tới verdict, không bị chặn). Repair chỉ xuất hiện khi verdict DIFFERENT và ≥1 điều kiện VIOLATED (RepairStep dạng embedded: bỏ header + điều hướng bước, giữ nguyên nút kiểm chứng và quyết định). Bằng chứng là drawer cùng màn, ticket summary là hành động chia sẻ khi đã duyệt policy (không có policy đã duyệt thì chỉ có artifact thô, như DeltaEvidence ở luồng sâu). Quy trình 5 ga giữ nguyên làm chế độ "Duyệt sâu" qua công tắc chế độ, hai chế độ dùng chung một state useReview. Chỉ đổi web + docs; không đụng Go/engine/schema/API. Gate Linux: web 97/97 test (14 file, gồm 6 test mới `p18quick.test.tsx` phủ auto-run 0 click / không điều kiện / sửa inline / repair theo ngữ cảnh / chuyển chế độ sâu) + `npm run build` PASS; Go gofmt/vet/test/`-race`/build PASS (code Go không đổi). usage-vi đã sửa đoạn mô tả luồng mặc định; ảnh chụp màn hình luồng nhanh chờ owner chụp lại ở Windows gate như P0-3. Manifest sẽ stale sau merge — regen theo quy trình thường lệ. |
| P1-9 | CLI tóm tắt (terse CLI verdict) | todo | Theo quyết định luồng nhanh 01/11/2026; ngân sách dòng cố định + mã thoát theo kết cục |
| P1-10 | Chat thẩm vấn trên màn verdict | todo | Theo quyết định luồng nhanh 01/11/2026; sau P1-7, mở rộng AI session hiện có; chat không bao giờ là đường nhập liệu |

## Phase 2 — Hợp đồng & tích hợp

| ID | Hạng mục | Trạng thái | Bằng chứng / ghi chú |
|---|---|---|---|
| P2-1 | Hai digest: content / envelope | todo | Làm đầu phase |
| P2-2 | Capabilities (CLI + API) | todo | |
| P2-3 | Error taxonomy thống nhất | todo | |
| P2-4 | Review bundle manifest | todo | |
| P2-5 | Ký bằng chứng (MoonStorm ký / standalone ký local) | todo | Trên content digest |
| P2-7 | Đặc tả tích hợp MoonStorm + client mẫu mở rộng | todo | |
| P2-8 | Trigger & hộp thư verdict (event-driven entry) | todo | Theo quyết định luồng nhanh 01/11/2026; watch mode + inbox standalone, phía MoonStorm chỉ đặc tả (thẳng hàng P2-7) |

## Phase 3 — Production & mở rộng

| ID | Hạng mục | Trạng thái | Bằng chứng / ghi chú |
|---|---|---|---|
| P3-1 | Release train (version, changelog, supported versions) | todo | |
| P3-2 | Qualification: live UI, screen reader, thiết bị thật | todo | |
| P3-3 | SMT/Batfish differential oracle trong CI | todo | |
| P3-4 | Stale-decision invalidation | todo | |
| P3-5 | Standard ACL | todo | |
| P3-6 | Object-groups | todo | |
| P3-7 | IPv6 ACL | todo | |
| P3-8 | Thư viện obligation CIS (phần ACL) | todo | |
| P3-9 | Route-maps | todo | Cần quyết định riêng |
| P3-10 | Vendor thứ hai | todo | Cần quyết định riêng |

## Phát hiện ngoài phạm vi

(Agent ghi phát hiện thấy trong lúc làm nhưng không thuộc hạng mục hiện tại vào
đây, kèm file:dòng. Không tự xử lý; Chooky quyết định đưa vào kế hoạch hay bỏ.)

- (trống)
