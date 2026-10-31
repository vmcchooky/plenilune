# Foundation review — 30/10/2026

Disposition của external review do owner cung cấp và follow-up review read-only.
Không dùng số finding để suy ra số lỗi semantics; runtime foundation vẫn chỉ có point trace/replay.

| Finding | Disposition |
|---|---|
| F1 scope/demo chưa có | Trạng thái đã ghi đúng. Giữ đích exact + AI + repair + UI; roadmap dùng checkpoint vertical slice, không cam kết mọi adapter hoàn tất trong một ngày. |
| F2/F6 repair refs/IR | Chốt ref base-after digest + ordinal + line và giữ source bytes; không cần global rule ID hay remark trong semantic IR. Patch/render/recheck vẫn là phần sắp triển khai. |
| F3 determinism | Thêm thứ tự traversal/witness, logical budget và TIMING_SENSITIVE vào VERIFICATION_DESIGN; chưa claim exact-engine runtime. |
| F4 submission | Git/remote/license/live Meta/video chưa xử lý trong slice này. File env mẫu trống không chứng minh owner thiếu quyền API. Không tự publish/chọn license. |
| F5 JSON diagnostics | Giữ safe typed domain message qua nested decode; arbitrary decoding error vẫn sanitized. CLI tests kiểm interval lỗi, non-TCP port và không lộ raw input. |
| F7 full evidence/exit | Chốt proposed diff 0/1/2, evidence fields và independent obligation witness trong CONTRACTS. Full Go DTO/replay phải đi cùng exact engine, chưa có ở foundation. |
| F8 demo fixture | Đổi destination thành .10; regression đọc fixture thật chứng minh bỏ deny đầu làm packet đổi deny→permit. |
| F9 ICMP tên/số | Parser nhận icmp như protocol 1; type/code và port constraint vẫn unsupported. |
| F10 line boundary | Đếm logical lines; 10.000 với LF/CRLF/no-final-newline hợp lệ, 10.001 bị từ chối. |
| F11 sequence warning | Partial warning chỉ xét executable rules; mọi sequenced entry kể cả remark vẫn phải tăng. |
| F12 tests | Thêm CLI obligations, explicit trace-source tampering, missing/null decision flag và bounded JSON fuzz. Không mở rộng thành test engine chưa tồn tại. |
| F13 claims | README ghi Goal; review note này lưu scope/disposition. Full tool commit/provenance vẫn phải bổ sung khi có full evidence và versioned build. |

Parser version hiện là **cisco-acl/v0.2**; trace prototype v0.1 được từ chối,
không tự đọc bằng semantics mới. Demo đã tạo lại trace với version hiện hành.

Validation thực chạy trên Windows/amd64, Go 1.26.7: ordinary/race tests, vet,
formatting, build và demo PASS; Linux/amd64 **cross-build** PASS (không phải Linux runtime test).
JSON fuzz PASS: 112.492 executions. Follow-up independent review không còn actionable
finding trong các thay đổi runtime/contract được kiểm. Chưa có exact/Meta/repair/UI runtime proof.
