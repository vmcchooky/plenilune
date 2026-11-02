# Đặc tả tích hợp MoonStorm — hợp đồng subprocess Plenilune

Ngày 02/11/2026 · Hạng mục P2-7 (Phase 2). Tài liệu này là đặc tả tích hợp
cho **một** integrator cụ thể — MoonStorm — soạn từ các hợp đồng đã cài đặt
ở P2-1 → P2-5 và P2-8. `CONTRACTS.md` vẫn là authority cho wire schema;
đặc tả này không định nghĩa schema mới, chỉ chốt cách MoonStorm dùng các
schema hiện có: thứ tự gọi, pin phiên bản, quyền sở hữu ngân sách, chuỗi
toàn vẹn và chính sách lỗi. Client mẫu chạy được của đặc tả là
`examples/moonstorm-client/` (Go, stdlib only).

Trạng thái pháp lý (P2-6): **pending** — rà soát pháp lý GPL bằng văn bản
chưa xong (xem bảng quyết định ở `docs/plan/STATUS.md`). Mẫu tích hợp dưới
đây là mẫu thuận lợi nhất theo FSF FAQ (subprocess + JSON có version),
nhưng trạng thái pending **chặn hành động phân phối** MoonStorm kèm
Plenilune; nó không chặn việc soạn đặc tả này hay mọi hạng mục khác.

## 1. Mô hình tiến trình và ranh giới

- Plenilune chạy như **subprocess**: MoonStorm exec binary `plenilune`,
  đưa JSON vào qua stdin/file, đọc JSON ở stdout và envelope lỗi ở stderr.
  **Không bao giờ import Plenilune thành thư viện Go vào MoonStorm** —
  ranh giới này là một phần của hợp đồng, không phải lựa chọn kỹ thuật
  tùy ý (xem P2-6).
- Phân chia sở hữu:
  - **MoonStorm giữ**: xác thực/ủy quyền người dùng, giải mã capture,
    kho artifact, audit log phía tích hợp, pool tiến trình và giới hạn
    OS (mục 4), khóa ký của MoonStorm (mục 7), mã hóa at-rest (mục 9).
  - **Plenilune giữ**: ngữ nghĩa phân tích và verdict (engine sở hữu
    mọi verdict; AI — nếu bật ở chế độ standalone — chỉ đề xuất), seal
    và replay của artifact, các trần cứng của contract (mục 5).
- Plenilune là binary local đơn lẻ: parser/verifier không có I/O mạng
  hay thiết bị; nó không fetch artifact, không biết danh tính caller.
  Mọi byte cấu hình MoonStorm đưa vào đều đã được MoonStorm giải mã
  trong bộ nhớ của chính MoonStorm.

## 2. Khởi động: cổng capabilities (bắt buộc, trước mọi job)

Trước job đầu tiên — và sau mỗi lần thay binary — MoonStorm chạy
`plenilune capabilities --json` (hoặc `GET /api/capabilities` trên demo
server; cả hai phục vụ cùng một document từ một builder duy nhất) và
**gate**: không khớp thì từ chối sớm kèm lý do, không gửi cấu hình.
Pin hiện tại của hợp đồng tích hợp:

| Kiểm tra | Giá trị đòi hỏi |
|---|---|
| `schema` của document | `plenilune.capabilities/v1` |
| `plenilune.evidence` current | `plenilune.evidence/v2` |
| `plenilune.analysis` current | `plenilune.analysis/v3` |
| `plenilune.analysis-request` current | `plenilune.analysis-request/v1` |
| `plenilune.analysis-replay-request` current | `plenilune.analysis-replay-request/v1` |
| `plenilune.bundle` current (khi dùng bundle) | `plenilune.bundle/v1` |
| `plenilune.signature` current (khi dùng chữ ký) | `plenilune.signature/v1` |
| features cho vòng lõi | `dual_digest`, `partial_results` |
| features theo chân dùng thêm | `review_bundle` (bundle), `evidence_signing` (chữ ký) |

