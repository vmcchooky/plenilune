# Contract nền và điểm nối kế tiếp

Ngày 30/10/2026. Các mục 1–7 **implemented**. Một ca backend Contributor live
đã hoàn tất với repair/replay thật; browser QA luồng D0–D4 và completed-model UI
cần chạy riêng. Prior UI QA không đại diện snapshot mới.
Go DTO hiện tại ở `internal/ciscoacl`, `internal/intent`, `internal/evidence`.

## 1. CLI và input

| Command | Input | Output |
|---|---|---|
| `version` | Không | tool/version/commit/modified/go/parser |
| `parse --config FILE` | Một ACL-only file | Normalized ACL IR; line 1-based, ordinal 0-based |
| `trace --config FILE --packet FILE` | ACL + tuple | `plenilune.trace/v1` |
| `verify --config FILE --evidence FILE` | Original bytes + trace | `TRACE_REPLAYED`, digest, scope |
| `obligations --file FILE` | Condition set | `VALIDATED_NOT_EVALUATED`, canonical conditions, digest |
| `diff --before FILE --after FILE [--obligations FILE]` | Hai ACL + optional conditions | Full comparison evidence; exit 0/1/2 |
| `diff --brief --out FILE --before FILE --after FILE [--obligations FILE]` | Như `diff` + path artifact do caller chọn | Evidence JSON ghi vào FILE (bytes y hệt output mặc định); stdout là tóm tắt cố định 5 dòng; exit 0/1/2 theo verdict, 3 khi lỗi |
| `replay --before FILE --after FILE --evidence FILE` | Original pair + full evidence | COMPARISON_REPLAYED hoặc TIMING_SENSITIVE |
| `repair --before FILE --after FILE --policy FILE --patch FILE` | Frozen policy + pinned patch | Repair receipt; 0 VERIFIED / 1 REJECTED / 2 UNVERIFIED/error |
| `repair-replay --before FILE --after FILE --evidence FILE` | Original pair + repair receipt | REPAIR_REPLAYED hoặc TIMING_SENSITIVE |
| `assist --before FILE --after FILE --intent TEXT --ai off\|on [--policy FILE]` | Intent + optional frozen policy | Session sidecar + deterministic receipts; AI defaults off |

Exit 0: operation thành công, kể cả packet bị deny. Exit 2: usage/input,
unsupported, cancellation, I/O hoặc replay mismatch. JSON result ở stdout;
JSON error có `code`, `message`, source `line` khi biết ở stderr. Không có output
result khi input/replay thất bại; lỗi ghi stdout có thể để lại stream JSON dở.
Không tự lưu/overwrite file; caller quyết định output path.

ACL input <=2 MiB, <=10.000 dòng, <=8.192 byte/dòng, <=512 executable rules.
Chỉ một `ip access-list extended NAME` block, rule phải indent; cho blank, `!`,
`end`. Full running-config, nhiều/empty ACL, banner và command khác bị từ chối.
Protocol tên `ip/tcp/udp/icmp` hoặc số 0–255; IPv4 any/host/wildcard kể cả không liên tục;
numeric port `eq/neq/lt/gt/range` cho TCP/UDP; sequence tăng, remark, log/log-input.
Không suy đoán hostname, service name, object-group, TCP state hoặc platform.

## 2. Packet và trace

Packet JSON <=4 KiB, đủ năm trường, xem [fixture](testdata/packet.json).
`protocol` 0–255; địa chỉ IPv4; port 0–65535. Non-TCP/UDP phải có cả hai port bằng 0.
Scope cố định: `ipv4-unfragmented-or-initial-with-complete-l4/v1`.
Đây là packet trong mô hình với L4 header đầy đủ, không kiểm tra fragment trên wire.

Trace gồm `schema`, `scope`, `parser_version`, `config_sha256`, `packet`,
`decision`, `sha256`. Decision: action permit/deny, explicit rule ordinal/line
hoặc `implicit=true` + deny và không có explicit source. `action` và `implicit`
bắt buộc non-null; không suy missing thành false.
Config digest lấy đúng input bytes; CRLF/whitespace đổi cũng mismatch dù nghĩa không đổi.
`sha256` lấy SHA-256 compact JSON của fixed-order projection gồm mọi trường
trace trừ `sha256`, theo Go DTO v1. Không claim JSON canonical chuẩn liên ngôn ngữ;
adapter dùng thứ tự/encoding này hoặc gọi binary để verify.

