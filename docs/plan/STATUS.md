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
| P1-1 | Engine v0.3 LCP residual + cổng benchmark | done | Nhánh `feat/p1-1-lcp-residual` từ main 8ca15b3. Engine mới `acl-exact/v0.3` (`internal/verify/delta_residual.go`, `compare_residual.go`): căn chỉnh tiền tố chung dài nhất theo ngữ nghĩa (cùng match + action; bỏ qua sequence, log, vị trí dòng) và chỉ tính delta trên residual, trên phần bù của covered; permit set của tiền tố không bao giờ vật chất hóa (chính điểm v0.2 chạm cap box ở 512 rule). Residual dùng lại đúng dãy cut của vòng lặp v0.2 (loop dùng chung `taggedPermitSetOver`) nên lost/gained giống v0.2 từng box; không có tiền tố chung → suy biến về đúng phép tính v0.2. Obligation: must_remain dùng lost/gained như cũ; must_permit/must_deny qua permit cover — box hiệu lực của đúng những rule chạm được vùng điều kiện — rồi difference/intersection dạng index như v0.2 (`permitCover`). Cổng benchmark tiền đăng ký (harness baseline sao nguyên không sửa, hai lượt đo cho ops giống hệt): (1) identical-512 HOÀN TẤT EQUIVALENT 1.733 ops (v0.2: INCONCLUSIVE 261.044 ops, stop=boxes) — PASS; (2) append_rule 589@100 / 1.373@250, change_last 589@100 / 1.901@250 so với mục tiêu ≤48.567 / ≤232.272 / ≤48.532 / ≤232.426, median time 32,0→0,5 ms — PASS; (3) insert-at-head bằng đúng v0.2 (105.291@100, 508.952@250; cổng ≤ +10%) — PASS; (4) toàn bộ suite xanh gồm -race — PASS; (5) differential `TestP11DifferentialV03AgainstV02` (600 ca ngẫu nhiên seed cố định, kèm obligation): verdict, trạng thái obligation đã quyết, witness của delta và change class khớp 100% ở mọi ca v0.2 hoàn tất; đúng 1 witness obligation khác biểu diễn box trong 600 ca (packet vẫn chứng minh đúng vi phạm qua evaluator độc lập — tiền lệ v0.1→v0.2); oracle exhaustive reduced-domain nay chạy cho cả hai construction delta. Bonus ngoài cổng: change_last@512 và append 511→512 nay hoàn tất (v0.2 INCONCLUSIVE). Replay giữ nguyên: `engineFor` còn v0.2/v0.1; analysis dispatch Compare/Changes theo `Versions.ACLEngine` (thêm `ChangesEngine`) và `supportedVersions` chấp nhận tuple ACL v0.2 — fixture đóng băng P0-1 vẫn replay nguyên seal; repair bump `acl-repair/v0.3`, `acl-repair/v0.2` bound về engine v0.2. Giới hạn còn lại: swap_adjacent/change_port ở 512 vẫn INCONCLUSIVE (residual còn rule rộng tự phân mảnh chạm cap; swap@512 tốn ops hơn v0.2 trước khi dừng — 344.430 vs 261.044 — kết cục không đổi, không verdict nào bị nâng); obligation trên region toàn cục ở ACL ~512 rule có thể INCONCLUSIVE trong khi verdict hoàn tất; chưa làm suffix alignment (kế hoạch chỉ đăng ký prefix). Số đo: `docs/benchmarks/p1-1-v03-2026-11-01.md` + JSON thô (kèm bộ baseline tương ứng). Tài liệu owner: CONTRACTS, VERIFICATION_DESIGN, ARCHITECTURE đã cập nhật. Cổng Linux đầy đủ: gofmt sạch; vet/build PASS; go test và go test -race 18/18 package PASS; web 97/97 + build PASS (không đụng web). Cổng Windows do owner chạy lại khi merge. Manifest sẽ stale sau merge — regen theo quy trình thường lệ. |
| P1-2 | INCONCLUSIVE một phần theo cặp rule | todo | |
| P1-3 | Liệt kê bản sửa deterministic | todo | |
| P1-4 | Chẩn đoán rule exact (audit) | todo | |
| P1-5 | Song song hóa object + budget toàn cục + cache comparison | todo | |
| P1-6 | Fuzzing vào CI | todo | |
| P1-7 | Nới ngân sách đề xuất AI | todo | Sau P1-3 |
| P1-8 | Luồng verdict nhanh (quick verdict path) | done | Màn "Kết luận nhanh" là entry mặc định: một vùng nạp/thả hai cấu hình (tái dùng SourceEditor + SampleBand của bước nguồn), đủ hai phía là tự chạy explore — verdict hiện với 0 click, kèm dòng phạm vi mô hình rút gọn luôn hiển thị. Obligations nằm inline trên cùng màn (ConditionEditorList/ScopeEditor trích chung với ConditionsStep; xem trước bản chuẩn rồi Chấp nhận & kiểm chứng; listing từng điều kiện theo P0-3 cập nhật tại chỗ; không có điều kiện vẫn tới verdict, không bị chặn). Repair chỉ xuất hiện khi verdict DIFFERENT và ≥1 điều kiện VIOLATED (RepairStep dạng embedded: bỏ header + điều hướng bước, giữ nguyên nút kiểm chứng và quyết định). Bằng chứng là drawer cùng màn, ticket summary là hành động chia sẻ khi đã duyệt policy (không có policy đã duyệt thì chỉ có artifact thô, như DeltaEvidence ở luồng sâu). Quy trình 5 ga giữ nguyên làm chế độ "Duyệt sâu" qua công tắc chế độ, hai chế độ dùng chung một state useReview. Chỉ đổi web + docs; không đụng Go/engine/schema/API. Gate Linux: web 97/97 test (14 file, gồm 6 test mới `p18quick.test.tsx` phủ auto-run 0 click / không điều kiện / sửa inline / repair theo ngữ cảnh / chuyển chế độ sâu) + `npm run build` PASS; Go gofmt/vet/test/`-race`/build PASS (code Go không đổi). usage-vi đã sửa đoạn mô tả luồng mặc định; ảnh chụp màn hình luồng nhanh chờ owner chụp lại ở Windows gate như P0-3. Manifest sẽ stale sau merge — regen theo quy trình thường lệ. |
| P1-9 | CLI tóm tắt (terse CLI verdict) | done | Nhánh `feat/p1-9-terse-cli` từ main acef171. Chỉ thêm vào đường `diff` (đường verdict + obligations của CLI; `analyze` không đánh giá obligations và không có verdict đơn — xem ghi chú phạm vi dưới bảng). `diff --brief --out FILE`: artifact giữ nguyên bytes như output JSON mặc định nhưng ghi vào FILE do caller đặt tên (0600; đúng quy ước CONTRACTS "caller quyết định output path"), stdout chỉ còn 5 dòng cố định, ổn định để grep: `verdict:` (INCONCLUSIVE luôn kèm `reason=` stop reason), `object:` (status của ACL đã so; DIFFERENT kèm `witnesses=` + `witnesses_truncated=true` khi bị cắt), `obligations:` (liệt kê từng obligation + status như P0-3; không có conditions ghi `none evaluated`, không ngụ ý đã kiểm), `limitations: scope=<scope của artifact>`, `artifact: <path>`. Mã thoát brief: 0 EQUIVALENT / 1 DIFFERENT / 2 INCONCLUSIVE / 3 mọi lỗi usage/parse/IO; lỗi brief là một dòng `error: CODE message` ở stderr (kể cả lỗi parse flag, phát hiện qua quét `--brief` trước khi parse), thay cho JSON exit 2 của đường mặc định — đường mặc định không đổi một byte. Brief render thuần từ artifact đã seal (không chạy thêm engine): effects của change class không suy từ witness mẫu vì witness bị cap/truncate — class exact chỉ có trong artifact `analyze` (verify.Changes). Test mới `internal/cli/brief_test.go` + golden `internal/cli/testdata/brief-*.golden`: 4 lớp kết cục byte-exact (EQUIVALENT + obligations HOLDS; DIFFERENT + OB-1 VIOLATED; DIFFERENT không obligations; INCONCLUSIVE reason=operations), artifact brief == bytes output đầy đủ, hợp đồng lỗi exit 3 một dòng (thiếu --out / thiếu file / flag lạ / parse ACL giữ typed code / obligations hỏng / không để lại file artifact khi lỗi), `--out` không `--brief` giữ nguyên exit 2 JSON, help ghi mã thoát. Cổng Linux đầy đủ: gofmt/vet/build PASS, go test + `-race` 18/18 package PASS, web 97/97 + build PASS (không đụng web). Tài liệu: help CLI, CONTRACTS (bảng lệnh + đoạn brief), README. Manifest sẽ stale sau merge — regen theo quy trình thường lệ. |
| P1-10 | Chat thẩm vấn trên màn verdict | todo | Theo quyết định luồng nhanh 01/11/2026; sau P1-7, mở rộng AI session hiện có; chat không bao giờ là đường nhập liệu |

Ghi chú phạm vi P1-9: file phase viết "đường diff/analyze". Trên code hiện tại, obligations chỉ
được đánh giá ở `diff` (`analyze` gọi `verify.Compare` với conditions `nil` — policy của analyze
chỉ phân loại scope cho repair, không sinh obligation result), và verdict đơn
EQUIVALENT/DIFFERENT/INCONCLUSIVE kèm mã thoát 0/1/2 cũng chỉ có ở `diff`. Brief vì vậy chỉ
làm ở `diff`; `analyze --brief` sẽ phải bịa verdict tổng hợp hoặc dòng obligations giả — không làm.

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
