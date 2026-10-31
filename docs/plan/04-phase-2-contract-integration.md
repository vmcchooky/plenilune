# Phase 2 — Hợp đồng tích hợp & danh tính bằng chứng

Điều kiện vào: Phase 1 đạt điều kiện ra (engine ổn định để đóng băng contract).
Điều kiện ra: P2-1 → P2-4 có contract test và tài liệu; P2-5 (đặc tả MoonStorm)
hoàn chỉnh; trạng thái cổng pháp lý P2-6 được ghi rõ.
Nguồn: Qoder mục 5.3–5.5 + nghiên cứu Q4/Q5/Q7 (cache key, signing, hàng đợi,
sự kiện obligation, mã hóa artifact).

Thứ tự trong phase quan trọng: **dual digest trước** (mọi thứ khác — cache,
ký, bundle — đều neo vào content digest), rồi capabilities, error taxonomy,
bundle, ký, đặc tả tích hợp.

## P2-1 — Hai digest: content và envelope

**Vấn đề (đã kiểm chứng):** `evidence.Comparison` chứa `Tool{Version, Commit}`
(`internal/evidence/comparison.go:30`) và digest marshal toàn bộ struct — cùng
một kết quả ngữ nghĩa nhưng khác build/commit cho content identity khác nhau
(REVIEW3 finding I-2: receipt live ghi commit `unversioned`, build từ checkout
có stamp thật). Caller không dedup/cache ổn định được.

**Làm:**
- `content_sha256`: trên payload ngữ nghĩa (inputs, definitions, scope,
  versions của parser/engine, options, results) — **không** gồm build
  provenance (`tool.commit`, thời gian, model metadata).
- `artifact_sha256`: trên toàn envelope như hiện nay, gồm provenance.
- Caller dùng `content_sha256` làm khóa dedup/cache/idempotency; dùng
  `artifact_sha256` kiểm integrity của đúng artifact đó.
- Đây là thay đổi schema: bump version evidence/analysis tương ứng; receipt
  version cũ vẫn replay bằng đúng quy tắc cũ.

**Nghiệm thu:** cùng input/options, hai build khác commit → `content_sha256`
bằng nhau, `artifact_sha256` khác nhau; tamper payload → content digest lệch;
replay v-cũ PASS; contract test khóa cả hai hành vi.

## P2-2 — Capabilities

**Làm:** lệnh `plenilune capabilities --json` và `GET /api/capabilities` trả:
tool version/commit, danh sách schema + version hỗ trợ, domains/kinds, limits
mặc định và trần, options, features (ví dụ `repair_enumeration`, `audit`), error
codes. Caller kiểm tương thích **trước** khi gửi cấu hình; không khớp → từ chối
job sớm với lý do rõ.

**Nghiệm thu:** contract test snapshot của capabilities; MoonStorm-client mẫu
(`examples/moonstorm-client/`) dùng capabilities để gate trước analyze.

## P2-3 — Error taxonomy thống nhất

**Làm:** mọi lỗi qua CLI/HTTP/subprocess mang `code`, `stage`
(request/parse/validate/engine/budget/replay/provider), `field` khi áp dụng
được, `retryable` (bool), và source line/side khi biết. UI và caller map theo
code/stage — không parse message text. Giữ nguyên các code hiện có ở nơi đã
công bố; bổ sung là chính, đổi tên code cũ phải qua bump contract.

**Nghiệm thu:** bảng error trong CONTRACTS cập nhật; test cho từng stage ít
nhất một lỗi đại diện; lỗi ngân sách/hủy không bao giờ mang trạng thái thành
công một phần không đánh dấu.

## P2-4 — Review bundle manifest

**Làm:** schema `plenilune.bundle/v1`: một manifest JSON liệt kê các file của
một hồ sơ review (before, after, comparison, repair, report, instructions) kèm
role + SHA-256 từng file. Không nén ZIP; receipt vẫn là proof, report là view,
manifest chỉ là mục lục toàn vẹn. Lệnh xuất bundle từ CLI và nút xuất trong UI
dùng cùng một schema.