`verify` kiểm schema/scope/parser/hash/input bytes, parse lại ACL, evaluate lại
packet và so cả action/source. Hash không chứng minh tác giả; kẻ sửa nội dung
và hash có thể tạo một artifact khác. Một trace không chứng minh equivalence.

## 3. Obligation definitions

Schema `plenilune.obligations/v1`; scope như trace; xem
[fixture](testdata/obligations.json). `obligations` có 1–12 phần tử:

- `id`: unique `OB-1` đến `OB-99`; `kind`: must_permit/must_deny/must_remain.
- `match.protocol`: ip/tcp/udp/icmp hoặc số dạng chuỗi; 6/17/1 normalize thành tên.
- `src_cidr`, `dst_cidr`: IPv4 prefix hoặc any; host bits được mask khi normalize.
- `exclude_src`, `exclude_dst`: <=16 prefix mỗi phía, nằm trong include, không
  được hợp lại xóa hết include; normalize loại prefix trùng/bị bao phủ.
- `src_ports`, `dst_ports`: <=16 interval `{lo,hi}`, hai bound bắt buộc non-null,
  0–65535, lo<=hi. TCP/UDP empty/null/omitted array nghĩa toàn bộ port;
  protocol khác chỉ cho array rỗng/null/omitted. Merge overlapping/adjacent intervals.

Digest = SHA-256 compact JSON của **normalized complete Set**, gồm schema, scope,
ID, kind, toàn bộ region/excludes/ports. Sort obligations theo ID, excludes theo
canonical string, intervals theo bound. Nil collection normalize thành `[]`.
Digest này chưa chứa result vì scaffold chưa evaluate obligation. Full evidence
sau này phải bind definitions **và** result; model metadata tách riêng.
Đổi predicate phải đổi digest; validate thành công không chứng minh lời nói đã
được dịch đúng hoặc ACL đáp ứng điều kiện.

## 4. JSON và version

UTF-8; reject duplicate key (kể cả escape alias), unknown field, sai hoa/thường,
trailing JSON, sai type, nesting >64. Null không thay required schema/scope,
identity, IP, packet numeric field, decision action/implicit hoặc interval bound. Parser và scope version
không được tự đoán khi replay. Wire change cần version/fixture/test cùng thay đổi.

## 5. Full comparison và API — implemented

Schema `plenilune.evidence/v1`, không đổi nghĩa trace v1. DTO `evidence.Comparison`
gồm schema/tool/parser_version/engine_version/scope/before/after/conditions/report/sha256.
Before/after pin raw bytes digest + ACL name/kind; conditions là normalized Set hoặc null.
Report gồm verdict, search_complete, reason, witnesses, witnesses_truncated,
obligations, budget và options. Mỗi witness có ID, packet, before/after decision+source.
Global verdict và từng obligation có outcome riêng.
Obligation VIOLATED có witness riêng, không buộc phải trỏ global difference.
Lệnh `diff`: 0=EQUIVALENT, 1=DIFFERENT, 2=INCONCLUSIVE/error; unsupported input trả typed error exit 2.
Chế độ `diff --brief` (bắt buộc kèm `--out FILE`; `--out` không có `--brief` bị từ chối): artifact
không đổi — cùng bytes như output JSON mặc định — nhưng ghi vào FILE do caller đặt tên, còn stdout
chỉ gồm 5 dòng ổn định để grep: `verdict:` (INCONCLUSIVE luôn kèm `reason=` stop reason),
`object:` (status của ACL đã so + số witness khi DIFFERENT), `obligations:` (liệt kê từng
obligation + status; không có conditions thì ghi `none evaluated`, không ngụ ý đã kiểm),
`limitations: scope=...`, `artifact: <path>`. Mã thoát brief: 0=EQUIVALENT, 1=DIFFERENT,
2=INCONCLUSIVE, 3=mọi lỗi usage/parse/IO; lỗi brief là đúng một dòng `error: CODE message`
ở stderr, không phải JSON.
Obligations độc lập: diff DIFFERENT nhưng mọi obligation HOLDS vẫn exit 1.
Tool commit trong evidence là placeholder `unversioned` để replay và kiểm build identity ổn định; không claim immutable source provenance. Binary build từ Git checkout được Go đóng dấu `vcs.revision`/`vcs.modified` (mặc định `-buildvcs=auto`); `plenilune version` in ra commit/modified. Đó là provenance của binary, không phải của receipt.
Một witness gồm packet và decision/source refs cho mỗi phía cần kiểm chứng;
obligation must_permit/deny chỉ cần after, must_remain cần cả before và after.
Digest = SHA-256 compact JSON toàn Comparison trừ trường sha256 (omitempty).
Hash bind definitions/scope/results/budgets/options; chưa có elapsed/AI metadata trong DTO này.
Replay kiểm schema/hash/input bytes, recheck witnesses rồi chạy lại engine cùng options.
Logical budget outcomes replay được; timing-limited original/rerun trả TIMING_SENSITIVE,
không full replay PASS. Điều này không phải chứng nhận ý định ngôn ngữ tự nhiên.

