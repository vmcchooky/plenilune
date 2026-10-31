# Plenilune — Meta AI và repair loop

Status: Responses adapter + exact engine + verified repair đã chạy một ca live
Contributor synthetic; receipt replay offline matched. Ngày: 30/10/2026.
Chi tiết và giới hạn tại [handoff hiện hành](docs/handoff/contributor-runtime.md).
Authority nghĩa obligation tại [VERIFICATION_DESIGN](VERIFICATION_DESIGN.md); DTO tại [CONTRACTS](CONTRACTS.md).

## Vai trò

Meta chuyển intent thành obligation, chọn câu hỏi kiểm tra, đọc phản ví dụ và đề xuất sửa.
Verifier sở hữu mọi verdict/witness. Model không tự xác nhận phép toán, tính đầy đủ hay độ an toàn của thiết bị.
Demo chỉ dùng `muse-spark-1.3-contributor`; HTTP adapter đã chặn model khác. Không dùng model
đắt hơn hoặc fallback để che vấn đề thiết kế/qualification. Live end-to-end với
engine/repair/UI thật vẫn cần kiểm chứng riêng; nhập key không đủ để qualify.
Không tự đổi provider, mua dịch vụ hoặc đưa model mock thành bằng chứng Meta runtime.

```text
intent + bounded ACL delta
 → propose obligations / ask clarification
 → compile + deterministic checks
 → inspect witness and rule evidence
 → propose typed patch
 → parse + recheck obligations + collateral
 → verified result OR unresolved result with reason
```

## Tools đã triển khai (live qualification riêng)

| Tool | Input | Output |
|---|---|---|
| `list_changes` | IDs + paging bounded | Typed changes, coverage, source refs |
| `check_obligations` | Validated obligation definitions | HOLDS/VIOLATED/INCONCLUSIVE + witness refs |
| `get_evidence` | Existing evidence IDs | Bounded rule/packet/trace details |
| `verify_patch` | Pinned before/after/policy digests + frozen allowed region + typed edits | Candidate checks + collateral status |

Tools chỉ đọc input hoặc tính candidate trong bộ nhớ, không nhận path/shell/credential từ model.
Engine không import provider; orchestration giữ registry tools, context, budgets và transcript.

## Intent và output validation

- Structured JSON schema → strict decoder → semantic validation → region compilation → verifier.
- Unknown fields, duplicate IDs, invalid CIDR/ports và empty region bị từ chối; không âm thầm drop điều kiện.
- “Only X may Y” cần cả tập cho phép X và universe của tập còn lại bị cấm. Thiếu universe → hỏi lại.
- Hiển thị các điều kiện đã dịch cạnh intent; ghi chúng là proposed/accepted theo thao tác thực tế.
- Engine giữ frozen policy; `PolicyAuthority` chỉ xác nhận acceptance khi cả hai input
  digests và conditions digest khớp. AcceptedConditions trong input chỉ là context;
  session kiểm authority trước provider call và không cho model thay set đã freeze.
- Không ghi “intent satisfied” chỉ vì model sinh JSON hợp lệ hoặc confidence cao.
- Mỗi câu khẳng định kết quả phải trỏ evidence ID có thật; explanation free text được đánh dấu AI, escape khi render.
- Model không được nâng `unknown`, `unsupported` hoặc `inconclusive` thành kết luận chắc chắn.

## Budgets và failure

Một session có tối đa 8 model requests (bao gồm retry), 12 local tool invocations, 2 repair proposals và deadline 60s;
verifier mỗi lần vẫn có budget riêng. Dừng ngay khi hết cap, cancel hoặc user không xác định được scope.
Một retry cho transient 429/5xx chỉ khi còn tổng budget; auth/schema không có loop vô hạn.
AI lỗi vẫn giữ deterministic result; repair lỗi vẫn giữ original-after và evidence thất bại.
Status phân biệt disabled, not_configured, unavailable, clarification_needed, partial và completed.
Tất cả cap là giới hạn ban đầu cần đo bằng corpus; không hứa chạy đủ hai vòng trong mọi session.

## Dữ liệu và Meta adapter

Mặc định AI off cho CLI nền. Live adapter chỉ được gọi khi người dùng chủ động bật và có cấu hình.
Demo dùng synthetic ACL/intent. Khi bật AI, UI/help nói rõ excerpt + IP/port/intent nào được gửi ra ngoài.
Không gửi full running-config, remark/banner hoặc credential; giới hạn excerpt/delta trước HTTP.
Config/ticket/tool result đều là untrusted data; prompt delimiter không phải security boundary.
Output schema/allowlisted tools và verifier mới giới hạn quyền của model.
API key chỉ ở environment; TLS verify, không redirect tùy tiện; timeout/body limit; không log key/prompt nguyên văn.
Evidence giữ model/prompt version, request/response hash và usage; giữ accepted obligations để replay không cần AI.

Tài liệu chính thức đã đối chiếu: [models](https://dev.meta.ai/models/muse-spark),
[tool calling](https://dev.meta.ai/docs/tool-calling), [structured output](https://dev.meta.ai/docs/structured-output).
Transport hiện dùng POST `/v1/responses`, flat function tools, `text.format`,
`max_output_tokens` (agent 1200/request), reasoning medium, stream=false,
store=false + encrypted reasoning replay trong session. Không lưu opaque reasoning
vào artifact/log; request 256 KiB / response 1 MiB, 8 model calls/session,
12 tools, 2 proposals, deadline session 60s. Timeout mỗi request 45s.
Client public và HTTP guard chỉ cho Contributor; không request override/fallback.
Một ca live không đủ chứng minh latency/cost/model quality cho corpus rộng.
Tham chiếu wire: [Responses](https://dev.meta.ai/docs/protocols/responses),
[tool calling](https://dev.meta.ai/docs/tool-calling).

## Ranh giới license với Meta

Mã adapter HTTP, tool loop và orchestration trong repo này thuộc
GPL-3.0-only của Plenilune. Model và API của Meta là dịch vụ của Meta, không
được phân phối kèm repo và không được cấp phép lại theo GPL của Plenilune. Khi
bật AI, người dùng phải tuân thủ Meta Model API Terms và Acceptable Use Policy
hiện hành; xem [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES.md). Tên và mark của
Meta chỉ dùng để định danh, không ngụ ý Meta bảo trợ hoặc liên kết với dự án.
