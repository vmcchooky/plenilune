# Verified repair + Meta engine adapter — 30/10/2026

> Historical snapshot. Current model guard/transport:
> [contributor-runtime](contributor-runtime.md). The model is fixed to
> `muse-spark-1.3-contributor`.

## Đã nối

- `patch.Apply`: immutable base refs, 1–5 add_before/append/remove/replace, no partial
  apply; giữ source/remark/wrapper, renumber rồi parse lại candidate.
- `repair.Verify`: frozen definitions + accepted policy, all HOLDS và exact
  collateral original-before→candidate ngoài union allowed regions. Nil scope
  chưa khai báo; [] buộc candidate giữ nguyên toàn hành vi trước thay đổi.
- `repair.Replay`: rerender original-after + patch, check candidate/policy/hash,
  replay comparison và collateral; timing không miễn deterministic byte binding.
- `engineadapter.Engine`: real agent.Engine, bounded evidence store, immutable
  input/policy, policy authority; không import provider vào exact core.
- `orchestrate.Run`: session facade; AI sidecar tách khỏi deterministic receipt hash.

## CLI và API để frontend nối

`./scripts/demo-repair.ps1` tạo synthetic VERIFIED repair + REPAIR_REPLAYED.
Artifacts: `out/repair-policy.json`, `out/repair-patch.json`, `out/repair-proof.json`.
`out/offline-session.json` là session AI disabled, không phải Meta runtime proof.

CLI: repair / repair-replay / assist; xem --help và CONTRACTS.
API localhost:

| Path | Body | Result |
|---|---|---|
| `/api/repair/verify` | before,after,policy,patch,options? | plenilune.repair/v1 |
| `/api/repair/replay` | before,after,evidence | REPAIR_REPLAYED / TIMING_SENSITIVE |
| `/api/agent/session` | before,after,intent,policy,enabled,options? | plenilune.session/v1 |

Luồng UI: session chưa có policy → xem proposed conditions/checks → operator
accept conditions + khai báo allowed_changes → gửi lại frozen policy với enabled=true.
Adapter đưa accepted_conditions vào model context và chặn đổi definitions/pins.
Mọi thay đổi source/acceptance phải invalidate receipt/session cũ như generation guard hiện có.
UI đã gọi runtime session/repair/replay; AI live vẫn chưa qualified. Không có khóa
server, UI hiển thị `not_configured`, không tạo receipt và giữ đường thủ công.
Model guard chỉ cho `muse-spark-1.3`; cấu hình khác trả `AI_MODEL_UNSUPPORTED`.
Receipt/candidate hiển thị là artifact Go thật; raw JSON tải xuống là response gốc.

## Qualification và review

Test dùng scripted model và mock HTTP endpoint với **ai.Client thật**, engine,
repair/collateral verifier và receipt replay thật. Chưa có META_MODEL_API_KEY ở
môi trường chạy; chưa có live inference, cost/latency hay prompt-quality qualification.
Khi có key trong server environment, bật assist --ai on hoặc API enabled=true;
không gửi key vào browser/chat. AI off không đọc config hoặc chịu provider excerpt limits.

Review độc lập đã đóng: thiếu ordinal JSON ref, rerender-binding bị bỏ qua khi
timing-sensitive, collateral stats, và offline path bị giới hạn excerpt của provider.
Tests có /25 canonical-witness trap, union/excludes, pins/acceptance, candidate
tampering+rehash+timing, source ownership/race, real tool loop và AI-off/no-key paths.
Không apply file/device, không sửa frontend/Sơ khai/MoonStorm, không publish/đổi license.