Localhost API: POST `/api/compare` nhận `{before,after,conditions,options?}`;
options omitted dùng defaults. POST `/api/compare/replay` nhận `{before,after,evidence}`.
Domain verdict hoặc replay status nằm trong JSON HTTP 200; input/error HTTP 400.
Body cap lần lượt 8 MiB/12 MiB; source vẫn chịu cap 2 MiB mỗi bên.
Options: limits `{max_operations,max_boxes,timeout_ns}` và max_witnesses; chỉ cho hạ
cap defaults 2M/50k/2s, max_witnesses 1–64 (default 8). UI gọi trực tiếp endpoint
này; sample input được dán nhãn, còn comparison hiển thị là kết quả runtime. Màn
hình có thể hạ budget cho một sample minh hoạ; retry mặc định gửi options omitted.

## 6. Repair và Meta session — implemented

AI tools khởi đầu: `list_changes`, `check_obligations`, `get_evidence`. `verify_patch`
chỉ được thêm vào tool list sau khi session có policy operator-accepted bind đúng input
digests + conditions digest; gọi sớm bị session trả `obligations_not_checked`, và engine
trả `operator_policy_not_accepted` nếu policy chưa accepted. Tool input nhận typed
ID/condition, không file path/shell.
Patch `plenilune.patch/v1` pin original-before digest, base-after digest,
accepted-obligations digest, scope và allowed-change region từ policy đã được
operator accept/freeze; model không được tự đổi policy này. Rule ref là
`{base_after_sha256, ordinal, line}`, kiểm cả ba trên immutable base; không hứa ID
giữ nguyên qua phiên bản. Giữ source bytes để report/render remark đúng vị trí.
Edits v1 (tối đa 5): add_before/append, remove, replace; refs đều thuộc cùng base,
reject duplicate/conflicting refs. Nội dung thêm/thay là một supported rule;
không cho newline, header, remark hoặc command ngoài grammar trong edit payload.
Renderer giữ các dòng không sửa/remark, cấp lại sequence tăng cho mọi entry có
sequence (kể cả remark), rồi parse lại toàn candidate. `move` chưa có primitive
riêng; reorder phải qua proposal remove+add được kiểm lại.
Apply atomically trong memory, parse/verify lại, tối đa hai repair rounds. Mỗi candidate phải kiểm
obligations và collateral original-before→candidate ngoài allowed-change region
đã khai báo. Partial check không được mang nhãn repair verified.

Frontend gọi runtime `/api/compare`, `/api/agent/session`, `/api/repair/verify` và
`/api/repair/replay`; comparison, obligation, repair, collateral và replay hiển thị
là artifact Go thật. Input sample là dữ liệu tổng hợp có nhãn; không còn fixture
comparison/repair. AI off mặc định, mỗi phiên phải do người dùng bấm và browser
không giữ khóa. Không có khóa server thì session trả `not_configured` và UI hiển
thị đúng trạng thái đó, vẫn giữ đường tự soạn điều kiện/bản sửa. CLI `assist --ai on` và demo server từ chối model khác `muse-spark-1.3-contributor` bằng `AI_MODEL_UNSUPPORTED` trước network (không fallback). Adapter cũng từ chối request thiếu `max_tokens` hoặc ngoài 1..4096; không có fallback 1200.
UI không tạo evidence schema riêng: JSON tải xuống là response gốc; báo cáo VI/EN
là một view riêng. Full evidence DTO vẫn do Go sở hữu.