Client mẫu gate thêm `audit` và `repair_enumeration` vì workflow của
chính nó dùng hai tính năng đó — nguyên tắc là **gate đúng những gì
luồng của mình dựa vào**, không gate thừa. Danh sách `supported` trong
document cho biết các version đã đóng băng còn replay được (evidence
v1|v2, analysis v1|v2|v3): job mới luôn ghi theo `current`; artifact cũ
trong kho vẫn xác minh được bằng đường replay của chính version đó.

Engine/parser version được pin qua `domains` của document (hiện tại:
acl — parser `cisco-acl/v0.2`, engine `acl-exact/v0.4`; prefix-list —
parser `cisco-prefix-list/v0.2`, engine `prefix-list-exact/v0.2`) và
được đối chiếu với `versions` ghi trong từng artifact. Các version này
nằm trong content projection của artifact (mục 7) nên đổi engine là
đổi content identity — kết quả của hai engine không bao giờ trùng khóa.

## 3. Mô hình job và idempotency

Mỗi job MoonStorm ghi (pin) ba thứ vào hồ sơ job của mình:

1. **analysis schema** (`plenilune.analysis/v3`) và **engine version**
   đã gate ở mục 2;
2. **budget profile**: toàn bộ khối `options` sẽ gửi (mục 5) — profile
   là một phần của danh tính job, vì options nằm trong content
   projection của artifact: cùng một cặp cấu hình dưới hai profile
   khác nhau là hai kết quả khác nhau với hai `content_sha256` khác
   nhau. Artifact tự ghi lại options hiệu lực ở request echo, nên
   profile luôn truy lại được từ chính artifact;
3. **khóa idempotency** = khóa cache của P2-1/P1-5: digest của (digest
   byte của hai phía, conditions/policies như được cấp, options đầy
   đủ, parser version, engine version). Trước lượt chạy đầu, MoonStorm
   tính khóa này để dedup job trùng; sau lượt chạy đầu, `content_sha256`
   của artifact là khóa lưu trữ chính thức.

Hệ quả vận hành:

- Cùng khóa idempotency → cùng `content_sha256` (trừ ca timing ở mục
  6): MoonStorm được phép coi lượt chạy sau là **cache hit** và dùng
  lại artifact đã lưu, không lưu hai bản.
- Tiến trình Plenilune bị giết (OOM, OS kill, MoonStorm hủy) cho ra
  **không có artifact**: trạng thái job phía MoonStorm là INCONCLUSIVE.
  Không có đường nào một job bị giết được ghi thành EQUIVALENT hay
  bất kỳ verdict hoàn tất nào.

## 4. Hàng đợi và giới hạn vận hành

- **Hai hàng đợi tách biệt, collection ưu tiên tuyệt đối.** Hàng đợi
  thu thập (backup capture) không bao giờ phải chờ hàng đợi phân tích:
  lỡ một bản backup là mất dữ liệu, còn phân tích chậm chỉ là chậm.
  Analysis không được phép giữ tài nguyên khiến collection trễ hạn.
- Khi MoonStorm tự quản pool, mỗi tiến trình analysis chạy
  `--workers=1`: song song theo object là việc của pool MoonStorm,
  không phải của một tiến trình con tự phình ra. (Số worker không đổi
  artifact — cùng input cho artifact giống hệt ở mọi số worker — đây
  thuần túy là kỷ luật tài nguyên.)
- **Giới hạn cứng ở OS do MoonStorm áp** (cgroup trên Linux, Job
  Object trên Windows): CPU, bộ nhớ, thời gian sống của tiến trình
  con. Trần của contract (mục 5) là lớp trong; giới hạn OS là lớp
  ngoài và là lớp duy nhất chặn được tiến trình treo không phản hồi.

## 5. Lượt phân tích

### Request

