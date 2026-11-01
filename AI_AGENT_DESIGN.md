# Plenilune — Meta AI và repair loop

Status: Responses adapter + exact engine + verified repair đã chạy một ca live
Contributor synthetic; receipt replay offline matched. Ngày: 30/10/2026.
Ghi chú triển khai lịch sử (số liệu cũ, không phải caps hiện hành) tại [Contributor runtime handoff](docs/handoff/contributor-runtime.md); caps/tool hiện hành ở mục Budgets bên dưới và [CONTRACTS](CONTRACTS.md).
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
`verify_patch` chỉ được đưa vào tool list khi session có accepted policy; session đề xuất
điều kiện trước khi duyệt không thấy repair tool.

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

Một session có tối đa 16 model requests (bao gồm retry), 24 local tool invocations, 6 repair proposals và deadline 120s;
verifier mỗi lần vẫn có budget riêng. Dừng ngay khi hết cap, cancel hoặc user không xác định được scope.
Một retry cho transient 429/5xx chỉ khi còn tổng budget; auth/schema không có loop vô hạn.
AI lỗi vẫn giữ deterministic result; repair lỗi vẫn giữ original-after và evidence thất bại.
Status phân biệt disabled, not_configured, unavailable, clarification_needed, partial và completed.
Tất cả cap là giới hạn cứng, không phải mục tiêu dùng hết; provenance của session ghi cả cap lẫn lượng đã dùng,
nên một lượt proposal dừng đúng ở cap đọc được từ record là cạn ngân sách, không phải model tự dừng.
Ngân sách ban đầu (8/12/2/60s) được P1-7 nới ở tầng đề xuất sau khi P1-3 (enumeration deterministic) chạy:
mọi proposal vẫn qua đúng verifier và acceptance vẫn thuộc người, nên nới số proposal không đổi tầng phán quyết.
Đo trên corpus session synthetic (fake provider, `TestP17SessionBudgetCorpus`): draft verify ở lượt 1/3/6 —
caps cũ chỉ xong lượt 1 (dừng ở 2 attempts), defaults mới xong cả ba; draft không bao giờ verify bị chặn ở đúng 6 attempts.
Chi phí/latency live vẫn là việc qualification riêng, không suy từ mock.

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
`max_output_tokens` (agent 3000/request; reasoning tokens cũng tính vào ngân sách
này, 1200 đã bị cạn trong một lượt live UI trước khi có JSON dùng được),
reasoning medium, stream=false,
store=false + encrypted reasoning replay trong session. Không lưu opaque reasoning
vào artifact/log; request 256 KiB / response 1 MiB, 16 model calls/session,
24 tools, 6 proposals, deadline session 120s. Timeout mỗi request 45s. Mỗi request
phải đặt `max_output_tokens` trong 1..4096; adapter không có fallback 1200.
CLI `assist --ai on`, client public và HTTP guard chỉ cho Contributor; không request override/fallback. Build Go từ Git checkout tự đóng dấu `vcs.revision`; `plenilune version` in ra commit/modified. Receipt evidence vẫn dùng placeholder `unversioned` (xem CONTRACTS §5); provenance của binary không được suy từ receipt.
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
