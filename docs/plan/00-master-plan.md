# Plenilune — Kế hoạch phát triển tới Production

Ngày lập: 31/10/2026 · Chủ dự án: Võ Mạnh Cường (Chooky) · Trạng thái sống tại
[STATUS](STATUS.md) · Quy trình thực thi tại [Agent protocol](01-agent-protocol.md).

## Nguồn gốc và thứ hạng tài liệu

Kế hoạch này hợp nhất ba tài liệu nghiên cứu ngày 31/10/2026 (báo cáo nghiên cứu
Q1–Q8, báo cáo audit code trên local repo, và báo cáo đối chiếu đã kiểm chứng
9/9 claim có trích dẫn code trên bản public). Khi có xung đột cách hiểu:

1. `ENGINEERING_GUARDRAILS.md` và các invariant trong `AGENTS.md` thắng mọi thứ.
2. `CONTRACTS.md` là authority cho wire schema; `ARCHITECTURE.md` cho ownership
   package; `VERIFICATION_DESIGN.md` cho ngữ nghĩa proof.
3. Kế hoạch này là authority cho **thứ tự và phạm vi công việc** — không phải
   authority thứ hai về semantics. Hạng mục nào cần đổi semantics thì bản thay
   đổi phải cập nhật tài liệu owner tương ứng trong cùng lượt.

## "Production chuyên nghiệp" nghĩa là gì với Plenilune

Không phải thêm tính năng vô hạn. Đích là một bộ tiêu chí ra (exit criteria) —
đạt đủ mới được gọi là production:

| # | Tiêu chí | Phase chứa việc chính |
|---|---|---|
| E1 | Không rò dữ liệu cấu hình/secret ra artifact, report hay AI prompt ngoài phần đã được người dùng đồng ý | 0, 2 |
| E2 | Local API chịu được request thù địch ở mức giảm thiểu đã công bố (Host/Origin/Fetch Metadata, timeout, giới hạn đồng thời); docs nói đúng đó là giảm thiểu localhost, không phải multi-user security | 0 |
| E3 | Engine hoàn tất trên corpus chuẩn tới 512 rule hoặc trả INCONCLUSIVE *một phần* nêu rõ phần chưa quyết; không còn INCONCLUSIVE toàn cục câm lặng vì các ca tầm thường (ACL giống hệt, sửa ở cuối) | 1 |
| E4 | Repair không phụ thuộc AI: có enumeration deterministic cho các ca phổ biến; AI là lớp đề xuất cộng thêm, không phải điều kiện để có bản sửa | 1 |
| E5 | Parser/algebra/verifier có fuzzing chạy thường xuyên và oracle độc lập (reduced-domain exhaustive + differential oracle khác họ trong CI) | 1, 3 |
| E6 | Hợp đồng tích hợp ổn định: capabilities, error taxonomy, hai digest (nội dung vs envelope), review bundle — caller như MoonStorm không phải hardcode | 2 |
| E7 | Bằng chứng có danh tính khi cần: ký trên content digest (MoonStorm ký khi tích hợp; ký local khi standalone), provenance build rõ ràng, không claim quá mức | 2 |
| E8 | Có release train: version, changelog, chính sách hỗ trợ version trong SECURITY.md, build từ checkout sạch có VCS stamp, quy trình publish giữ nguyên cổng của chủ dự án | 3 |
| E9 | Qualification trung thực: live UI với model đã qualify riêng, screen-reader pass, corpus synthetic ghi rõ giới hạn; không tuyên bố thiết bị thật khi chưa đo | 3 |
| E10 | Tích hợp MoonStorm chỉ sau cổng pháp lý GPL (rà soát bằng văn bản) và theo đặc tả ở Phase 2 — giữ ranh giới subprocess/JSON, không import thư viện | 2 (đặc tả), ngoài kế hoạch này (triển khai phía MoonStorm) |

## Các phase và thứ tự bắt buộc

