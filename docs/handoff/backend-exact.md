# Exact backend — 30/10/2026

Verified repair và Meta engine adapter đã nối tiếp ở [handoff repair](backend-repair.md).

Đã triển khai `internal/packetset`, `internal/verify`, full comparison evidence,
`internal/workflow`, CLI diff/replay và localhost compare/replay API. Slice này
không sửa frontend source; slice UI sau đã nối runtime. Không sửa Meta packages,
Sơ khai, MoonStorm hoặc artifact QA cũ.

## Cách nối frontend

- POST `/api/compare`: `{before,after,conditions,options?}`. Conditions là Set
  chuẩn hóa hiện có; có thể null nếu chỉ cần global diff. Response là
  `plenilune.evidence/v1`, DTO `evidence.Comparison`.
- POST `/api/compare/replay`: `{before,after,evidence}`. Response status
  COMPARISON_REPLAYED hoặc TIMING_SENSITIVE; input bytes phải khớp hoàn toàn.
- `report.verdict`: EQUIVALENT/DIFFERENT/INCONCLUSIVE; `search_complete` chỉ cho
  global comparison. `report.obligations[]` có status riêng và witness riêng.
- Witness chứa packet + before/after decision với ordinal/line source. Dùng
  chính source bytes đã gửi để highlight; không suy ra từ textual diff.
- `report.witnesses_truncated` chỉ cap danh sách đại diện, không đổi verdict.
  Không hiển thị EQUIVALENT là "intent satisfied" khi obligation không HOLDS.
- Body cap 8 MiB cho compare, 12 MiB cho replay. Unsupported/identity/options
  invalid trả 400; valid domain outcomes trả 200. Health báo comparison available.
- UI đã gọi `/api/compare` và `/api/compare/replay`; comparison hiển thị là
  runtime, không còn fixture. Repair/collateral dùng `/api/repair/*` theo
  [handoff repair](backend-repair.md); raw JSON là response gốc.

## Cách chạy và bằng chứng

`./scripts/demo-exact.ps1`: build, compare hai ACL thật, xác nhận DIFFERENT +
obligation VIOLATED, lưu `out/comparison.json`, rồi replay cùng inputs.
`out/exact-before.acl` là synthetic before; after dùng testdata/acl/management.acl.
CLI diff exits 0/1/2; replay timing-sensitive exits 2. Trace commands cũ giữ nghĩa cũ.

PASS trên Windows Go 1.26.7: gofmt, vet, ordinary/race tests toàn project,
build, demo-exact. Trên UI runtime hiện tại: `npm test` 34 test, `npm run build`,
Brave QA 82 check và axe 0 serious/critical. HTTP tests đi qua handler thật và
real engine. Review chéo verifier/workflow/evidence đã đóng finding workspace accounting.

Oracle tests: exhaustive reduced-bit cube pairs và box domains; verifier random
finite-domain ACL so với linear evaluator, first-match/reorder, wildcard,
port0/65535, all protocols/non-TCP port0, exclude, budget/cancel và deadline điều khiển.
Replay tests gồm predicate đổi hash, rehashed false HOLDS bị chặn và hai ACL
equivalent vẫn có obligation VIOLATED với witness riêng.

## Phần chưa qualified

Meta `agent.Engine` adapter và typed patch/collateral đã có ở slice tiếp theo;
live model chưa qualified. Không import agent/provider vào exact core.
Model provenance sau này là sidecar ngoài deterministic digest. Tool commit hiện
unversioned vì project chưa có Git; hash artifact không chứng minh tác giả hoặc build nguồn.
Kết luận exact chỉ trong modeled IPv4 ACL subset, không phải network reachability/device proof.