**Nghiệm thu:** fresh session chỉ từ bundle: import → replay matched; sửa một
byte trong bất kỳ file nào → manifest/replay phát hiện; bundle thiếu file →
lỗi typed, không suy đoán.

## P2-5 — Ký bằng chứng (chia theo cách triển khai)

Theo phán quyết của báo cáo đối chiếu:
- **Khi tích hợp MoonStorm:** MoonStorm ký `content_sha256` bằng Ed25519 (Go
  stdlib, `crypto/ed25519`), lưu khóa và danh tính người ký ở phía MoonStorm.
  Plenilune chỉ cần lệnh verify bằng public key. Không đưa private key vào
  Plenilune core/artifact.
- **Khi chạy standalone:** Plenilune có tùy chọn ký local (khóa do người dùng
  giữ ngoài artifact) trên cùng `content_sha256`.
- Ký trên đúng bytes/digest đã pin; không canonicalize lại bằng JCS ở phase này
  (xem do-not-do). Ghi rõ khắp nơi: chữ ký chứng minh *ai đã ký lên nội dung
  này*, không chứng minh phân tích đúng hay người duyệt đã duyệt.
- Build provenance (phục vụ release, không trộn vào runtime evidence): build từ
  git checkout với `-buildvcs` để binary tự mang commit; cân nhắc attestation
  kiểu SLSA cho artifact phát hành ở Phase 3.

**Nghiệm thu:** verify PASS với đúng khóa, FAIL với sai khóa/nội dung sửa;
không khóa bí mật nào xuất hiện trong artifact/log/test fixture; tài liệu
SECURITY cập nhật mô hình khóa.

## P2-6 — Cổng pháp lý GPL (DECISION — ngoài thẩm quyền agent)

Plenilune là GPL-3.0-only. FSF FAQ: chương trình riêng giao tiếp qua
pipe/socket/exec *thường* được coi là tách biệt, nhưng trao đổi cấu trúc nội bộ
phức tạp với ngữ nghĩa thân mật có thể bị coi là một chương trình hợp nhất, và
phán quyết cuối thuộc tòa án. Mẫu subprocess + JSON có version là mẫu thuận lợi
nhất theo FAQ, nhưng **phân phối MoonStorm kèm Plenilune cần rà soát pháp lý
bằng văn bản trước**. Agent chỉ ghi trạng thái (`pending/done`) vào STATUS;
không kết luận, không để việc này chặn các hạng mục khác — chỉ chặn hành động
phân phối.

## P2-7 — Đặc tả tích hợp MoonStorm (tài liệu, chưa triển khai phía MoonStorm)

Viết đặc tả (đặt tại `docs/plan/` này hoặc bàn giao sang dự án MoonStorm theo
quyết định của Chooky) gồm:
- Pin: analysis schema + engine version + budget profile cho mỗi job; khóa
  idempotency = khóa cache của P2-1/P1-5.
- **Hai hàng đợi tách biệt, collection ưu tiên tuyệt đối:** lỡ backup là mất dữ
  liệu, analysis chậm không sao; analysis chạy `--workers=1` khi MoonStorm tự
  quản pool; giới hạn cứng ở OS (cgroup/Job Object) do MoonStorm áp; process bị
  giết → job INCONCLUSIVE, không bao giờ EQUIVALENT.
- Sự kiện kích hoạt phân tích: capture mới; **và** "obligation-set changed" —
  obligation mới phải chạy lại trên capture cũ đã lưu.
- Lưu trữ: artifact phân tích là dữ liệu suy ra từ cấu hình (witness lộ policy)
  → mã hóa at-rest như capture; chỉ receipt/digest được neo ở lớp metadata.
- Không gộp verdict toàn cục cho N-way drift: MoonStorm so từng cặp
  (baseline↔current, current↔proposed), lưu từng receipt.

**Nghiệm thu của đặc tả:** client mẫu mở rộng chạy được vòng
capabilities → analyze → cache hit theo content digest → replay, trên dữ liệu
synthetic.
