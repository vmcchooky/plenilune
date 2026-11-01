# Phase 1 — Engine nhạy delta, repair chủ động, fuzz

Điều kiện vào: Phase 0 đạt điều kiện ra. Điều kiện ra: P1-1 đạt cổng benchmark
đã đăng ký trước; P1-3/P1-4 chạy offline không cần AI; fuzz CI xanh.
Nguồn: Qoder mục 3 (LCP, enumeration, diagnostics, song song) + nghiên cứu Q1–Q8
mục Q1/Q2/Q4 (coalescing/streaming, INCONCLUSIVE một phần, budget toàn cục, fuzz).

Nguyên tắc chung của phase: mọi thay đổi engine đều là **engine version mới**,
giữ nguyên khả năng replay của version cũ; mọi cải tiến hiệu năng đều phải có
số đo trước/sau trên cùng corpus, cùng môi trường; cổng đạt/không đạt được ghi
trước khi chạy, không sửa cổng sau khi thấy kết quả.

## P1-1 — Engine v0.3: LCP residual (so theo tiền tố ngữ nghĩa chung)

**Ý tưởng:** nếu `before` và `after` có `k` rule đầu giống nhau về ngữ nghĩa
(cùng match + action; bỏ qua sequence, log, whitespace) thì packet khớp một
trong `k` rule đó được quyết định giống nhau ở hai bên. Chỉ so phần còn lại
trên vùng chưa bị `k` rule phủ:

```text
covered = union(region(k rule chung đầu))
diff    = so sánh before[k:] với after[k:] trên phần bù của covered
```

Tiền đề đã kiểm chứng: `exactDelta` hiện dựng cả hai permit set từ đầu
(`internal/verify/delta.go:152`).

**Phạm vi:** engine version mới `acl-exact/v0.3`; v0.1/v0.2 giữ nguyên replay.
Map ordinal/line về ACL gốc; witness vẫn qua linear evaluator kiểm lại.

**Cổng benchmark (đăng ký trước, không sửa sau):** trên corpus cố định hiện có
(100/250/512 rule; các ca identical, insert-at-head, swap, change-port, append/
sửa cuối — bổ sung ca append và sửa cuối vào corpus vì đó là ca LCP nhắm tới):
- Ca identical 512 rule phải hoàn tất (hiện INCONCLUSIVE).
- Ca sửa cuối/append: ops và median time giảm ít nhất 50% so với v0.2.
- Ca insert-at-head: không tệ hơn v0.2 quá 10% (overhead tính covered).
Không đạt cổng → dừng theo protocol mục 3, báo số đo; không tự chuyển phương án.

**Nghiệm thu correctness:** box-for-box/set-equivalence với v0.2 trên corpus;
exhaustive reduced-domain oracle; witness re-evaluation; replay receipt v0.1/
v0.2 không đổi.

**Non-claim:** exact trong mô hình ACL IPv4 hiện tại; không nói gì về thiết bị,
routing, NAT, fragment ngoài scope đã công bố.

## P1-2 — INCONCLUSIVE một phần theo cặp rule

**Vấn đề:** khi budget cạn, kết quả toàn cục là INCONCLUSIVE dù phần lớn cặp
rule quyết định đã quyết xong — người review không biết phần nào còn mở.

**Làm:** khi search dừng sớm, artifact nêu rõ các cặp rule (before-ordinal ×
after-ordinal) / vùng chưa quyết được, tách khỏi phần đã quyết. Trạng thái từng
obligation giữ độc lập như hiện tại.

**Nghiệm thu:** ca ép cạn budget cho artifact có cả phần decided lẫn undecided
được liệt kê đúng; không phần undecided nào bị diễn đạt thành an toàn/đã kiểm.

## P1-3 — Liệt kê bản sửa deterministic (repair enumeration)

**Ý tưởng:** từ change classes và obligation violations, sinh tập candidate
hữu hạn bằng các phép biến đổi có giới hạn:
- xóa rule `after` đang quyết định class bị vi phạm;
- xóa/thay rule `after` giao với vùng obligation và có action sai;
- chèn rule ngoại lệ ngay trước rule quyết định (đúng vùng vi phạm);
- reorder bằng remove + add khi class cho thấy thứ tự là nguyên nhân;
- thu hẹp predicate của rule về `rule \ vùng vi phạm` nếu biểu diễn được bằng
  một rule hợp lệ.