| Phase | Tên | Điều kiện vào | Điều kiện ra |
|---|---|---|---|
| 0 | Hardening dữ liệu & local API | Chooky chốt kế hoạch này + quyết định schema `DECISION P0-D1` | Cả 3 hạng mục đạt nghiệm thu; test secret synthetic PASS |
| 1 | Engine nhạy delta, repair chủ động, fuzz | Phase 0 xong | LCP đạt cổng benchmark đã đăng ký trước; enumeration + audit chạy được offline không AI; fuzz CI xanh |
| 2 | Hợp đồng tích hợp & danh tính bằng chứng | Phase 1 xong (engine ổn định để đóng băng contract) | Capabilities/dual-digest/bundle/error taxonomy có contract test; đặc tả tích hợp MoonStorm hoàn chỉnh; cổng pháp lý được ghi trạng thái rõ (chưa xong thì đánh dấu chặn phân phối, không chặn các việc khác) |
| 3 | Mở rộng miền & hoàn thiện Production | Phase 2 xong | Release train chạy thật một phiên bản; qualification E9 hoàn thành ở mức đã công bố; các miền mới (nếu làm) có corpus + tiêu chí fail-closed |

Trong mỗi phase, hạng mục đánh số tăng dần là thứ tự làm. Hạng mục ghi
`DECISION` cần Chooky quyết trước khi bắt đầu; agent dừng và hỏi, không tự chọn.

## Việc bị cấm trong toàn bộ kế hoạch (do-not-do, từ cả ba báo cáo)

- Không thay engine box bằng BDD/FDD/SMT trong runtime; không thêm Z3 hay bất kỳ
  dependency Go bên ngoài nào vào core (SMT chỉ được dùng làm oracle trong CI).
- Không để LLM tạo verdict, risk score số học, hay "mức an toàn"; không dịch
  tỉ lệ điều kiện đạt thành điểm phần trăm.
- Không auto-apply candidate lên thiết bị; không tự xóa rule bị chẩn đoán
  chết/trùng — mọi xóa đi qua repair verification và quyết định của người.
- Không thêm DB/queue/service/plugin system/framework vào core khi chưa có nhu
  cầu đã chứng minh bằng đo đạc.
- Không JCS (RFC 8785) cho tới khi có verifier không phải Go thật.
- Không import Plenilune thành thư viện Go vào MoonStorm trước kết luận pháp lý.
- Không đưa raw config/secret vào artifact, report, ticket hay AI prompt.
- Không đổi license, không tự push/publish/deploy; chủ dự án thực hiện như hiện tại.

## Ngân sách rủi ro AI (áp cho mọi hạng mục liên quan AI)

- **Xanh — tự do:** AI sinh đề xuất (candidate sửa, obligation nháp, giải thích,
  parser nháp sau golden-test gate). Sai chỉ tốn một lượt verify bị từ chối.
  Được nâng số lượng đề xuất trong budget tổng đã công bố.
- **Vàng — có điều kiện:** AI xếp hạng/gợi ý thứ tự hiển thị, với nhãn "gợi ý"
  giữ nguyên và toàn bộ tập đã verify vẫn xem được đầy đủ.
- **Đỏ — cấm:** AI chạm vào verdict, cổng chấp nhận repair (VERIFIED), hay áp
  thay đổi lên thiết bị. Không nới ở bất kỳ mức độ chính xác nào của model, vì
  phép kiểm chứng phải độc lập với bên đề xuất (tránh lỗi tương quan).

## Cách kế hoạch này được cập nhật

Chỉ Chooky hoặc agent được giao mới sửa kế hoạch, và mọi sửa đổi ghi một dòng vào
cuối file này (ngày, ai, đổi gì, vì sao). Trạng thái từng hạng mục chỉ sống ở
[STATUS](STATUS.md) — không ghi trạng thái rải rác trong các file phase.

### Nhật ký thay đổi kế hoạch

- 31/10/2026 — Chí Nhân lập kế hoạch từ ba báo cáo nghiên cứu/audit/đối chiếu
  ngày 31/10/2026 theo yêu cầu của Chooky.
- 01/11/2026 — Chí Nhân bổ sung luồng sử dụng nhanh theo quyết định của
  Chooky: P1-8 (luồng verdict nhanh), P1-9 (CLI tóm tắt), P1-10 (chat thẩm
  vấn trên màn verdict) vào Phase 1 và P2-8 (trigger & hộp thư verdict) vào
  Phase 2, xếp tầng 0 (trigger/inbox) — 1 (verdict nhanh/CLI) — 2 (chat thẩm
  vấn + bằng chứng). Vì sao: nghiên cứu hành vi người dùng cho thấy quy
  trình 5 ga hiện tại dài dòng, làm giảm khả năng dùng thực tế dù chi tiết
  là giá trị cốt lõi — kết luận phải lên trước, chi tiết theo yêu cầu.