`plenilune analyze --request -` với document `plenilune.analysis-request/v1`
qua stdin — đường stdin tồn tại để cấu hình đã giải mã **không phải ghi
xuống đĩa** phía caller:

```json
{
  "schema": "plenilune.analysis-request/v1",
  "before": {"text": "<cấu hình trước>", "ref": "<nhãn caller>"},
  "after":  {"text": "<cấu hình sau>",  "ref": "<nhãn caller>"},
  "options": {
    "acl": {"limits": {"max_operations": 2000000, "max_boxes": 50000,
                       "timeout_ns": 2000000000},
            "max_witnesses": 8},
    "max_classes": 32,
    "deadline_ms": 30000
  }
}
```

- `ref` là nhãn tương quan mờ của caller (tối đa 256 ký tự ASCII in
  được), không phải danh tính, không tham gia content digest.
- Nguồn mỗi phía ≤ 4 MiB và ≤ 200.000 dòng; decoder strict: field lạ,
  sai type, duplicate key đều bị từ chối typed.
- `select` giới hạn các object được phân tích hành vi; `policies` gắn
  policy đã freeze cho object acl (đánh giá obligation/scope theo
  CONTRACTS §8) khi job cần — vắng mặt là lựa chọn hợp lệ, không phải
  lỗi.

### Ngân sách: ai sở hữu gì

- **MoonStorm sở hữu ngân sách theo job**: `deadline_ms` (mặc định
  30000) và việc hạ các giới hạn ACL theo profile của job.
- **Plenilune sở hữu trần cứng, không thương lượng**: `deadline_ms`
  ∈ 1..120000; `max_classes` ∈ 1..128; `max_analyzed` ∈ 1..64;
  `max_witnesses` ∈ 1..64; giới hạn ACL chỉ được **hạ** dưới defaults
  (2.000.000 operations / 50.000 boxes / 2s timeout), không nâng được.
  Giá trị ngoài khoảng bị từ chối bằng `OPTIONS_INVALID` (stage
  validate) — không bao giờ bị clamp lặng lẽ. Bên chặt hơn thắng theo
  cấu trúc: caller không gửi được giá trị lỏng hơn trần, và tool
  không nới giá trị caller đã siết.
- **Khối `options` là all-or-nothing**: có mặt là thay toàn bộ
  defaults, không có defaulting theo field (ngoại lệ duy nhất:
  `max_analyzed` vắng mặt/0 nghĩa là default 16, theo CONTRACTS §8).
  Gửi khối một phần — ví dụ chỉ `deadline_ms` — để các field còn lại
  bằng 0 và bị `OPTIONS_INVALID`. Caller hoặc bỏ hẳn `options`, hoặc
  gửi đủ profile (defaults hiện hành công bố ở `options` trong
  capabilities). Client mẫu gửi đủ profile và chỉ thay `deadline_ms`
  theo job.

### Artifact và cách đọc kết quả

- Exit 0 nghĩa là **có artifact** (`plenilune.analysis/v3`), kể cả khi
  mọi object đều `inconclusive`/`not_analyzed`. Exit code chỉ phân
  biệt artifact-với-lỗi; kết quả nằm ở `objects[].status` và
  `summary`, không nằm ở exit code.
- **Không có verdict toàn cục.** Object có status
  `changed|equivalent|unchanged|added|removed|unsupported|inconclusive|not_analyzed`;
  chỉ `changed|equivalent|unchanged|inconclusive` mang so sánh hành
  vi. MoonStorm lưu từng artifact làm kết quả của đúng cặp đó (mục 8),
  không tổng hợp thành một verdict cho cả cấu hình.
- Artifact mang sẵn mọi thứ cần để xác minh độc lập: sources pin
  digest byte chính xác, request echo (select/policies/options),
  versions, và cặp digest ở mục 7.

## 6. Kết quả một phần và INCONCLUSIVE

Một artifact có thể hoàn tất một phần. Kỷ luật phía integrator:

- Object `inconclusive` (engine bị budget dừng — verdict INCONCLUSIVE
  kèm `partial` đã seal ở đường ACL) và object `not_analyzed` (lý do
  `global_budget`, `deadline_exceeded`, `canceled`…) là **kết quả
  thật đã đánh dấu**, không phải lỗi và không phải EQUIVALENT. Phần
  đã quyết trong `partial` là exact; vùng `undecided` không được suy
  diễn theo cả hai hướng.
- Dừng vì **ngân sách logic** (operations/boxes, global budget ledger)
  là tất định: cùng input cho cùng artifact, cache theo
  `content_sha256` được — với điều kiện hiển thị/trạng thái giữ
  nguyên nhãn một phần.
- Cắt vì **deadline/cancel theo wall-clock** là phụ thuộc timing:
  artifact mang limitation `analysis_deadline_exceeded` /
  `analysis_canceled` và replay của nó trả `TIMING_SENSITIVE`.
  **Không bao giờ cache một artifact phụ thuộc timing như kết quả
  cuối của job**; muốn kết quả cuối thì chạy lại với ngân sách thời
  gian đủ, và chỉ artifact nào replay ra `ANALYSIS_REPLAYED` mới là
  bản cuối.
- `TIMING_SENSITIVE` ở mọi đường replay đều có nghĩa "chưa xác minh
  trọn vẹn được" — không phải PASS, không phải FAIL — và không bao
  giờ được nâng cấp thành kết quả đầy đủ ở phía integrator.

## 7. Chuỗi toàn vẹn: digest, bundle, chữ ký, replay

Bốn lớp, mỗi lớp một việc khác nhau — không lớp nào thay lớp khác:

1. **Cặp digest (P2-1)** — mọi artifact hiện hành mang:
   - `content_sha256`: digest trên payload ngữ nghĩa (inputs,
     definitions, scope, parser/engine versions, options, results) —
     **không** gồm build provenance (`tool.commit`, thời gian, nhãn
     `ref`, metadata AI). Đây là khóa dedup/cache/idempotency và là
     thứ được ký.
   - `sha256` (**artifact digest**): digest trên toàn envelope gồm
     provenance và cả content digest — định danh đúng bản artifact
     đó, dùng để kiểm integrity của một bản cụ thể.
   Cùng input/options, hai build khác commit cho `content_sha256`
   bằng nhau và artifact digest khác nhau.
2. **Bundle (P2-4)** — khi một hồ sơ review phải di chuyển (giữa các
   máy, giữa MoonStorm và người duyệt): `plenilune.bundle/v1` là
   manifest JSON liệt kê các tệp của case kèm role + SHA-256 từng
   tệp. Manifest chỉ là mục lục toàn vẹn: nó không tự hash và không
   thay receipt; proof vẫn là các artifact đã seal bên trong, được
   replay kiểm lại. Thiếu tệp đã liệt kê hay lệch một byte đều là lỗi
   typed (`BUNDLE_FILE_MISSING` / `BUNDLE_DIGEST_MISMATCH`).
3. **Chữ ký (P2-5)** — khi artifact rời khỏi máy đã sinh ra nó và cần
   danh tính người ký: **MoonStorm ký `content_sha256`** (chính xác
   hơn: ký cặp digest theo envelope `plenilune.signature/v1`) bằng
   khóa Ed25519 của MoonStorm; private key và danh tính người ký ở
   lại phía MoonStorm. Plenilune chỉ cần đường verify bằng khóa công
   khai: `plenilune verify-signature --evidence FILE --signature FILE
   [--fingerprint HEX | --pubkey FILE]` — fingerprint (SHA-256 hex
   trên byte thô của khóa công khai) là cơ chế ghim signer duy nhất,
   không có PKI trong phạm vi này. Chữ ký chứng minh đúng một điều:
   người giữ private key đã ký lên cặp digest này. Nó **không**
   chứng minh phân tích đúng, không phải quyết định duyệt, không
   chứng minh danh tính pháp lý của người ký và không chứng minh
   thời điểm ký (envelope cố ý không có timestamp). Vì vậy chữ ký
   không bao giờ là cổng của verdict hay replay: artifact có chữ ký
   sai — hoặc không có chữ ký — vẫn replay y hệt.