Mỗi candidate là một `patch.Request`, bắt buộc qua `repair.Verify` hiện có.
Chỉ hiển thị candidate `VERIFIED`; candidate bị từ chối hiện lý do, không gọi
là thành công. Sắp xếp tất định theo (số edit, số class giải quyết, hash
candidate) — không gọi là "best" hay "safest". Bị chặn theo số class/rule và
deadline; tôn trọng patch v1 (tối đa 5 edit, mỗi edit là rule hợp lệ —
`internal/patch/patch.go`).

**Điều kiện:** policy phải do operator accept; collateral phải VERIFIED;
không apply thiết bị; không nâng UNVERIFIED thành VERIFIED.

**Nghiệm thu:** corpus synthetic có ca sửa được và ca phải từ chối; mọi
candidate hiển thị đều có receipt `repair.Verify` thật; không candidate nào
VERIFIED khi policy chưa accept; kết quả tất định qua nhiều lần chạy; chạy
hoàn toàn offline khi AI tắt.

## P1-4 — Chẩn đoán rule exact (audit)

**Làm:** lệnh/phân tích on-demand cho object đã chọn, ngân sách riêng:
- **Unreachable:** `E_i = R_i \ union(R_1..R_{i-1}) = ∅` → rule không quyết
  định packet nào.
- **Redundant:** `E_i \ union(các rule sau cùng action) = ∅` → bỏ rule không
  đổi hành vi trong mô hình.
- Bổ sung quan hệ generalization/correlation theo taxonomy Al-Shaer & Hamed ở
  mức quan hệ (exact), nhưng **cảnh báo** suy ra từ chúng là heuristic — diễn
  đạt phải tách hai lớp.
- Xếp hạng hiển thị: rule có vùng hiệu lực rỗng lên đầu (ACL thật nhiều idiom
  deny-trước-permit có chủ đích; không gây nhiễu bằng cảnh báo hàng loạt).
- Budget cạn → `INCONCLUSIVE` cho chẩn đoán, không suy thành "không có rule chết".

**Nghiệm thu:** oracle exhaustive trên reduced domain; không false positive;
test wildcard không liên tục, port range, implicit deny; đối chiếu linear
evaluator. Không có đường tự xóa rule trong hạng mục này.

## P1-5 — Song song hóa object + budget toàn cục + cache comparison

**Vấn đề (đã kiểm chứng):** phân tích object tuần tự (`internal/analysis/
analysis.go`, `MaxAnalyzed=16`, deadline 30s dùng chung) — deadline có thể hủy
**toàn bộ** artifact; và UI mở object đã analyze vẫn parse + compare lại
(`web/src/hooks/useReview.ts:437`).

**Làm:**
1. Hai pha: phân loại object tuần tự (unchanged/added/removed/unsupported/
   not_analyzed) để quyết định tất định; chạy engine song song bằng worker pool
   cố định. Kết quả ghi vào mảng theo index; artifact giữ nguyên thứ tự.
2. Budget box/bộ nhớ **toàn cục** cho cả lượt phân tích, cộng budget riêng từng
   object như hiện tại. Deadline hủy phải cho ra artifact một phần có đánh dấu
   rõ object nào chưa xong — thay cho all-or-nothing (liên kết P1-2).
3. `--workers` mặc định `min(4, GOMAXPROCS)`; caller (MoonStorm) đặt được
   `--workers=1` để tự quản pool, tránh oversubscription.
4. `max_analyzed` thành option (1–64, mặc định 16) thay cho hằng số cứng.
5. Cache comparison theo `(beforeBlockSHA, afterBlockSHA, options)` ở UI/server
   để `openObject` không tính lại; cache chỉ là tối ưu nội bộ, evidence vẫn
   được tạo một lần và replay độc lập.

