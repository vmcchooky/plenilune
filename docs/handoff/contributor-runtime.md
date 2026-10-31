# Contributor + review workflow — 30/10/2026

> Historical implementation handoff. Current UI/demo QA and public prep:
> [demo-ui-public](demo-ui-public.md). Live UI with Meta was not run there.

## Implemented

- D0: edit/candidate generation guards chặn late verify/replay success/error.
- D1: hai ACL đủ khám phá delta `conditions:null`, chưa cần intent/policy.
- D2: `/api/policy/normalize` canonical toàn policy; preview false trước duyệt,
  freeze true sau duyệt, bind toàn definitions/scope cùng input revision.
- D3: receive/hold/reject riêng, gắn receipt hash. Disputed sau mismatch không
  được giải tỏa bằng đổi candidate hoặc TIMING_SENSITIVE.
- D4: source-byte/standalone receipts export, fresh-session offline import/replay;
  UTF-8 strict, giữ BOM/CRLF/no-final-newline, receipt JSON chuyển nguyên văn.
- Public Meta client + HTTP guard chỉ Contributor, không model fallback/override.
  Stateless `/responses`, flat tools, `text.format`, medium reasoning,
  stream=false, store=false + encrypted reasoning replay nội bộ session.
  Agent cap 1200 output/request, 8 requests, 12 tools, 2 proposals, 60s/session.
  Request timeout45s; body request256KiB/response1MiB. Không export opaque reasoning.

## Live backend evidence

Một ca SSH tổng hợp đã chạy qua server có key tại `127.0.0.1:8082`:

- Actual reported model `muse-spark-1.3-contributor`, AI `completed`.
- 6 model requests, 5 tool calls, khoảng37.7s; usage input27412/output2784,
  tổng30196 tokens. Không quy đổi USD hoặc suy chất lượng corpus từ một ca.
- Original comparison DIFFERENT; một repair VERIFIED, collateral VERIFIED.
- Offline comparison replay COMPARISON_REPLAYED (2 rechecked witnesses).
- Offline repair replay REPAIR_REPLAYED (0 witnesses: candidate tương đương
  original-before, các điều kiện đạt; proof vẫn là full computation).
- Session: `out/live-contributor-session.json`; standalone comparison/repair và
  replay: `out/live-contributor-{comparison,repair}{,-replay}.json`.
- Repair hash `c8672b435ccaa096f62fcf39f17198b4084b91876ac44b4d91727d872bb05a48`.

Đây là **một ca backend live + engine/repair/replay thật**, không phải live browser,
real-device hay model-quality qualification. Giữ JSON receipts và provenance;
không lưu key. Runtime settings qua `scripts/start-local.ps1`, default port8082;
binary `bin/plenilune-demo.exe` được build từ `cmd/plenilune-demo` (tên binary
cũ trong lịch sử là `plenilune-demo-contributor.exe`).

## Review và nghiệm thu

Review độc lập đã kiểm provider, canonical acceptance, candidate revision và byte
import/export. Hai finding được đóng: refusal bị tool-call ghi đè; timing replay
từng giải tỏa dispute. Focused regression có cả hai ca.

Go vet/ordinary/race đã PASS với live tests opt-in bị skip. Frontend 7 files / 53 tests và production build đã PASS. New-flow Brave QA/axe và completed-model UI còn chờ model khác
thực hiện theo prompt nghiệm thu (`QA_NEXT.vi.md`, internal brief, not
published). QA82-check trước là lịch sử,
không đại diện cho snapshot này.

## Post-QA fixes — 30/10/2026

Bốn finding từ nghiệm thu độc lập đã được đóng bằng code + regression, không hạ
assertion:

- **F2 receipt binding:** manual và AI-session repair receipt chỉ được hiển thị,
  nhận hoặc export khi input pair, request pins, obligation digest và full
  accepted policy hash/scope khớp accepted snapshot. Session có bất kỳ receipt lạ
  bị từ chối trước khi hiện candidate; lỗi `RECEIPT_BINDING_MISMATCH` có VI/EN.
- **F1 replay classification:** chỉ `EVIDENCE_MISMATCH` hoặc identity mismatch
  đặt disputed; HTTP validation (`JSON_INVALID`), transport và cancel không.
- **F3 policy preview error:** `POLICY_PREVIEW_MISMATCH` có thông báo VI/EN riêng
  và hướng dẫn xem lại preview/xác nhận lại.
- **F4 segmented contrast:** active/hover giữ nền `var(--ink)`; axe VI/EN
  light/dark đều 0 violation, ratio >13:1; keyboard focus ring còn hiển thị.

Frontend hiện 9 files / 69 tests và production build PASS. Fixed-suite Brave QA:
135/135 check PASS, gồm regression scope-only-policy và recorded-foreign-session.
Evidence trước sửa được giữ nguyên tại `web/docs/qa/acceptance-2026-10-30/`;
evidence sau sửa ở `web/docs/qa/fixes-2026-10-30/`. Live UI chưa chạy vì server
8082 không còn nghe trước bước live; không đọc/khôi phục key, không dùng ca backend
cũ thay cho live UI mới.

Wire reference: [Meta Responses](https://dev.meta.ai/docs/protocols/responses),
[function tools](https://dev.meta.ai/docs/tool-calling). Không đổi proof schemas,
không sửa Sơ khai/MoonStorm, không apply thiết bị hoặc publish.

## Configuration-level review — 31/10/2026

Plenilune now leads with configuration review rather than ACL-only review:

- `internal/analysis`: full-configuration intake, object inventory with exact
  byte/line extents, coverage/limitations, `plenilune.analysis/v1` + replay.
- Domains: named extended IPv4 ACL and IPv4 `ip prefix-list`; other objects are
  `unsupported`, never given a verdict. Unanalyzed sections are listed separately.
- ACL change classes (`plenilune.acl-changes/v1`) group exact differences by
  deciding rule pair with re-evaluated witnesses and optional accepted-scope
  classification. Prefix-list classes report exact route counts.
- Engine v0.2 lowers measured operations from 1.83M to 97k (100-rule corpus) and
  completes 250-rule cases; v0.1 evidence/repair receipts still replay.
- UI: exact-byte file import, configuration overview, object selection, change
  classes, packet probe, full-config/analysis exports and fresh replay. The
  single-ACL flow still auto-opens, so prior paths remain intact.
- Integration: `analyze`/`analyze-replay` CLI, `/api/analyze[/replay]`, and an
  independent Node client at `examples/moonstorm-client/` (11/11 checks).
- Evidence: `web/docs/qa/semantic-diff-2026-10-31/` (new Brave QA, F1–F4
  regression rerun 135/135), `docs/research/` (baseline vs v0.2 measurements).
