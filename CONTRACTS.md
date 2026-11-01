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
| `repair-enumerate --before FILE --after FILE [--policy FILE] [--proposals FILE]` | Original pair + optional frozen policy + optional proposals | Enumeration record đã seal (xem §6); 0 có candidate VERIFIED / 1 không có / 2 incomplete/error |
| `audit --config FILE [--max-operations N] [--max-boxes N] [--timeout D]` | Một ACL | Audit record đã seal (xem §9); 0 complete / 2 incomplete/error; budget flags chỉ được hạ thấp default |
| `audit-replay --config FILE --evidence FILE` | ACL + audit record | AUDIT_REPLAYED hoặc TIMING_SENSITIVE; 0 replayed / 2 còn lại |
| `assist --before FILE --after FILE --intent TEXT --ai off\|on [--policy FILE]` | Intent + optional frozen policy | Session sidecar + deterministic receipts; AI defaults off |

Exit 0: operation thành công, kể cả packet bị deny. Exit 2: usage/input,
unsupported, cancellation, I/O hoặc replay mismatch. JSON result ở stdout;
JSON error theo taxonomy P2-3 (mục 4): `code`, `stage`, `message`, `retryable`, `field` khi áp dụng, source `line`/`side` khi biết ở stderr. Không có output
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

### Capabilities — implemented (P2-2)

Schema `plenilune.capabilities/v1`. Một builder duy nhất
(`internal/capabilities.Build`) sinh document; cả CLI
(`plenilune capabilities [--json]`, JSON là dạng duy nhất như `version`) và
`GET /api/capabilities` đều phục vụ đúng document đó — caller subprocess và
caller HTTP gate trên cùng một sự thật. Document gồm: `tool` (name, version,
commit theo VCS stamp của build, `unversioned` khi không có), `schemas`
(id, current, supported — gồm cả version đã đóng băng còn replay được),
`domains` (id, parser version, engine version), `limits` (mặc định + trần
lấy từ constants/defaults của package sở hữu: packetset, parser, analysis,
AI session), `options` (bề mặt options của analysis/compare kèm default),
`features`, `error_codes` (các typed code caller có thể nhận; status/outcome
không phải error code và không nằm trong danh sách).

Quy tắc ổn định trong v1: chỉ cộng thêm — field, phần tử list và version hỗ
trợ có thể thêm; không xóa, không đổi tên, không đổi nghĩa. Đổi phá vỡ phải
bump schema capabilities. Mọi giá trị suy từ constants/defaults của code tại
thời điểm build; hai list curated duy nhất là `features` và `error_codes`,
được contract test đối chiếu với code (mỗi error code phải thật sự được phát
ra ở non-test source; snapshot test ghim toàn bộ JSON). Caller kiểm
capabilities TRƯỚC khi gửi cấu hình và từ chối sớm kèm lý do khi version/
feature không khớp — client mẫu `examples/moonstorm-client/` (Go, stdlib)
làm đúng việc này và là điểm mở rộng cho vòng analyze của P2-7.

### Error taxonomy — implemented (P2-3)

Mọi lỗi qua CLI/HTTP mang cùng một envelope: `code`, `stage`, `message`,
`retryable`, cộng `field` khi biết field gây lỗi và `line`/`side` khi biết
vị trí nguồn. CLI ghi envelope phẳng ở stderr; HTTP bọc trong
`{"error": ...}`; riêng `diff --brief` giữ định dạng lỗi một dòng
`error: CODE message` đã ghim từ P1-9 (không phải JSON). Caller map theo
code/stage — không parse message text. Exit code và HTTP status không đổi
so với trước P2-3; thay đổi này thuần cộng thêm field.

`stage` ∈ `request | parse | validate | engine | budget | replay |
provider`. `retryable=true` nghĩa là gửi lại đúng request đó có thể thành
công — chỉ dành cho điều kiện thoáng qua (quá tải, hủy, deadline theo
wall-clock, lỗi đọc/ghi I/O, replay phụ thuộc timing); mọi lỗi còn lại là
`false` vì gửi lại nguyên trạng sẽ fail y hệt. Registry duy nhất nằm ở
`internal/errtax` và bằng đúng `error_codes` của capabilities (test khóa
cả hai chiều); bảng dưới đây là bản in của registry đó. Code không bao giờ
đổi tên trong v1 — thêm code mới phải đăng ký ở errtax VÀ capabilities
trong cùng một thay đổi.