**Nghiệm thu:** cùng input/options → cùng artifact digest bất kể số worker;
`go test -race`; đo latency + peak memory trên config 16 object; deadline/cancel
không để lại artifact dở dang không đánh dấu.

## P1-6 — Fuzzing vào CI

**Làm:** Go native fuzz targets (`testing.F`) cho: parser ACL/prefix-list;
JSON decoder nghiêm ngặt; round-trip parse → print → re-parse; differential
trace (point evaluator) vs brute-force trên domain rút gọn. Mọi fuzz case phải
tôn trọng budget: được phép INCONCLUSIVE, cấm panic/OOM/không kết thúc.
Chạy ngắn trong CI mỗi thay đổi, chạy dài theo lịch nightly; seed corpus từ
testdata hiện có. OSS-Fuzz chỉ xét sau khi fuzz local sạch ổn định (ghi vào
Phase 3 nếu muốn).

**Nghiệm thu:** targets chạy trong CI; crash tìm được (nếu có) thành regression
test trước khi sửa; không có target nào bị tắt vì "khó".

## P1-7 — Nới ngân sách đề xuất AI (theo ngân sách rủi ro)

Sau khi P1-3 (enumeration deterministic) chạy: đánh giá lại các cap của AI
session (hiện 8 model requests / 12 tools / **2 proposals** / 60s). Được nới số
proposal trong budget tổng vì mọi proposal đều qua verify (tầng xanh). Không
đổi: model lock `muse-spark-1.3-contributor`, không fallback, verdict thuộc
engine, acceptance thuộc người.

**Nghiệm thu:** đo trên corpus session synthetic: tỉ lệ proposal qua được
verify, chi phí/latency; cap mới ghi vào `AI_AGENT_DESIGN.md` + CONTRACTS.
Live qualification vẫn là việc riêng, không suy từ mock.

## P1-8 — Luồng verdict nhanh (quick verdict path)

**Vấn đề:** UI hiện ép người dùng đi qua thanh 5 ga (Source → Conditions →
Result → Repair → Evidence); verdict đầu tiên chỉ thấy được sau ga
Conditions. Trong khi đó verdict và obligations phụ thuộc lẫn nhau: thấy
thay đổi mới biết nên đặt obligation gì, đặt xong lại muốn quay lại xem thay
đổi — luồng chia bước bắt người dùng qua lại đúng hai ga đó.

**Cơ sở nghiên cứu (Chooky đã duyệt 01/11/2026):**
- Progressive disclosure (Nielsen, 1995; NN/g): phần nâng cao/ít dùng thuộc
  về lớp phụ; lớp đầu chỉ chứa thứ đa số ca cần.
- Wizard hợp với tác vụ hiếm gặp dành cho người mới; soát thay đổi là việc
  *thường xuyên của chuyên gia* — hướng dẫn từng bước làm chậm họ.
- Luồng chia bước (staged) khiến người dùng kẹt khi các bước phụ thuộc nhau;
  ở đây verdict ↔ obligations phụ thuộc nhau đúng nghĩa đó.
- Time-to-first-value quyết định việc giữ chân ở phiên đầu: bắt cấu hình
  trước khi thấy giá trị làm rớt người dùng.
- Hiệu ứng mặc định (default effect): đa số người dùng không đổi mặc định —
  đường mặc định thực chất là sản phẩm.

**Làm:**
1. Một vùng thả duy nhất cho before/after; đủ hai input là parse + phân tích
   **tự chạy** — verdict hiện với **0 click** thêm.
2. Màn verdict là màn nhà: banner verdict + một dòng tóm tắt; danh sách
   object kèm chip trạng thái; obligations **inline ngay trên cùng màn**,
   sửa là trạng thái cập nhật tại chỗ (dạng liệt kê từng obligation của
   P0-3); ca không có obligation không bị chặn ở bất kỳ đâu.
3. Repair chỉ xuất hiện theo ngữ cảnh trên màn verdict khi verdict là
   DIFFERENT và có obligation bị vi phạm.
4. Bằng chứng thành ngăn kéo/tab trên cùng màn; ticket summary (P0-3) là
   hành động chia sẻ kết quả.