Policy `plenilune.policy/v1`: scope, operator_accepted, conditions Set,
allowed_changes (<=12 Region). Nil/null/omitted allowed_changes = undeclared;
[] = explicit zero allowed changes. Normalize/digest giữ distinction này.
Repair `plenilune.repair/v1`: version, status/reason, input hashes, frozen policy/hash,
patch request, options, candidate/hash, conditions/digest, comparison, collateral,
sha256. Digest là compact JSON toàn DTO trừ sha256. Không có AI metadata trong hash.
Receipt có candidate (toàn văn cấu hình ứng viên) được gắn nhãn `"sensitive": true`;
nhãn là metadata xử lý, bị loại khỏi digest projection, nên receipt cũ không có nhãn
giữ nguyên digest và vẫn replay được. Nhãn không phải cơ chế bảo mật; nơi lưu trữ
phải xử lý mọi repair receipt như dữ liệu nhạy cảm.
VERIFIED cần acceptance thật từ caller policy, all HOLDS và collateral VERIFIED;
UNVERIFIED không được nâng thành thành công. Replay rerenders original-after + patch
và so bytes/policy trước timing shortcut; replay PASS không đổi status gốc của proposal.

API: POST `/api/repair/verify` `{before,after,policy,patch,options?}` cap 8 MiB;
POST `/api/repair/replay` `{before,after,evidence}` cap 16 MiB. Domain statuses HTTP200;
invalid input/error HTTP400. POST `/api/agent/session` nhận
`{before,after,intent,policy,enabled,options?}` cap 8 MiB. Enabled defaults false;
true mới đọc key/config từ server env, yêu cầu Host/Origin local, không nhận key từ browser.
Session `plenilune.session/v1` gồm comparison, ai (`plenilune.ai/v1`) và repairs[];
AI/provenance là sidecar. Missing key -> not_configured cùng comparison thật.
PolicyAuthority đối chiếu input+definition hash với immutable caller-supplied
accepted policy trước provider call; không tự tạo authenticated approval record.
Full witness IDs được map thành bounded evidence IDs <=64 ký tự ở agent port.

Frontend receipt binding (30/10/2026): receipt sửa thủ công hoặc từ AI session chỉ
được hiển thị/nhận/export khi `before_sha256`, `base_after_sha256`, pin trong
`request`, obligation digest và full `policy_sha256` khớp accepted snapshot
(scope cũng khớp). Receipt lạ trả `RECEIPT_BINDING_MISMATCH` trước khi tạo kết luận
có thể nhận; UI không tin response thuộc request generation hiện tại là đủ.
Replay chỉ đặt disputed khi `EVIDENCE_MISMATCH` hoặc identity mismatch; HTTP
validation (`JSON_INVALID`), transport và cancel không đặt disputed.
`POLICY_PREVIEW_MISMATCH` có thông báo VI/EN riêng, hướng dẫn xem lại preview và
xác nhận lại; không map thành lỗi server generic. Đây là guard UI, không đổi wire
v1 hay hash projection.

## 7. Full-policy preview — implemented

Phục vụ [thiết kế sản phẩm](docs/design/product-ux.md), không thay nghĩa
wire v1 hiện tại. POST `/api/policy/normalize`:

- Body `{policy}` dùng `plenilune.policy/v1`; strict decoder, cap 8 MiB như repair.
- Response HTTP 200 `{policy, policy_sha256, obligations_digest}`: policy qua
  `repair.NormalizePolicy`, hash qua `repair.PolicyDigest`, condition digest qua
  `intent.Digest`. Không có verdict, receipt hay persisted approval mới.
- Caller gửi acceptance false để preview; sau thao tác duyệt rõ ràng mới gửi
  cùng definitions/scope với true để lấy frozen identity. Server giữ bool caller
  cung cấp, không tự nhận diện/đại diện người phê duyệt. UI so toàn bộ normalized
  policy sau khi đặt riêng operator_accepted=false ở cả preview/response (gồm
  schema/scope/conditions/allowed_changes, nil khác []); mọi field khác và input
  revision phải giữ nguyên trước khi publish acceptance. Không so hai policy hashes
  trực tiếp vì acceptance đổi làm hash đổi, không chỉ so condition digest.