| Code | Stage | Retryable | Field/line |
|---|---|---|---|
| CANCELED | request | true | |
| CONTENT_TYPE | request | false | |
| INPUT_FAILED | request | false | |
| INPUT_READ | request | true | |
| INPUT_TOO_LARGE | request | false | |
| INVALID_JSON | request | false | |
| JSON_INVALID | request | false | |
| LOCAL_ORIGIN_REQUIRED | request | false | |
| OUTPUT_FAILED | request | true | |
| SERVER_BUSY | request | true | |
| UI_NOT_BUILT | request | false | |
| USAGE | request | false | |
| ACL_INVALID | parse | false | line |
| DUPLICATE_DEFINITION | parse | false | line |
| INPUT_INVALID | parse | false | side, line khi có |
| PARSE_DOCUMENT | parse | false | line |
| PARSE_EMPTY_ACL | parse | false | line |
| PARSE_ENCODING | parse | false | line |
| PARSE_FAILED | parse | false | side, line khi có |
| PARSE_HEADER | parse | false | line |
| PARSE_INVALID_IP | parse | false | line |
| PARSE_INVALID_PORT | parse | false | line |
| PARSE_INVALID_TOKEN | parse | false | line |
| PARSE_LIMIT | parse | false | line |
| PARSE_MULTIPLE_ACLS | parse | false | line |
| PARSE_NOT_FOUND | parse | false | line |
| PARSE_SEQUENCE_ORDER | parse | false | line |
| ROUTE_INVALID | parse | false | line |
| AUDIT_INPUT_INVALID | validate | false | |
| BUNDLE_FILE_MISSING | validate | false | field: role |
| BUNDLE_INVALID | validate | false | field: role khi có |
| CHAT_INVALID | validate | false | |
| CHAT_LOCALE_UNSUPPORTED | validate | false | field: locale |
| CHAT_NO_ARTIFACT | validate | false | |
| CHAT_QUESTION_EMPTY | validate | false | field: question |
| CHAT_QUESTION_TOO_LONG | validate | false | field: question |
| COMPARISON_INVALID | validate | false | |
| INBOX_INVALID | validate | false | field: id |
| INPUT_ENCODING | validate | false | |
| INVALID_OBLIGATION | validate | false | |
| INVALID_PACKET | validate | false | |
| INVALID_PATCH | validate | false | |
| INVALID_POLICY | validate | false | |
| INVALID_PROPOSALS | validate | false | |
| OBLIGATIONS_INVALID | validate | false | |
| OPTIONS_INVALID | validate | false | field: options |
| PACKET_INVALID | validate | false | |
| POLICY_INPUT_INVALID | validate | false | |
| POLICY_INVALID | validate | false | field: policies |
| POLICY_NOT_ACCEPTED | validate | false | field: policies |
| REPAIR_INPUT_INVALID | validate | false | |
| REQUEST_LIMIT | validate | false | |
| REQUEST_SCHEMA | validate | false | field: schema |
| RESULT_TOO_LARGE | validate | false | |
| SELECT_INVALID | validate | false | field: select |
| SESSION_INPUT_INVALID | validate | false | |
| SOURCE_REF_INVALID | validate | false | field: ref |
| UNSUPPORTED_KIND | validate | false | |
| WATCHES_INVALID | validate | false | field: watches |
| ANALYSIS_FAILED | engine | false | |
| COMPARISON_FAILED | engine | false | |
| EVIDENCE_FAILED | engine | false | |
| TRACE_INVALID | engine | false | |
| DEADLINE_EXCEEDED | budget | true | |
| BUNDLE_DIGEST_MISMATCH | replay | false | field: role |
| CHAT_ARTIFACT_INVALID | replay | false | |
| EVIDENCE_MISMATCH | replay | false | |
| TIMING_SENSITIVE | replay | true | (xem ghi chú) |
| AI_CONFIG_INVALID | provider | false | |
| AI_MODEL_UNSUPPORTED | provider | false | |

Ghi chú phân tầng:

- Code chi tiết của parser ACL (`PARSE_*`, `ACL_INVALID`, `ROUTE_INVALID`,
  `PACKET_INVALID`) được bề mặt parse/trace phát nguyên trạng (kèm line)
  vì các bề mặt đó trả chính lỗi parser. Trên đường compare/analysis, cùng
  lỗi đó được gói thành `PARSE_FAILED` (workflow) hoặc nằm trong artifact
  dưới dạng lỗi theo object (analysis intake) — chi tiết parser prefix-list
  cũng vậy, không phát thành error code ở biên.
- Code chi tiết của package trong (patch `PATCH_*`, agent `invalid_input`)
  không bao giờ ra biên nguyên trạng: bề mặt gói thành `INVALID_PATCH` /
  `REPAIR_INPUT_INVALID` / `SESSION_INPUT_INVALID`. Registry chỉ chứa code
  thật sự ra biên; `errtax.FromError` từ chối adapt code chưa đăng ký để
  việc gói đó không bị rò.
- `TIMING_SENSITIVE` là *status* của replay (không phải error): nằm trong
  `error_codes` và registry để caller có một bảng tra duy nhất; retryable
  vì replay lại dưới tải khác có thể kết luận trọn vẹn.
- Ngân sách/hủy không bao giờ là thành công một phần không đánh dấu:
  budget dừng → verdict INCONCLUSIVE kèm `search_complete=false` (hoặc
  `partial` đã seal); deadline rơi giữa analysis → object `not_analyzed`
  trong artifact đã seal; deadline đã qua trước khi bắt đầu / hủy →
  error typed `DEADLINE_EXCEEDED` / `CANCELED`, không có artifact.