5. Luồng 5 ga hiện tại giữ làm **chế độ duyệt sâu** tường minh — không còn
   là mặc định.
6. Ăn kiêng chữ: mỗi vùng một tiêu đề + một dòng ngắn; giải thích dài
   chuyển vào tooltip/lớp trợ giúp.

**Không làm (đường đỏ):** verdict không bao giờ nói quá phạm vi mô hình;
INCONCLUSIVE nổi bật ngang các verdict khác; dòng limitations ở lại trên
màn verdict (rút ngắn, không xóa); không đổi engine/schema/API; chế độ
duyệt sâu không mất năng lực nào so với hiện tại.

**Nghiệm thu (đo được):**
- Thả hai file xong verdict hiện với 0 click; ca không-obligations tới
  được verdict mà không qua bất kỳ UI điều kiện nào.
- Sửa obligation inline → trạng thái render lại tại chỗ, không điều hướng.
- Toàn bộ web test hiện có xanh, cộng test mới cho auto-run và obligations
  inline.
- Kiểm tra trực quan trên cổng Windows của owner (như đã làm ở P0-3).

## P1-9 — CLI tóm tắt (terse CLI verdict)

**Vấn đề:** đầu ra CLI hiện là artifact/báo cáo đầy đủ. Người dùng terminal
và script cần một dạng ngắn, ổn định để liếc nhanh và để máy grep, kèm mã
thoát phân biệt kết cục.

**Làm:** chế độ tóm tắt cho đường diff/analyze của CLI với ngân sách dòng
cố định: một dòng verdict; một dòng trạng thái cho từng object; obligations
dạng liệt kê (như P0-3); một dòng limitations. Từ ngữ ổn định, máy grep
được. Mã thoát có tài liệu, phân biệt EQUIVALENT / DIFFERENT /
INCONCLUSIVE / lỗi.

**Không làm:** không đổi artifact JSON; báo cáo đầy đủ cho người giữ
nguyên.

**Nghiệm thu:** golden test cho định dạng tóm tắt ở cả bốn lớp kết cục; mã
thoát có test bao phủ; tài liệu (README/usage) cập nhật.

## P1-10 — Chat thẩm vấn trên màn verdict (verdict interrogation)

**Ý tưởng:** sau khi có artifact hoàn chỉnh, người dùng hỏi về *chính kết
quả đó* ngay trên màn verdict ("vì sao OB-2 bị vi phạm", "cho xem
witness"). Đây là mở rộng của AI agent session hiện có, liên kết với ngân
sách rủi ro của P1-7. Chat là lớp thẩm vấn tầng 2, không phải đường nhập
liệu: nghiên cứu NN/g về chatbot cho thấy gõ câu hỏi chậm hơn nhìn kết quả
có cấu trúc ở tác vụ dạng này, và hộp chat trống làm mất tính khám phá.

**Làm:** panel hỏi đáp neo vào một artifact đã hoàn tất trên màn verdict.
Mọi câu trả lời phải bám artifact: retrieval do core tất định thực hiện
trực tiếp trên field của artifact đã seal, model (khi bật diễn đạt) không
được cấp tool nào — AI chỉ diễn đạt, sự thật do artifact quyết định (đúng
quy tắc dự án và ngân sách rủi ro ở master plan); mọi khẳng định thực tế
trong câu trả lời phải truy được về field của artifact. (Chỉnh theo quyết
định D5, 01/11/2026 — xem STATUS: cách này chặt hơn bản "qua engine tools"
ban đầu và được Chooky chấp nhận thay cho tool-use của model.)

**Không làm (đường đỏ):** chat không bao giờ là đường nhập cấu hình hay
obligation; không chặn và không thay thế màn verdict; chat không phát ra
và không sửa được bất kỳ verdict nào.

**Nghiệm thu:** panel chạy trên artifact đã hoàn tất mà không phải nhập
lại cấu hình; test với artifact fixture + bộ câu hỏi kịch bản, kiểm mọi
khẳng định truy được về field của artifact; AI chưa cấu hình → thông báo rõ
ràng, màn verdict vẫn dùng được đầy đủ.