- Nil/null/omitted allowed_changes giữ undeclared; explicit [] giữ zero changes.
  Invalid policy trả HTTP 400 `POLICY_INPUT_INVALID`; JSON/body errors giữ envelope
  hiện hữu. Không trả dữ liệu nhạy cảm trong error.
- Không cần source bytes cho normalization; UI bind kết quả với immutable input
  digests và revision của request. Endpoint không chứng nhận intent fidelity.

Compatibility: additive route, không thêm field vào strict v1 DTO hay thay hash
projection. CLI, compare, patch/repair/evidence v1 và replay cũ giữ nguyên. Migration
frontend gọi preview mới trước freeze; khi server cũ trả 404, cho khám phá/replay,
hiện cần backend hỗ trợ preview để duyệt theo luồng mới, không giả canonical scope.
Đã có focused tests nil-vs-empty, canonical fields/digests, strict decode và
late preview/acceptance guards. Browser QA luồng mới cần chạy riêng.

## 8. Configuration analysis — implemented 31/10/2026

Domain-neutral review layer around the domain engines. It does not replace ACL
comparison evidence; existing v1 evidence/replay is unchanged.

Schemas: `plenilune.analysis-request/v1` → `plenilune.analysis/v2`;
`plenilune.analysis-replay-request/v1` for one-document replay. The v2 bump
(P0-1, owner decision P0-D1) removes raw configuration text from the
artifact: `unanalyzed_changes` entries carry `header_kind` (at most the first
two header tokens), `lines` and `section_sha256` instead of the raw `header`,
and object `references` carry `line`, `kind` (referencing mechanism) and
`name` instead of the raw line `text`. `plenilune.analysis/v1` is frozen:
new analysis is never emitted as v1, but v1 artifacts remain replayable from
their original sources (replay recomputes the v1 shape). Request
`{schema,before:{text,ref?},after:{text,ref?},select?,policies?,options?}`;
`ref` is an opaque caller correlation label, not identity/authentication.
Options: ACL compare options, `max_classes` 1–128, `deadline_ms` 1–120000
(default 30000). Sources ≤4 MiB and ≤200000 lines each; unknown fields and wrong
types are rejected by the strict decoder.

Result `{schema,tool,versions,request,sources,objects,coverage,unanalyzed_changes,
summary,limitations,sha256}`. `sha256` is SHA-256 of compact JSON of the whole
artifact except `sha256`. Sources pin exact byte digests; no browser/UI state is
required to replay.

Objects are paired by `(domain,kind,name)` and carry `status`:
`changed|equivalent|unchanged|added|removed|unsupported|inconclusive|not_analyzed`.
Only `changed|equivalent|unchanged|inconclusive` have a behavior comparison; added
and removed objects are structural only. Up to 16 objects are behavior-analyzed
per request; the rest are `not_analyzed` with a reason. `unchanged` means
canonical object text is identical; it short-circuits an engine run only for that
reason.

Extents carry source line ranges, exact byte offsets for contiguous blocks, a
slice digest and units. ACL engine input is the exact block slice; a standalone
ACL document keeps the entire file as its block so old digests are preserved.
Prefix-list entries are non-contiguous; the artifact binds their digest and line
ranges and replay re-extracts them from the full sources.

Coverage counts analyzed, unsupported and unanalyzed lines per side plus
unanalyzed section kinds; `unanalyzed_changes` pairs sections by header text
but records only the redacted `header_kind`, the section line count and
`section_sha256` — for a modified section the digest binds the before text, a
NUL separator and the after text (capped, with a truncated flag). `limitations` always
includes per-object behavior without application binding and both domain scopes,
and adds unanalyzed/unsupported/added-removed qualifiers when present. No
unanalyzed change becomes a behavior verdict.

### ACL change classes

`plenilune.acl-changes/v1` is embedded in the ACL object (separate from evidence
v1). It groups every exact difference by the deciding before/after rule pair
(`ordinal|implicit`), with effect `newly_denied|newly_permitted`, one
independently re-evaluated witness packet, exact disjoint box examples (capped),
class count/truncation, and its own budget. If the class budget stops, `classes`
is empty and `complete=false`; partial explanations are never presented as full.
An operator-accepted policy (normalized, `operator_accepted=true`, bound by
`policy_sha256`) adds `scope: inside_allowed|outside_allowed|mixed` and an
outside witness verified against the policy predicate. No policy → `undeclared`.

