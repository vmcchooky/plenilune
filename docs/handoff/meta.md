> Historical adapter snapshot. Current transport/model/live evidence: [contributor-runtime](contributor-runtime.md).

# Meta adapter + AI orchestration — handoff

Snapshot trước khi nối engine thật. Trạng thái hiện hành xem [backend-repair.md](backend-repair.md);
các kết quả test dưới đây thuộc lượt triển khai Meta adapter ban đầu.

Ngày 30/10/2026. Slice sở hữu `internal/ai/`, `internal/agent/`, test tương ứng và file này.
Không sửa parser/intent/evidence, CLI, frontend hay tài liệu root.

## Trạng thái

Đã có adapter HTTP Meta và tool loop chạy được với provider/engine inject. **Chưa có live
Meta runtime**: môi trường không có `META_MODEL_API_KEY`, hai live test skip. Không dùng fake
làm bằng chứng Meta. Engine exact đang làm riêng; test dùng fake engine.

## Ports đã công bố

Engine adapter implement (consumer-owned interface):

```go
type Engine interface {
    ListChanges(ctx context.Context, req ListChangesRequest) (ChangesPage, error)
    CheckObligations(ctx context.Context, set intent.Set) (ObligationCheck, error)
    GetEvidence(ctx context.Context, req EvidenceRequest) (EvidenceBatch, error)
    VerifyPatch(ctx context.Context, req PatchCheckRequest) (PatchCheckResult, error)
}
```

- `CheckObligations` trả `HOLDS | VIOLATED | INCONCLUSIVE` + evidence IDs; VIOLATED phải có
  witness ID. Agent không tạo verdict, chỉ kiểm tính nhất quán/digest của kết quả engine.
  Engine adapter map `UNSUPPORTED` (nếu có) thành `INCONCLUSIVE` kèm reason; không nâng thành
  HOLDS.
- `GetEvidence` chỉ trả record cho ID đã biết, tối đa 8; agent không cho model “fishing”.
- `VerifyPatch` nhận `edits` (≤5; `add_before|append|remove|replace`, ref
  `{base_after_sha256, ordinal, line}`) + `base_before_sha256`, `base_after_sha256`,
  `obligations_digest`, `scope`. **Allowed-change region, original-before bytes và policy
  acceptance thuộc engine adapter**, agent không gửi/không cho model đổi các pin này.
  Candidate parse/grammar failure nên trả `REJECTED` + reason bounded (không phải Go error)
  để model sửa trong vòng repair; engine error được coi là `UNVERIFIED`, không phải reject
  ngữ nghĩa.
- Package engine có thể implement trực tiếp bằng cách import `internal/agent`; hoặc viết
  adapter mỏng để tránh phụ thuộc ngược. Engine adapter được khởi tạo với before/after ACL
  bytes, identity và allowed-change policy; các method chỉ nhận typed request, không nhận
  path/config/shell.
- Repair `append` không cần ref. `add_before|remove|replace` cần `ordinal`/`line`; caller
  phải đưa refs qua `Input.Changes` hoặc engine trả chúng trong `list_changes` — model không
  được bịa ref từ excerpt.

Frontend nhận `agent.Result` (JSON):

| Field | Ý nghĩa |
|---|---|
| `schema` | `plenilune.ai/v1` |
| `status` | `disabled`, `not_configured`, `unavailable`, `clarification_needed`, `partial`, `completed`, `canceled` |
| `obligations` | set canonical đã validate + digest; `operator_accepted=false` trong slice này |
| `checks` | kết quả engine, kèm evidence IDs |
| `repair` | attempt cuối: status `VERIFIED|REJECTED|UNVERIFIED`, collateral, evidence IDs |
| `claims` | text AI + `obligation_ids`/`evidence_ids` đã kiểm tồn tại; `ai_generated=true` |
| `summary` | free text AI, renderer phải escape và gắn nhãn AI |
| `provenance` | provider/model/prompt version, requests, retries, tool calls, usage, SHA-256 request/response, elapsed |

`claims.text`/`summary` không được trình bày như verdict. UI phải hiện checks/repair của
engine cạnh chúng.

## Nối vào

```go
cfg, err := ai.ConfigFromEnv(os.Getenv) // chỉ gọi khi operator bật AI
client, err := ai.New(cfg)
session, err := agent.New(agent.Config{
    Enabled: true, Provider: client, Engine: engineAdapter, Model: cfg.Model,
})
res, err := session.Run(ctx, agent.Input{
    Intent: intentText, ACLName: name, Before: beforeRules, After: afterRules,
    Changes: changes, BaseBeforeSHA256: beforeHash, BaseAfterSHA256: afterHash,
})
```

- AI off: `agent.Config{Enabled:false}` → `disabled`, không đọc env/key, không gọi mạng/engine.
- `agent.New` mặc định 8 model requests (gồm retry), 12 tool calls, 2 repair proposals,
  deadline 60s; provider timeout 20s/request; retry backoff 250ms.
