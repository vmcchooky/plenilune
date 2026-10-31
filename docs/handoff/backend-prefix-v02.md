# Prefix-list semantics / replay hardening — 31/10/2026

> Ghi chú lịch sử: handoff này viết trước khi license được chốt. License hiện
> hành là GPL-3.0-only; xem `LICENSE`, `NOTICE` và
> [license-public-prep](license-public-prep.md). Nội dung dưới đây giữ nguyên
> trạng thái tại thời điểm đó.

## Đã hoàn thành

- Chốt [profile canonical IPv4 IOS-style](../semantics/prefix-list-v02.md):
  semantics mô hình tách khỏi qualification thiết bị. Ge/le inclusive rõ ràng;
  host bits và empty lists vẫn ngoài subset, không suy hành vi firmware.
- Parser/engine prefix v0.2: đúng sequence max4294967294, cả explicit/implicit;
  Validate kiểm action/network/range/source refs. Reparse theo source order để
  không giả mạo implicit sequence rồi đảo first-match. Source lines không phải
  tăng theo sequence: explicit sequence khác thứ tự dòng vẫn hợp lệ.
- Context/budget được kiểm trong partition/search/merge/sweep; v0.2 tính thêm
  logical work. Counts complete là canonical prefixes; universe2^33-1 đã có test.
- Frozen prefix parser/engine v0.1 chỉ phục vụ receipt replay. New Analyze dùng
  v0.2; current unsupported không fallback old parser. Exact supported pairs,
  không đổi schema analysis/v1 hoặc proof ACL/repair v1/v2.
- Sửa timing bypass trong analysis replay: timing của ACL không cho phép thay
  prefix counts, sources/extents, coverage, request/policy hay inventory. Point
  witnesses/source refs và điều kiện scope cần thiết vẫn phải khớp trước khi
  trả TIMING_SENSITIVE. Trạng thái này không có nghĩa full region/proof matched.
- Chặn deadline integer overflow trước phép đổi duration.

## Compatibility / review

Hai analysis receipts prefix v0.1 được tạo từ binary cũ trước khi sửa backend:
`testdata/receipts/analysis-routing-v01.json` và `analysis-prefix-v01.json` cùng
nguồn .cfg. Ca thứ hai giữ maxSeq cũ4294967295 để chứng minh replay lịch sử mà
không mở lại cú pháp này cho new analysis. Existing ACL v0.1 frozen fixtures giữ
nguyên. Hash/producer version vẫn không xác thực danh tính hoặc provenance.

Review độc lập đã kiểm semantics/evidence theo AGENTS. Đã sửa thêm hai finding
review: validation từng entry chưa bind implicit sequence vào source order;
timing projection thiếu kiểm witness inside/outside allowed union. Có regression
cụ thể cho cả hai. Không còn blocker trong phạm vi code reviewer đã đọc.

## Validation thực tế

- gofmt sạch; go vet ./..., Go ordinary/race và go build ./... PASS.
  Hai live opt-in tests bị skip chủ động; không gọi Meta.
- Focused prefix/analysis tests PASS: malformed IR, sequence/range/count boundary,
  max IPv4 universe, frozen replay, re-sealed timing tamper, scope-point và deadline.
- Client độc lập `examples/moonstorm-client/client.mjs`: 11/11 PASS với binary
  vừa build `bin/plenilune-prefix-v02.exe`.
- UI/source frontend không sửa. Không suy Brave50/135 hoặc75frontend tests lịch
  sử là QA của backend mới; integration Brave/axe được giao riêng.

Không có Cisco IOS/XE lab hoặc device qualification trong lượt này. Không đọc
key/clipboard, không chạy live Meta, không đổi server người dùng, không sửa
Sơ khai/MoonStorm, không đổi license hoặc publish.

## Tiếp quản

1. Prompt QA độc lập (`PROMPT_QA_PREFIX.vi.md`, internal brief, not published):
   backend/API/replay + UI integration.
2. Sau khi đóng blockers: prompt UI/demo/live/public prep
   (`PROMPT_DEMO_PUBLIC.vi.md`, internal brief, not published).

Build source mới khi dựng server QA; đừng dùng binary/dist hoặc manifests của
snapshot cũ để claim hiện hành. License vẫn chờ chủ dự án chốt.