### Prefix-list domain (IPv4)

Scope `ipv4-route-prefix/v1`, semantics profile
`ios-style-canonical-ipv4-prefix-list/v1`, identified by parser
`cisco-prefix-list/v0.2` + engine `prefix-list-exact/v0.2` in analysis versions.
See [normative subset and source disposition](docs/semantics/prefix-list-v02.md).
This is exact comparison of the declared model, not IOS/XE device qualification.

Grammar: `ip prefix-list NAME [seq N] permit|deny network/len [ge X] [le Y]`
and description. Canonical IPv4 prefixes, nonempty lists, increasing unique
sequences after source-order assignment; first match wins, implicit deny.
Current explicit/generated seq is 1..4294967294; source refs and semantic fields
must agree, including implicit sequences. Empty/description-only lists, host
bits, reserved names and unsupported syntax yield unsupported rather than a guess.
The model uses inclusive ranges: no qualifier=[len,len], ge=[ge,32], le=[len,le],
both=[ge,le]; specified ge/le exceed len, max32; ge==le means one length.
CLI acceptance on a target device/release remains unqualified, not silently
inferred from NX-OS/IOS-XR/FRR or ambiguous documentation formulas.

Complete reports count canonical route prefixes exactly (whole universe2^33-1),
not observed routes or application reachability. Class/range truncation is
explicit. Incomplete reports withhold classes, not claim zero changes. Intent/
repair and interface/neighbor application binding are not supported for this domain.

New analysis never falls back to v0.1. Frozen v0.1 prefix parser+engine are used
only when replaying a receipt explicitly pinning that supported version pair.
Mixed/unknown versions fail. The existing analysis schema/DTO is unchanged.

Timing-sensitive analysis replay still binds exact sources/metadata, request/
policy, inventory/coverage, prefix reports and unaffected ACL outputs. Original
ACL point evidence and necessary scope membership are rechecked before excluding
only the timing-affected computation. TIMING_SENSITIVE withholds full proof/
region matching and never upgrades the original verdict or authenticates producer.

### API and CLI

- `POST /api/analyze` body = request; HTTP 200 artifact (any object status),
  HTTP 400 typed `ANALYSIS_*|REQUEST_*|OPTIONS_*|POLICY_*|INPUT_*` error.
- `POST /api/analyze/replay` `{before,after,evidence}` → HTTP 200
  `{status,sha256,rechecked_witnesses}`; `ANALYSIS_REPLAYED` or
  `TIMING_SENSITIVE`. Source/digest/version/result mismatch → HTTP 400.
- CLI `analyze --before FILE --after FILE [--select domain:name,...]` or
  `analyze --request FILE|-`; exit 0 any artifact, 2 error.
- CLI `analyze-replay --before FILE --after FILE --evidence FILE` or
  `--request FILE|-` (`plenilune.analysis-replay-request/v1`); exit 0 replayed,
  2 TIMING_SENSITIVE/error.
- `--request -` reads decrypted in-memory text over stdin so an integrating caller
  need not write it to disk. Plenilune performs no auth/identity/artifact fetch.

### Engine versions

New ACL evidence is `acl-exact/v0.3`; new repair receipts `acl-repair/v0.3`.
v0.3 aligns the longest common prefix of semantically identical rules and
computes the exact delta only over the residual rules, on the complement of
the prefix's covered region; obligations are evaluated against a permit
cover of just the rules that can touch the condition region. It produces
the same verdicts, change classes and obligation outcomes as v0.2
(`TestP11DifferentialV03AgainstV02`, box-for-box delta tests) but different
logical operation/box accounting, so version is part of evidence.
v0.2 indexes candidate cuts and decides fragments against raw rules in first-match
order, producing the same boxes/verdicts as v0.1 with its own accounting.
`acl-exact/v0.2`/`acl-exact/v0.1` and `acl-repair/v0.2`/`acl-repair/v0.1`
remain supported for replay with their original accounting; SealComparison
preserves a caller-supplied supported version.