4. **Replay là nguyên thủy xác minh.** `plenilune analyze-replay
   --request -` (document `plenilune.analysis-replay-request/v1`:
   `{schema, before, after, evidence}`) tính lại artifact từ byte
   gốc và kiểm cả hai digest trước khi tính. `ANALYSIS_REPLAYED` +
   cặp digest của kết quả replay khớp cặp đã lưu = xác minh trọn
   vẹn. Mọi lệch byte/version/kết quả → lỗi typed ở stage replay
   (mục 10). Người duyệt ngoài MoonStorm cũng xác minh bằng đúng
   đường này — không cần tài khoản, không cần AI.

## 8. Sự kiện kích hoạt và cách so cặp

Phân tích phía MoonStorm chạy theo sự kiện, không theo lịch quét:

- **Capture mới**: một cặp (baseline, capture mới) hoặc (capture
  trước, capture mới) hình thành một job phân tích.
- **Obligation-set changed**: khi tập obligation thay đổi, obligation
  mới phải được chạy lại **trên các capture cũ đã lưu** — kết quả dưới
  tập obligation cũ không trả lời được câu hỏi của tập mới. Vì
  conditions/policies nằm trong content projection, kết quả của hai
  tập obligation khác nhau có content identity khác nhau: chúng là
  các job khác nhau, không bao giờ trùng khóa cache.
- **So từng cặp, không gộp N-way**: MoonStorm so baseline↔current và
  current↔proposed thành các job riêng và lưu từng receipt/artifact
  riêng. Không tồn tại "verdict drift toàn cục" cho ba chiều trở lên
  trong hợp đồng này.

## 9. Lưu trữ phía MoonStorm

- Artifact phân tích là **dữ liệu suy ra từ cấu hình**: witness và
  change class lộ chi tiết policy. MoonStorm mã hóa artifact at-rest
  như capture, cùng chính sách truy cập như capture.
- Lớp metadata chỉ neo **receipt/digest**: `content_sha256`, artifact
  digest, schema/engine version và trạng thái job. Metadata không
  chứa cấu hình, không chứa witness.
- Repair receipt và bundle chứa candidate text: xử lý như dữ liệu
  nhạy cảm cùng cấp (nhãn `sensitive` trên receipt là chỉ dấu xử lý,
  không phải cơ chế bảo mật).

## 10. Lỗi và chính sách thử lại

Mọi lỗi qua biên mang envelope của taxonomy P2-3: `code`, `stage`
(`request|parse|validate|engine|budget|replay|provider`), `field`
khi áp dụng, `retryable`, và `line`/`side` khi biết. Bảng đầy đủ 75
code nằm ở CONTRACTS §4 và bằng đúng `error_codes` trong capabilities;
MoonStorm map theo code/stage, **không parse message text**. Chính
sách theo stage:

