# Plenilune — Trạng thái kế hoạch phát triển

File sống duy nhất về trạng thái. Agent cập nhật sau mỗi hạng mục theo
[protocol](01-agent-protocol.md). Giá trị trạng thái: `todo`, `in-progress`,
`done`, `blocked` (ghi rõ chờ gì), `already-done`, `invalid` (finding không còn
đúng — kèm bằng chứng), `deferred` (Chooky hoãn).

## Quyết định chờ Chooky

| ID | Câu hỏi | Trạng thái |
|---|---|---|
| P0-D1 | Schema cho field đã redact: bump version hay giữ field deprecated? (khuyến nghị: bump) | done — Chooky chốt 31/10/2026: bump schema version (`plenilune.analysis/v1` → `v2`) |
| P2-6 | Rà soát pháp lý GPL trước phân phối MoonStorm kèm Plenilune | todo |
| P3-9 | Làm route-maps hay không (và với phạm vi diễn đạt nào)? | todo |
| P3-10 | Vendor thứ hai: làm hay không; nếu làm thì vendor nào? | todo |

## Phase 0 — Hardening

| ID | Hạng mục | Trạng thái | Bằng chứng / ghi chú |
|---|---|---|---|
| P0-1 | Redact secret khỏi artifact và UI | done | Nhánh `fix/p0-1-redact-secrets` từ main a1d3b65. Schema bump v1→v2 theo P0-D1: SectionChange bỏ `header` thô → `header_kind` (≤2 token) + `lines` + `section_sha256`; Reference bỏ `text` → `line`/`kind`/`name`; artifact v1 đóng băng vẫn replay được (fixture `internal/analysis/testdata/analysis-v1-artifact.json` + test P01). Repair receipt có candidate gắn nhãn `sensitive` (ngoài digest projection; receipt cũ giữ digest, vẫn replay). Test synthetic secret: `TestP01ArtifactCarriesNoRawSecretText`, `TestP01ReplayV2ArtifactAndTamperDetection`, `TestP01ReplayAcceptsFrozenV1Artifact`, `TestP01CandidateReceiptIsLabeledSensitive` PASS; cổng Go đầy đủ (gofmt/vet/test/race/build) + web (npm test/build) xanh trên Linux/Go 1.26.7 — cổng Windows do owner chạy lại khi merge. Không đổi verdict/engine/replay semantics. |
| P0-2 | Siết local API (Host/Origin/Fetch Metadata, timeout, semaphore) | done | Nhánh `fix/p0-2-local-api-hardening` từ main b523987. Guard `localOnly` dùng chung cho mọi `/api/*` (`internal/demohttp/guard.go`): Host phải là loopback (`localhost`/`127.0.0.1`/`[::1]`) và — qua `NewHandlerForAddr` mà cmd dùng — phải mang đúng port listen; `Origin` (nếu có) phải là origin http(s) loopback, port không bị khóa (giữ hành vi cũ: UI dev ở port loopback khác vẫn chạy, theo `TestAIOriginAndMissingProvider`); `Sec-Fetch-Site: cross-site` bị từ chối; từ chối = 403 typed `LOCAL_ORIGIN_REQUIRED`. Endpoint AI giữ check riêng `localAIRequest` làm lớp thứ hai. Server (`cmd/plenilune-demo` `newServer`): thêm `ReadTimeout` 60s, `WriteTimeout` 180s (phải vượt deadline phân tích tối đa 120s), `MaxHeaderBytes` 1MiB; giữ `ReadHeaderTimeout` 5s/`IdleTimeout` 60s. `decodeRequest` chuyển sang `http.MaxBytesReader` (body quá cỡ vẫn `JSON_INVALID` như trước). Semaphore 2 cho các route nặng (analyze, analyze/replay, compare, compare/replay, repair/verify, repair/replay, agent/session): vượt quá trả 429 `SERVER_BUSY` + `Retry-After`, không xếp hàng vô hạn; parse/trace/normalize nằm ngoài semaphore vì UI probe gọi 3 trace đồng thời (`web/src/hooks/useReview.ts`) và các route đó nhẹ. Test mới: `TestP02ForeignHostRejectedOnNonAIRoutes`, `TestP02CrossSiteOriginAndFetchMetadataRejected`, `TestP02LoopbackRequestsStillWork`, `TestP02HandlerForAddrRequiresListenPort`, `TestP02ConcurrencyCapRejectsBeyondLimit`, `TestNewServerAppliesHardeningTimeouts` PASS; cổng Go đầy đủ (gofmt/vet/test/race/build) + web (npm test 83/83, build) xanh trên Linux/Go 1.26.7 — cổng Windows do owner chạy lại khi merge. SECURITY.md đã ghi đây là giảm thiểu localhost, không phải bảo mật đa người dùng. Không đổi schema artifact/verdict/engine. |
| P0-3 | UX vi mô (references panel, bỏ framing tỉ lệ, ticket summary, nhãn equivalent) | todo | |

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

## Phase 2 — Hợp đồng & tích hợp

| ID | Hạng mục | Trạng thái | Bằng chứng / ghi chú |
|---|---|---|---|
| P2-1 | Hai digest: content / envelope | todo | Làm đầu phase |
| P2-2 | Capabilities (CLI + API) | todo | |
| P2-3 | Error taxonomy thống nhất | todo | |
| P2-4 | Review bundle manifest | todo | |
| P2-5 | Ký bằng chứng (MoonStorm ký / standalone ký local) | todo | Trên content digest |
| P2-7 | Đặc tả tích hợp MoonStorm + client mẫu mở rộng | todo | |

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