### Review bundle — implemented (P2-4)

Schema `plenilune.bundle/v1`: một review case được xuất thành một thư
mục thường (không ZIP/archive) gồm các tệp của case và một tệp chỉ mục
`bundle-manifest.json`. Manifest là *integrity index* thuần túy: mỗi
entry ghi `role`, `path` (đường dẫn tương đối trong thư mục bundle) và
`sha256` của đúng byte của tệp đó. Manifest không mang verdict, không
thay receipt (receipt trong các tệp mới là bằng chứng — digest của
chúng, gồm cặp `content_sha256`/`artifact_sha256` của P2-1, là thứ neo
kết quả replay), và report trong bundle vẫn chỉ là view cho người đọc.

Role và tên tệp canonical khi export: `before` → `before.acl`,
`after` → `after.acl`, `comparison` → `comparison.json` (bắt buộc cả
ba); `repair` → `repair.json`, `report` → `report.md`,
`instructions` → `instructions.md` (tùy chọn khi export). Quy tắc toàn
vẹn khi load: tệp đã được liệt kê trong manifest — kể cả role tùy
chọn — là bắt buộc phải có mặt và khớp digest; thiếu tệp →
`BUNDLE_FILE_MISSING`, lệch một byte → `BUNDLE_DIGEST_MISMATCH`, cả
hai đều mang tên role trong `field`. Manifest bị từ chối bằng
`BUNDLE_INVALID` nếu: sai schema, role lạ, trùng role hoặc trùng
path, thiếu entry bắt buộc, entry không theo thứ tự role canonical,
digest không đúng định dạng hex, hoặc path không phải đường dẫn tương
đối sạch nằm trong thư mục bundle (chặn tuyệt đối, `..`, `\`, tiền tố
ổ đĩa và chính tệp manifest). Manifest **không tự hash**: hash của chỉ
mục không neo được gì (ai sửa tệp cũng sửa được chỉ mục); neo thật là
các digest đã seal bên trong artifact, được chính replay kiểm lại.

Builder duy nhất nằm ở `internal/bundle` (thuần, không I/O); CLI và
demo server dùng chung nên manifest của hai đường xuất là byte-identical
cho cùng một case. Đường CLI: `plenilune bundle-export --out DIR
--before FILE --after FILE --comparison FILE [--repair FILE]
[--report FILE] [--instructions FILE]` ghi các tệp (manifest ghi sau
cùng) và in manifest; `plenilune bundle-replay --dir DIR` xác minh
toàn vẹn trước, rồi chuyển artifact đã xác minh cho replay hiện hữu —
role `comparison` chứa evidence `plenilune.evidence/v1|v2` →
`workflow.Replay`, chứa analysis `plenilune.analysis/v1|v2|v3` →
`analysis.Replay`, và `repair.json` nếu có → `repair.Replay`; exit 0
chỉ khi mọi artifact có mặt đều replay khớp, 2 khi lỗi toàn vẹn hoặc
replay timing-sensitive. Đường web: `POST /api/bundle/export` nhận các
text artifact của case và trả về đúng byte manifest canonical; nút
xuất ở màn hình evidence tải `bundle-manifest.json` cùng các tệp case
vốn đã tải riêng lẻ.

### Trigger & hộp thư verdict — implemented (P2-8)

Tầng 0 của luồng nhanh. Người dùng ghim (watch) các đối tượng cần
canh theo `(domain, name)` — `domain` là `acl` hoặc `prefix-list`,
vắng mặt mặc định `acl` — kèm tập điều kiện kích hoạt:
`verdict_changed`, `any_different`, `obligation_failed`. Mỗi khi một
lượt phân tích (`POST /api/analyze`) hoặc so sánh (`POST /api/compare`)
hoàn tất trong server, kết quả của từng đối tượng có verdict engine
được đối chiếu với kết quả đã ghi gần nhất của chính đối tượng đó;
mỗi điều kiện khớp sinh một mục hộp thư verdict (inbox entry).
Đánh giá là đồng bộ tại thời điểm hoàn tất, trong tiến trình server:
không daemon, không scheduler, không watcher file.

Ngữ nghĩa từng điều kiện:

- `verdict_changed`: chỉ bắn khi cả kết quả trước và kết quả hiện tại
  đều là verdict đã quyết định (EQUIVALENT/DIFFERENT) và khác nhau;
  entry mang `previous_verdict` và `verdict`. INCONCLUSIVE — kể cả
  kết quả một phần dừng vì ngân sách, mà verdict của nó vẫn là
  INCONCLUSIVE — được ghi thành trạng thái của đối tượng nhưng không
  bao giờ là một phía của "verdict changed": cạn ngân sách là sự kiện
  của lượt tìm kiếm, không phải của cấu hình, và không được nâng
  thành thay đổi hành vi.
- `any_different`: bắn khi kết quả hiện tại là DIFFERENT đã quyết
  định và artifact sinh ra nó (theo `content_sha256`) chưa từng được
  báo cho đối tượng này; cùng một artifact gặp lại (compare cache
  hit, chạy lại cùng input) không bắn hai lần.
- `obligation_failed`: chỉ đường compare mới đánh giá obligation.
  Bắn khi tập obligation VIOLATED xuất hiện ID chưa vi phạm trong
  kết quả đã ghi gần nhất; entry mang đúng các ID mới vi phạm.
  Vi phạm cũ còn nguyên không bắn lại, và một lượt phân tích (không
  đánh giá obligation) không xóa tập vi phạm đã ghi.

Entry mang: đối tượng (domain + name), điều kiện đã bắn, verdict
hiện tại, `violated_obligations` khi áp dụng, `artifact_kind`
(`analysis` hoặc `comparison`), cặp digest (`content_sha256`,
`sha256`) của artifact sinh ra nó, thời điểm server ghi và cờ đã
đọc. `stale` được tính lúc đọc: true khi đối tượng đã có kết quả
mới hơn kể từ entry đó — đây là đánh dấu hiển thị, không phải cơ
chế vô hiệu hóa quyết định (việc đó thuộc P3-4). Hộp thư không chứa
verdict nào ngoài verdict engine đã ghi trong artifact mà entry
trỏ tới; mở entry là mở artifact đó.

Bề mặt API (mọi route sau guard `localOnly` như mọi `/api/*`):
`GET /api/watches`; `PUT /api/watches` thay toàn bộ danh sách (lỗi
định dạng → `WATCHES_INVALID`, stage validate, field `watches`);
`GET /api/inbox` trả entries mới nhất trước kèm số chưa đọc;
`POST /api/inbox/read` nhận `{id}` hoặc `{all: true}` (id lạ →
`INBOX_INVALID`, field `id`). Toàn bộ trạng thái — watches, kết quả
đã ghi theo đối tượng, entries — sống trong bộ nhớ tiến trình
server như compare cache (P1-5), giới hạn 64 watches, 256 entries
(rớt cũ nhất) và 512 đối tượng có trạng thái; khởi động lại server
là bắt đầu trống. Không ghi đĩa; không thông báo ra ngoài app
(phía MoonStorm của trigger là đặc tả ở P2-7). Engine nằm ở
`internal/inbox`; capabilities công bố feature `verdict_inbox`.

## 5. Full comparison và API — implemented

Schema `plenilune.evidence/v2` (P2-1; v1 đóng băng, xem đoạn dual digest dưới đây),
không đổi nghĩa trace v1. DTO `evidence.Comparison`
gồm schema/tool/parser_version/engine_version/scope/before/after/conditions/report/content_sha256/sha256.
Before/after pin raw bytes digest + ACL name/kind; conditions là normalized Set hoặc null.
Report gồm verdict, search_complete, reason, witnesses, witnesses_truncated,
obligations, budget, options và partial (chỉ xuất hiện khi engine v0.4+ bị
budget dừng — xem đoạn partial dưới đây). Mỗi witness có ID, packet, before/after decision+source.
Global verdict và từng obligation có outcome riêng.
Obligation VIOLATED có witness riêng, không buộc phải trỏ global difference.
Lệnh `diff`: 0=EQUIVALENT, 1=DIFFERENT, 2=INCONCLUSIVE/error; unsupported input trả typed error exit 2.
Chế độ `diff --brief` (bắt buộc kèm `--out FILE`; `--out` không có `--brief` bị từ chối): artifact
không đổi — cùng bytes như output JSON mặc định — nhưng ghi vào FILE do caller đặt tên, còn stdout
chỉ gồm 5 dòng ổn định để grep: `verdict:` (INCONCLUSIVE luôn kèm `reason=` stop reason;
kết quả partial của v0.4 kèm `partial=true stage=<stage> prefix_pairs=<k>
decided_pairs=<n>` ngay trên dòng verdict, kể cả khi verdict là DIFFERENT),
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

Dual digest (P2-1, từ evidence v2): artifact mang hai digest. `content_sha256` =
SHA-256 compact JSON của projection ngữ nghĩa cố định gồm schema, parser_version,
engine_version, scope, before, after, conditions và report (options nằm trong report) —
tức inputs, definitions, scope, versions của parser/engine, options và results.
KHÔNG gồm build provenance: tool (version/commit), và không gồm chính hai digest.
`sha256` giữ nguyên vai trò cũ và chính là artifact digest: SHA-256 compact JSON của
toàn envelope trừ trường sha256 — gồm cả provenance và cả content_sha256 — nên nó định
danh đúng artifact đó. Hệ quả: cùng input/options, hai build khác commit cho
content_sha256 bằng nhau, sha256 khác nhau; caller dedup/cache/idempotency theo
content_sha256, kiểm integrity của một artifact cụ thể theo sha256. Kiểm chứng
(`CheckComparison`) tính lại CẢ HAI digest từ payload: sửa payload rồi tính lại envelope
trên một content claim giả vẫn bị từ chối vì content được tính lại chứ không được tin.
Artifact v1 đóng băng: chỉ có sha256, kiểm theo đúng quy tắc cũ (envelope digest, không
được mang content_sha256); replay dispatch theo schema của chính artifact. Repair receipt
nhúng comparison: khi replay, comparison được tính lại theo schema evidence đã ghi trong
receipt (`SealComparisonAs`/`CompareEngineSchema`) để receipt thời v1 giữ nguyên byte.
Kết quả replay (`workflow.ReplayResult`) mang thêm `content_sha256` khi artifact là v2.

Partial results (P1-2, engine `acl-exact/v0.4`): field cộng thêm, không bump
schema — evidence v1 và analysis v2 giữ nguyên, artifact cũ replay nguyên bytes.
Khi budget dừng search, `partial` ghi lại stage đã dừng (`prefix|permit_before|
permit_after|decide_lost|decide_gained|witnesses|scope_index|classes`), vị trí
rule đang xử lý, `prefix_pairs` (số cặp rule đầu đã chứng minh identical từng cặp),
`decided_pairs` (các cặp rule đã quyết định khác nhau kèm số mảnh; witness tương
ứng vẫn là witness evaluator-checked như thường) và `undecided` (phần chưa quyết
định, bị chặn bằng khoảng ordinal rule theo hướng `delta|lost|gained|classes`,
không suy diễn tương đương hay khác biệt trong vùng đó). Verdict vẫn có thể là
DIFFERENT nếu đã có witness chứng minh, nhưng `search_complete=false` và
`partial` luôn đi kèm nên không thể đọc như verdict đầy đủ. Delta đã dừng thì
mọi obligation đều INCONCLUSIVE với stop reason của comparison. Dừng ở stage
obligation (delta đã xong) giữ nguyên hành vi cũ, không có `partial`.

Localhost API: POST `/api/compare` nhận `{before,after,conditions,options?}`;
options omitted dùng defaults. POST `/api/compare/replay` nhận `{before,after,evidence}`.
Domain verdict hoặc replay status nằm trong JSON HTTP 200; input/error HTTP 400.
Body cap lần lượt 8 MiB/12 MiB; source vẫn chịu cap 2 MiB mỗi bên.
Options: limits `{max_operations,max_boxes,timeout_ns}` và max_witnesses; chỉ cho hạ
cap defaults 2M/50k/2s, max_witnesses 1–64 (default 8). UI gọi trực tiếp endpoint
này; sample input được dán nhãn, còn comparison hiển thị là kết quả runtime. Màn
hình có thể hạ budget cho một sample minh hoạ; retry mặc định gửi options omitted.

Chat thẩm vấn (P1-10): POST `/api/chat/ask` nhận `{comparison?, analysis?, question, locale?, ai?}`
— ít nhất một artifact; body cap 4 MiB (comparison) + 16 MiB (analysis) + 1 MiB. Request không có
trường nào chứa configuration text nên endpoint không thể chạy comparison/analysis/repair hay
accept obligation; decode strict từ chối field lạ (HTTP 400 `JSON_INVALID`). Artifact được kiểm seal
trước khi trả lời (comparison qua `evidence.CheckComparison`; analysis qua re-seal và schema v1/v2);
lệch digest trả `CHAT_ARTIFACT_INVALID`, thiếu artifact trả `CHAT_NO_ARTIFACT`. Question tối đa 1000 rune
(`CHAT_QUESTION_TOO_LONG`); locale `en|vi`, mặc định `en`. Response `plenilune.chat/v1` gồm `class`,
`text` (tất định, dựng hoàn toàn từ dữ liệu artifact), `facts[]` — mỗi fact có `label`/`value`/`source`
là đường dẫn field trong artifact — và `artifact_sha256` neo vào digest của artifact được hỏi
(comparison khi có cả hai). Endpoint đi qua localOnly guard và concurrency cap như các route nặng
khác (P0-2); chat không ghi vào artifact store và không có đường nào đổi verdict.
AI phrasing (opt-in bằng `ai: true`): đúng một provider request, không tool, model khóa
`muse-spark-1.3-contributor`; model chỉ diễn đạt lại answer + facts của core tất định, không tự truy
xuất artifact. Response mang `ai.status` (`off|ok|not_configured|model_unsupported|unavailable|error`);
mọi status khác `ok` vẫn trả answer tất định đầy đủ với HTTP 200 — AI chưa cấu hình không làm hỏng màn
verdict. UI luôn hiển thị facts (và answer tất định khi có phrasing) cạnh bản diễn đạt: artifact thắng
khi hai bên mâu thuẫn.

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
Apply atomically trong memory, parse/verify lại; số proposal sửa chữa model được nộp trong một AI session bị chặn bởi ngân sách proposal của session (mặc định 6, trần cấu hình 16). Mỗi candidate phải kiểm
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

Enumeration deterministic (P1-3) `plenilune.repair-enumeration/v1`, scheme
`repair-enumeration/v1`: từ change classes (vùng box exact đầy đủ, không chỉ
example) và obligation violations (vùng vi phạm exact tính bằng permit-cover
như engine), sinh candidate hữu hạn bằng 5 phép biến đổi có giới hạn (xóa rule
quyết định; đảo action rule quyết định; chèn rule ngoại lệ đúng vùng vi phạm
trước rule quyết định; reorder remove+add cho ca hai rule bị hoán đổi; thu hẹp
predicate về `rule \ vùng vi phạm` khi hiệu là đúng một box biểu diễn được).
Mỗi candidate là patch v1 và qua đúng `Verify` ở trên — enumeration không có
đường tắt nào quanh obligations/collateral/acceptance. Target và candidate bị
chặn (32 target, 64 candidate sinh, 8 box/rule quyết định mỗi target, deadline
của caller); analysis dừng budget thì record ghi `complete=false` + stop reason
và không có candidate. Thứ tự hiển thị tất định theo (số edit, số target giải
quyết, hash candidate) — chỉ là thứ tự trình bày, không phải tuyên bố "best"
hay "safest"; số target giải quyết tính từ receipt của chính candidate
(obligation vi phạm thành HOLDS; class target tính trọn khi collateral
VERIFIED). Proposal từ ngoài (kể cả AI) vào cùng khung qua cùng dedup theo
digest của request, cùng Verify, cùng thứ tự — nguồn proposal không tạo lợi
thế thứ hạng. Record tự seal bằng SHA-256 (nhãn `sensitive` bị loại khỏi
digest như receipt, vì record nhúng candidate text); replay chạy lại
enumeration từ input gốc + proposals đã ghi và đòi cùng seal, lệch thuần
timing trả TIMING_SENSITIVE. Enumeration không đổi format receipt: receipt
nó cấp vẫn là `plenilune.repair/v1` version hiện hành và replay bằng đường
repair replay thường.

API: POST `/api/repair/verify` `{before,after,policy,patch,options?}` cap 8 MiB;
POST `/api/repair/replay` `{before,after,evidence}` cap 16 MiB. Domain statuses HTTP200;
invalid input/error HTTP400. POST `/api/agent/session` nhận
`{before,after,intent,policy,enabled,options?}` cap 8 MiB. Enabled defaults false;
true mới đọc key/config từ server env, yêu cầu Host/Origin local, không nhận key từ browser.
Session `plenilune.session/v1` gồm comparison, ai (`plenilune.ai/v1`) và repairs[];
AI/provenance là sidecar. Missing key -> not_configured cùng comparison thật.
Ngân sách AI session (P1-7, chỉ tầng đề xuất — verdict/acceptance không có knob ngân sách): mặc định tối đa
16 model requests (gồm retry), 24 local tool invocations, 6 repair proposals, 2 output corrections và deadline 120s;
trần validation tương ứng 32/64/16/8. Mọi cap là giới hạn cứng: hết proposal thì `verify_patch` trả
`repair_budget_exhausted` và engine không được gọi thêm. Provenance (`plenilune.ai/v1`) ghi `budgets` — các cap hiệu lực —
cạnh lượng đã dùng (`requests`, `tool_calls`, `repair_proposals`), nên record của session tự cho thấy giới hạn nó chạy dưới;
budgets là configuration, không vào hash của verdict/receipt nào. Model vẫn khóa `muse-spark-1.3-contributor`, không fallback.
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

Schemas: `plenilune.analysis-request/v1` → `plenilune.analysis/v3` (P2-1 dual digest; v2 đóng băng, xem dưới);
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
(default 30000), `max_analyzed` 1–64 (default 16; an absent field means the
default, which is how artifacts recorded before P1-5 carry it). Sources ≤4 MiB
and ≤200000 lines each; unknown fields and wrong types are rejected by the
strict decoder.

Result `{schema,tool,versions,request,sources,objects,coverage,unanalyzed_changes,
summary,limitations,content_sha256,sha256}`. `sha256` is SHA-256 of compact JSON of the whole
artifact except `sha256`. Sources pin exact byte digests; no browser/UI state is
required to replay.

Dual digest (P2-1, from analysis v3): `content_sha256` is SHA-256 of compact JSON
of the semantic projection — schema, versions, the request echo (selection,
policy definitions, options), the sources by content (byte digest, byte and line
counts — NOT the caller `ref` labels, which are correlation metadata), objects,
coverage, unanalyzed changes (+truncated flag), summary, the global-budget record
and limitations. Tool provenance is excluded, so the same result from a different
build — or with different `ref` labels — keeps one content identity; `sha256`
remains the whole-envelope (artifact) digest over every field, provenance and the
content digest included. Replay verifies BOTH digests against the payload before
recomputing (a forged content claim under a consistently recomputed envelope is
rejected), and the replay result surfaces both. Frozen v1/v2 artifacts carry only
the envelope digest and replay in their own shape under the original rules; a
frozen shape carrying a content digest is rejected. In the TIMING_SENSITIVE
projection both digests are excluded, like-for-like with the envelope digest,
because the content payload contains the same timing-dependent budget outcomes.

Objects are paired by `(domain,kind,name)` and carry `status`:
`changed|equivalent|unchanged|added|removed|unsupported|inconclusive|not_analyzed`.
Only `changed|equivalent|unchanged|inconclusive` have a behavior comparison; added
and removed objects are structural only. Up to `max_analyzed` objects (default
16) are behavior-analyzed per request; the rest are `not_analyzed` with a
reason. `unchanged` means
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

### Parallel execution, global budget, deadline partials

Execution is two-phase (P1-5). Classification (pairing, extents, references,
structural statuses, the `max_analyzed` slot assignment) is sequential in
canonical order; engine evaluation then runs on a bounded worker pool
(default `min(4, GOMAXPROCS)`; CLI `--workers`; library `AnalyzeExec`). The
worker count is an execution detail only: results are assembled in canonical
order and the artifact — digest included — is byte-identical for any worker
count and any GOMAXPROCS.

One global budget ledger applies to the whole run, in addition to the
per-object budgets: the caps are the request's ACL limits (`max_operations`,
`max_boxes`). Objects are admitted in canonical order and charged, per
recorded object, the sum of their compare and changes operations and the
larger of the two stages' peak boxes (an object's stages run in sequence, so
that is its live-box footprint; prefix-list comparisons charge their
operations counter only, they use no box algebra). While an object's charge
fits the remainder it is recorded exactly as computed — the clamping a
sequential run would apply never binds for it. The first object that does
not fit is recomputed with the remainder as its stage limits, so its
recorded result, budget-stopped partials included, is the exact sequential
result under the shared budget; an object whose stages cannot start under
the remainder is `not_analyzed` with reason `global_budget`, and so is every
later object. When the ledger constrains anything, the artifact carries a
`global_budget` record (the caps plus the totals charged by the recorded
objects) and the `global_budget_exhausted` limitation. When it never binds,
neither appears and unconstrained artifacts are byte-identical to pre-P1-5
artifacts; pre-P1-5 artifacts replay unchanged.

A request deadline or caller cancellation that lands while objects are
being evaluated no longer aborts the analysis. Objects that did not finish
are `not_analyzed` with reason `deadline_exceeded` or `canceled` and carry
no engine payload; the artifact gains the `analysis_deadline_exceeded` /
`analysis_canceled` limitation and stays sealed. (A context already
cancelled before the run starts remains an error.) Which objects a deadline
cut is timing-dependent, so such artifacts replay as `TIMING_SENSITIVE` —
the completed objects' point evidence is still rechecked exactly — never
as a full replay and never silently upgraded.

The demo server memoizes sealed comparisons (`internal/workflow`
`CompareCache`, process scope, FIFO-bounded) keyed by the input-side
content projection (P2-1): the digests of both inputs plus the conditions
as supplied, the full options, and the parser and engine versions — exactly
the inputs that determine the comparison's `content_sha256`. Build
provenance is not keyed: neither the tool commit (excluded from content
identity by P2-1) nor the tool version (a release label; semantics are
pinned by the parser/engine versions and the version is validated against
the producing build at seal time). A hit returns the artifact exactly as
the producing computation sealed it, provenance included; callers identify
it by its content digest. Reopening an already-compared object
does not recompute it. A hit is the exact artifact a fresh computation
produced for those inputs; timing-limited results are never stored; the
cache is an internal optimization only and evidence replays independently
of it.

### ACL change classes

`plenilune.acl-changes/v1` is embedded in the ACL object (separate from evidence
v1). It groups every exact difference by the deciding before/after rule pair
(`ordinal|implicit`), with effect `newly_denied|newly_permitted`, one
independently re-evaluated witness packet, exact disjoint box examples (capped),
class count/truncation, and its own budget. If the class budget stops,
`complete=false`; engines before v0.4 then withhold classes entirely. From
v0.4 the report may keep the classes decided before the stop and adds a
`partial` explanation (stage, decided/undecided rule pairs with fragment
counts, bounded undecided rule ranges): the emitted classes are exact, and
the partial block — never the class list alone — bounds the claim. The ACL
object likewise carries the comparison's split as `acl.partial` next to its
verdict; the object status vocabulary is unchanged (a stopped ACL object is
still `inconclusive`, a proven-different one still `changed`).
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
- CLI `analyze --before FILE --after FILE [--select domain:name,...]
  [--workers N]` or `analyze --request FILE|-`; `--workers` 0 = default
  `min(4, GOMAXPROCS)`, explicit range 1–64; exit 0 any artifact, 2 error.
- CLI `analyze-replay --before FILE --after FILE --evidence FILE` or
  `--request FILE|-` (`plenilune.analysis-replay-request/v1`); exit 0 replayed,
  2 TIMING_SENSITIVE/error.
- `--request -` reads decrypted in-memory text over stdin so an integrating caller
  need not write it to disk. Plenilune performs no auth/identity/artifact fetch.

### Engine versions

New ACL evidence is `acl-exact/v0.4`; new repair receipts `acl-repair/v0.4`.
v0.4 is the v0.3 construction plus partial results (P1-2, see §5): identical
verdicts, change classes, obligation outcomes and logical budget accounting
on every completing run (`TestP12DifferentialV04AgainstV03`); only a budget
stop changes shape, returning the proven/undecided split instead of
discarding the search.
v0.3 aligns the longest common prefix of semantically identical rules and
computes the exact delta only over the residual rules, on the complement of
the prefix's covered region; obligations are evaluated against a permit
cover of just the rules that can touch the condition region. It produces
the same verdicts, change classes and obligation outcomes as v0.2
(`TestP11DifferentialV03AgainstV02`, box-for-box delta tests) but different
logical operation/box accounting, so version is part of evidence.
v0.2 indexes candidate cuts and decides fragments against raw rules in first-match
order, producing the same boxes/verdicts as v0.1 with its own accounting.
`acl-exact/v0.3`/`acl-exact/v0.2`/`acl-exact/v0.1` and
`acl-repair/v0.3`/`acl-repair/v0.2`/`acl-repair/v0.1`
remain supported for replay with their original accounting; SealComparison
preserves a caller-supplied supported version.

## 9. Single-ACL audit — implemented (P1-4)

Exact, report-only audit of ONE named extended IPv4 ACL (the comparison
engine's modeled scope). The audit never modifies the configuration and
offers no removal or auto-fix path. The record is
`plenilune.audit/v1` (`audit_version` `acl-audit/v1`): it binds the
configuration digest, ACL name/kind, comparison engine version and modeled
scope, embeds the full audit report, and seals with `sha256` over the
compact JSON of everything except `sha256`. The record carries no raw
configuration text, line text or remarks — only the digest.

Definitions (rule `i` matches region `R_i`; its effective region is
`E_i = R_i \ union(R_1..R_{i-1})`; decisions are first-match with an
implicit final deny):

- `unreachable` (finding, exact): `E_i` is empty — every packet the rule
  matches is decided by an earlier rule. The witness is a packet from
  `R_i` that the linear evaluator decides at an earlier rule (absent
  only when `R_i` itself is empty).
- `redundant` (finding, exact): `E_i` is nonempty, but replaying
  first-match over `E_i` with only the later rules — the exact
  post-removal semantics — decides every packet with rule `i`'s own
  action. Evidence is regional, not a packet: `covered_by` lists the
  later same-action rules deciding part of `E_i`, and `implicit_deny`
  records that the remainder falls through to the implicit final deny
  (possible only for a deny rule — an explicit final
  `deny ip any any` whose effective region survives is therefore
  redundant in this model, because removal leaves the implicit deny).
  For the covered part this implies the classic coverage form
  `E_i ⊆ union(later same-action matches)`.
- `generalization` / `correlation` (relations, exact relations,
  heuristic warnings): for a different-action pair (earlier `i`, later
  `j`) — generalization when `R_i ⊊ R_j` (an exception before a broader
  rule of the other action), correlation when the matches partially
  overlap both ways. Equal matches are not relations (the later rule is
  already an unreachable finding); `R_j ⊆ R_i` is shadowing territory,
  covered by findings; same-action pairs are never relations. The
  witness packet lies in both matches — inside `E_i` when part of it
  reaches `R_j` — with the decision the evaluator actually gives. The
  two layers are never mixed: relations carry no claim that either
  rule is wrong.

A rule carries at most one finding; unreachable takes precedence over
redundant. Findings are ordered unreachable-first, then redundant, each
in ascending rule order; relations are ordered by (earlier, later).

The whole audit runs under one comparison budget (operations/boxes/
wall-clock). No effective region is materialized: passes stream carve
pieces of `E_i` (the effective region of a broad late rule fragments
beyond the box budget even for small ACLs), deciding each piece as it
is emitted. Budget exhaustion ends the audit with `complete=false`,
the budget stop `reason`, and the `stage`
(`effective|redundancy|relations`) plus rule cursors where the search
stopped; findings decided before the stop stay (each is exact on its
own), everything from the stop onward is unexamined — never a clean
bill of health. `relations_complete` is set only when the relation
pass finishes. Box counts in findings describe the streaming carve
decomposition; like all box counts in this engine they are
representation-dependent, not semantic quantities.

Replay (`audit-replay`, or `audit.Replay`) re-derives the audit from
the same configuration: `AUDIT_REPLAYED` when the re-derived record
hash matches, `TIMING_SENSITIVE` when only budget accounting differs
(the findings must still agree), otherwise an error. CLI `audit`
exits 0 when the audit completes and 2 when it is incomplete or fails;
`audit-replay` exits 0 on `AUDIT_REPLAYED`, 2 otherwise.