| Stage | Chính sách phía MoonStorm |
|---|---|
| `request` | Lỗi vận chuyển/gọi (đọc/ghi, quá tải `SERVER_BUSY`, hủy `CANCELED`): thử lại theo `retryable` của envelope. |
| `parse` | Cấu hình ngoài ngữ pháp đã mô hình: không thử lại nguyên trạng; đánh dấu nguồn, chuyển người xử lý. |
| `validate` | Job sai hợp đồng (`OPTIONS_INVALID`, `REQUEST_SCHEMA`, `SELECT_INVALID`…): sửa job rồi mới gửi; thử lại nguyên trạng luôn fail y hệt. |
| `engine` | Lỗi tính toán nội bộ: không thử lại mù; ghi nhận kèm artifact đầu vào để tái hiện. |
| `budget` | `DEADLINE_EXCEEDED` có `retryable: true` — thử lại chỉ có nghĩa khi đổi budget profile hoặc tải máy khác; đó là job mới với profile mới, không phải cùng job. |
| `replay` | **Không bao giờ thử lại mù.** `EVIDENCE_MISMATCH`, `BUNDLE_DIGEST_MISMATCH`, `SIGNATURE_DIGEST_MISMATCH`, `SIGNATURE_MISMATCH`, `CHAT_ARTIFACT_INVALID` là vấn đề của bằng chứng (bị sửa, sai bản, sai khóa): cách ly artifact, báo người phụ trách, giữ nguyên bản để điều tra. Riêng `TIMING_SENSITIVE` là *status*, không phải lỗi: xử lý theo mục 6. |
| `provider` | Chỉ tồn tại ở chế độ AI standalone của Plenilune; không thuộc vòng job MoonStorm. |

Mã thoát của các lệnh trong vòng tích hợp: `capabilities` 0 khi có
document; `analyze` 0 khi có artifact / 2 khi lỗi typed; `analyze-replay`
0 khi `ANALYSIS_REPLAYED` / 2 cho `TIMING_SENSITIVE` và mọi lỗi;
`bundle-replay` 0 chỉ khi mọi artifact trong bundle replay khớp;
`verify-signature` 0 khi chữ ký hợp lệ. Client mẫu bọc các mã này
thành mã thoát riêng của nó (mục 12) — mã của client không phải một
phần của contract.

## 11. Version và nâng cấp

- Capabilities v1 **chỉ cộng thêm**: field, phần tử list và version hỗ
  trợ có thể thêm; không xóa, không đổi tên, không đổi nghĩa. Đổi phá
  vỡ phải bump schema capabilities — và integrator gate ở mục 2 sẽ từ
  chối document lạ thay vì đoán.
- Schema artifact bump theo quyết định P0-D1: **bump khi field bị xóa
  hoặc đổi nghĩa; thay đổi thuần cộng thêm không bump** (tiền lệ:
  partial results của P1-2 cộng field, không bump; redaction của P0-1
  xóa field, bump v1→v2; dual digest của P2-1 đổi nghĩa identity,
  bump evidence v1→v2 và analysis v2→v3).
- Version đã đóng băng vẫn replay được bằng đúng quy tắc của nó và bị
  từ chối nếu mang field của version mới (ví dụ artifact v1/v2 mang
  `content_sha256`). Integrator không tự "nâng cấp" artifact cũ: muốn
  bản hiện hành thì phân tích lại từ byte gốc.
- Khi capabilities báo một **schema mới hơn** mức integrator hỗ trợ:
  **từ chối job, không đoán** — gate fail là hành vi đúng. Khi báo
  version cũ hơn: chỉ chạy nếu `current` vẫn bằng pin của integrator;
  các bản cũ hơn nữa chỉ còn vai trò replay cho artifact đã lưu.
- Nâng cấp binary giữa chừng không đổi kết quả đã lưu: artifact đã
  seal giữ nguyên cặp digest và replay theo quy tắc của version nó
  ghi; build provenance khác nhau không đổi content identity (mục 7).

## 12. Bề mặt ổn định và bề mặt nội bộ

**Ổn định cho integrator** (có version, có contract test, đổi là phải
theo mục 11):

- Document `plenilune.capabilities/v1` qua CLI và `GET /api/capabilities`.
- Vòng analysis: `plenilune.analysis-request/v1` →
  `plenilune.analysis/v3`; `plenilune.analysis-replay-request/v1` →
  kết quả replay; cặp digest `content_sha256`/`sha256`.
- Đường evidence cặp ACL: `plenilune.evidence/v2` qua `diff`/`replay`.
- Bundle `plenilune.bundle/v1` (`bundle-export`/`bundle-replay`) và
  chữ ký `plenilune.signature/v1` (`sign`/`verify-signature`) cho vận
  chuyển và danh tính.