- Một retry cho 429/5xx; 401/403/400/timeout/malformed không retry. Cancel/deadline dừng ngay.
- 429/5xx retry và mọi request đều tính vào budget 8.
- `Reason` khi lỗi: `provider_auth_failed`, `provider_rate_limited`, `provider_timeout`,
  `provider_rejected`, `provider_invalid_response`, `provider_request_too_large`,
  `provider_response_too_large`, `provider_unavailable`, `provider_refused`,
  `model_budget_exhausted`, `tool_budget_exhausted`, `invalid_model_output`, `claims_dropped`,
  `checks_unavailable`, `repair_unverified`, `obligations_not_held`. `partial` khi đã có
  accepted set/check; `unavailable` khi chưa có deterministic result.

## Wire Meta đã dùng

Đối chiếu tài liệu chính thức ngày 30/10/2026:
`dev.meta.ai/docs/api-reference`, `/docs/tool-calling`, `/docs/structured-output`,
`dev.meta.ai/models/muse-spark`.

- `POST https://api.meta.ai/v1/chat/completions`, `Authorization: Bearer $META_MODEL_API_KEY`.
- Tools nested: `{"type":"function","function":{"name","description","parameters"}}`;
  assistant `tool_calls[]`; tool result `{"role":"tool","tool_call_id","content"}`.
- Structured output: `"response_format":{"type":"json_schema","json_schema":{"name","schema"}}`.
- Model mặc định `muse-spark-1.3`, đổi bằng `PLENILUNE_META_MODEL`; base URL đổi bằng
  `PLENILUNE_META_BASE_URL` (bắt buộc https, trừ loopback cho test).
- Adapter không gửi streaming, ảnh hay full config. Request ≤256 KiB, response ≤1 MiB,
  content ≤64 KiB, tool args ≤16 KiB; TLS verify; không follow redirect.
- Key chỉ đọc từ env, không ghi `.env`, không log; lỗi provider redacted theo key.
- Request/response chỉ lưu SHA-256 + usage, không lưu prompt/response thô.

Lưu ý: tổ hợp tools + `response_format` trong cùng request chưa được live-test. Nếu Meta từ
chối, đặt `agent.Config.PromptOnlyJSON=true` để bỏ `response_format`; strict decoder cục bộ
vẫn giữ nguyên.

## Kiểm tra đã chạy

```text
gofmt -l cmd internal                  -> rỗng
go vet ./...                          -> PASS
go test -count=1 ./...                -> PASS (7 package)
go test -race -count=1 ./...          -> PASS
go build ./...                        -> PASS
go test -run TestLiveMeta ...         -> SKIP (không có key)
```

49 test function PASS; 2 live test SKIP. Test bao phủ: wire request/response, provider
401/429/5xx, response/request quá lớn, redirect, cancel/timeout; strict JSON + envelope
bắt buộc; obligation compile; tool args (refs, op, content, pins, duplicate); budget model/
tool/repair; claim trỏ evidence/obligation không tồn tại bị drop → `partial`; engine digest/
witness sai bị từ chối; patch engine lỗi giữ check gốc; clarification; injection intent vẫn
là data; repair verified chỉ khi engine trả đủ HOLDS + collateral VERIFIED.

## Chưa có / cần nối

1. **Live Meta smoke**: chạy khi có key, không in key:
   `PLENILUNE_LIVE_META=1 go test -run TestLiveMetaStructuredOutput ./internal/ai`
   `PLENILUNE_LIVE_META_AGENT=1 go test -run TestLiveMetaAgentSession ./internal/agent`
   Lần chạy này phải xác nhận model ID/quota, shape tools + structured output.
2. **Engine adapter thật**: hiện test dùng fake. Chưa có evidence/witness/collateral runtime
   thật, nên chưa claim exact/repair verified ngoài fake.
3. **Operator acceptance**: agent đánh dấu `operator_accepted=false`; UI/engine phải chấp
   nhận obligation set + allowed-change region trước khi coi repair là policy-approved.
4. **Full evidence owner**: AI artifact (`provenance`, hashes, usage) phải nằm ngoài hash
   deterministic và được bind cạnh accepted obligations theo CONTRACTS; slice này không tạo
   schema full evidence thứ hai.
5. **CLI/UI wiring**: chưa thêm command hay màn hình; frontend dùng `agent.Result`.
6. **Claim semantics**: agent chỉ kiểm evidence ID tồn tại, không chứng minh câu claim được
   evidence entail; renderer phải hiện evidence + nhãn AI. Prompt injection chỉ giảm, không
   loại bỏ rủi ro model đề xuất obligation sai.
7. **Corpus/quality**: chưa đo prompt/model trên corpus thật, chưa tune rate limit/cost.
8. **Input privacy**: excerpt bị chặn nếu chứa secret/config header/remark/sequence; intent
   vẫn do operator gửi và có thể chứa dữ liệu nhạy cảm — chỉ dùng synthetic cho demo.

## Trạng thái claim

Có thể nói: “adapter HTTP + tool loop + strict validation chạy đúng trong test với fake
provider/engine; build/vet/race PASS”. Không thể nói: “Meta runtime đã chạy”, “repair đã
verified trên engine thật”, hay “UI đã hiển thị”.