- Audit `plenilune.audit/v1`; repair `plenilune.repair/v1` và
  enumeration `plenilune.repair-enumeration/v1` cho các luồng review
  dùng đến chúng.
- Error taxonomy (envelope + registry code/stage/retryable) và mã
  thoát đã ghi ở CONTRACTS §1/§8.

**Nội bộ — không phải hợp đồng tích hợp:**

- Các route HTTP của demo server ngoài `GET /api/capabilities`: đó là
  transport của UI local (gồm cả watches/inbox của P2-8 — tính năng
  local single-user của bản standalone; phía MoonStorm của trigger
  là đặc tả ở mục 8, chưa có webhook/notification nào được cài đặt).
- Package Go của Plenilune (kể cả khi kỹ thuật cho phép import —
  ranh giới mục 1 cấm), internals của web UI, và định dạng lưu trữ
  phía server demo.
- Phiên AI (`assist`, `plenilune.ai/v1`): tính năng local opt-in;
  MoonStorm không gọi nó trong vòng job. Verdict không bao giờ đi qua
  AI ở cả hai phía của ranh giới.

## 13. Vòng mẫu và bảng từ chối

Client mẫu `examples/moonstorm-client/` (Go) chạy đúng vòng: gate
(mục 2) → analyze (mục 5) → kiểm dạng artifact và cặp digest → chạy
lại nguyên request để trình diễn cache hit theo `content_sha256`
(mục 3) → replay (mục 7) → báo cáo theo object + summary + cặp
digest. Mọi từ chối của client đều suy ra được từ đặc tả này:

| Tình huống | Đầu ra của client | Exit |
|---|---|---|
| Binary không chạy được / capabilities thoát ≠ 0 | `capabilities call failed: …` | 2 |
| Document capabilities không phải JSON hợp lệ | `capabilities document is not valid JSON` | 2 |
| `-before` thiếu `-after` (hoặc ngược lại); file cấu hình không đọc được | thông báo usage/input ở stderr | 2 |
| `schema` của document ≠ `plenilune.capabilities/v1` | `REFUSED: capabilities schema …` | 3 |
| evidence current ≠ v2, analysis current ≠ v3, hoặc schema id vắng mặt | `REFUSED: schema …` (từng lý do một dòng) | 3 |
| Thiếu feature trong {`audit`, `dual_digest`, `repair_enumeration`} | `REFUSED: tool lacks required feature …` | 3 |
| analyze thoát ≠ 0 | `ANALYZE FAILED: <code> (stage …, retryable …)` từ envelope | 4 |
| Artifact sai schema hoặc cặp digest thiếu/hỏng định dạng | `ARTIFACT REJECTED: …` | 5 |
| Lượt chạy lại cho `content_sha256` khác | dòng `NO CACHE HIT: …` (cảnh báo, giữ artifact đầu — không phải từ chối) | — |
| Replay trả cặp digest khác cặp đã lưu | `REPLAY MISMATCH: …` | 5 |
| Replay lỗi typed (vd `EVIDENCE_MISMATCH` khi artifact bị sửa) | `REPLAY FAILED: <code> (stage replay, …)` | 6 |
| Replay `TIMING_SENSITIVE` | `REPLAY: TIMING_SENSITIVE — … NOT final` | 6 |
| Mọi bước khớp | `REPLAY: ANALYSIS_REPLAYED …` sau các dòng `OBJECT`/`SUMMARY`/`DIGESTS`/`CACHE HIT` | 0 |

Chạy tay client không tham số cấu hình thì dừng sau gate (in
`COMPATIBLE: …`, exit 0) — hành vi gate-only giữ từ P2-2.
`client.mjs` trong cùng thư mục là bản phác thảo Node cũ hơn cho cùng
vòng analyze/replay; client Go là bản tham chiếu hiện hành.
