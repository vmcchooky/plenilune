# Plenilune — thiết kế verifier

Status: exact comparison, obligation, verified repair, full replay, change
classes và domain prefix-list đã triển khai. Ngày: 31/10/2026. Live Meta và UI
wiring là qualification riêng.

## Mô hình và claim

`p = (protocol, srcIPv4, dstIPv4, srcPort, dstPort)`; scope `ipv4-unfragmented-or-initial-with-complete-l4/v1`.
Chỉ xét packet không phân mảnh hoặc initial fragment có L4 đầy đủ khi cần.
Rule match trong thứ tự text; rule đầu khớp quyết định, không rule nào khớp thì deny.
Equivalence chỉ xét permit/deny; thay đổi log/remark/sequence không phải khác biệt trong verdict này.
TCP/UDP có port 0–65535; protocol khác bắt buộc hai port bằng 0 để khớp point evaluator.
Residual/obligation phải giao với domain này; witness non-TCP/UDP không được mang port ưu tiên 22.
Scope này không bao gồm routing, NAT, interface binding, stateful firewall, non-initial fragment hay hành vi IOS/ASIC đã nghiệm thu.
File scaffold chỉ chứa một `ip access-list extended NAME`; không giả làm running-config extractor.
Token ngoài subset, nhiều ACL, sequence mơ hồ hoặc vùng opaque đều bị từ chối.

## Exact engine hiện hành

Một rule là box: protocol bitset × source address cube × destination cube × source ports × destination ports.
Address cube dùng `(value, wildcard)`; bit wildcard=1 được bỏ qua, `value &= ~wildcard`.
Port là các interval inclusive đã chuẩn hóa; phép toán dùng số rộng hơn uint16 ở biên 65535.

```text
E_i = R_i \ union(R_1 ... R_(i-1))
A   = union(E_i where action_i == permit)
D   = (A_before \ A_after) union (A_after \ A_before)
```

Cube/box subtraction phải trả các phần rời nhau, đủ phủ phép hiệu và không chứa điểm thừa.
Không cần tối giản region để kết luận đúng. Chỉ normalize/merge khi có test bảo toàn tập.
Có witness đã kiểm tra → `DIFFERENT`; hết residual sau search hoàn tất → `EQUIVALENT` trong scope.
Unsupported → `UNSUPPORTED`; hết budget trước khi có kết luận → `INCONCLUSIVE`.
Budget ban đầu: 50k live boxes, 2M logical operations, 2s safety deadline; kiểm tra context trong vòng lặp.
`search_complete` là global comparison, không đại diện tất cả obligations.
Mỗi obligation có status riêng; deadline sau global proof vẫn có thể để obligation INCONCLUSIVE.
`witnesses_truncated` báo cap output (mặc định 8, tối đa 64); witness là packet đại diện,
không hứa liệt kê mọi packet hay biểu diễn region tối giản. Chưa có diagnostics engine.

### Determinism và replay

Pin engine version và logical caps. Duyệt rule theo text, dimension theo
protocol/src/dst/srcPort/dstPort, split wildcard ở bit thấp nhất cần tách; protocol
và interval theo số tăng. Sort region output theo cùng tuple khóa; chưa bắt buộc merge tối giản.
Witness chọn protocol nhỏ nhất còn lại, IP là fixed cube value, TCP/UDP port thấp
nhất trong interval; non-TCP/UDP port 0. Mọi witness vẫn phải qua evaluator.
Logical operations/boxes là budget tái lập. Wall-clock chỉ là safety stop:
ghi stop reason, timing_limited=true và search_complete=false nếu dừng sớm.
Khi replay, kiểm input/hash/policy và recheck witnesses trước. Bản gốc timing-limited
trả TIMING_SENSITIVE nếu không thể replay đầy đủ; không coi chênh lệch do deadline
là tamper hoặc full replay PASS. Timing ngoài hash; logical caps/stop reason thuộc hash.
Test bằng deadline/clock được điều khiển, không dùng tốc độ máy làm oracle correctness.

## Witness và oracle

Mỗi witness phải thuộc scope và residual, được linear evaluator xác nhận before/after khác action.
Trace chứa input side, source line/rule identity hoặc implicit deny; không chỉ có lời giải thích AI.
Evaluator không dùng subtraction. Parser/IR vẫn là dependency chung nên cần golden parser riêng.
Tests bắt buộc: first-match/reorder, implicit deny, non-contiguous wildcard, source-vs-destination port, port 0/65535;
exhaustive reduced domain đối chiếu set algebra với direct mask/linear evaluation; fuzz malformed input.

## Obligation

| Kind | Điều kiện HOLDS | Phản ví dụ khi VIOLATED |
|---|---|---|
| `must_permit` | `region \ A_after` rỗng | Packet trong phép hiệu |
| `must_deny` | `region intersect A_after` rỗng | Packet trong phần giao |
| `must_remain` | `region intersect D` rỗng | Packet trong symmetric difference |

Định nghĩa obligation là input đã canonicalize, được hash cùng kết quả; không chỉ hash `HOLDS/VIOLATED`.
Obligation có witness/trace riêng: hai ACL giống nhau vẫn có thể cùng vi phạm `must_permit`.
Region sau exclude phải không rỗng; exclude nằm trong include; thiếu phạm vi thì không tự đoán.
Đối lập giữa obligations được báo rõ, không tự chọn một điều kiện để bỏ qua.
Schema-valid hoặc HOLDS không chứng minh lời tự nhiên đã được dịch đủ/đúng.

## Repair được kiểm chứng

Policy hash bind acceptance, condition definitions và allowed-change union.
Nil allowed region là undeclared/UNCHECKED; explicit [] là không cho thay đổi gì.
Collateral tính chính xác `(A_before Δ A_candidate) \ union(allowed)`, không dùng
chỉ những witness đã được report. Non-empty residual sinh witness đã linear-evaluate.
So sánh candidate/obligation và collateral là hai lượt engine, mỗi lượt có cap
options riêng (default 2s/2M ops/50k boxes), nằm dưới deadline của session khi gọi từ AI.

Proposal pin original-before/base-after/accepted-policy digests; ref ordinal+line
được qualify bằng base-after digest, không phải global stable ID. Xem [patch contract](CONTRACTS.md).
Kiểm digest + refs, áp dụng toàn bộ trong bộ nhớ, parse lại, rồi kiểm obligations trên candidate.
Chỉ chấp nhận nếu mọi obligation bắt buộc HOLDS và collateral policy được chứng minh.
Collateral so **original before → repair candidate** ngoài allowed-change region đã được người dùng khai báo rõ.
Không dùng candidate→candidate hoặc original-after→candidate để tự chứng minh toàn change ticket an toàn.
Thiếu allowed-change region thì báo chưa kiểm chứng collateral; không gắn nhãn bản sửa đã verified đầy đủ.
Giữ bản cũ khi patch lỗi, unsupported hoặc budget cạn; không partial apply. Tối đa hai vòng sửa; không ghi thiết bị.
Receipt `plenilune.repair/v1` bind input/policy/request/candidate bytes+hash/options,
comparison và collateral. Replay phải rerender đúng candidate trước khi xử lý
TIMING_SENSITIVE; timeout không miễn kiểm byte/pin/accepted-policy binding.

## Engine v0.2 và change classes (31/10/2026)

Đo trên corpus synthetic cố định (Go 1.26.7, Windows/amd64, 8 CPU, median 5 lần,
default 2s/2M ops/50k boxes): engine v0.1 cần 1.831.170 ops cho ACL 100 rule
identical, chạm cap 2M khi chèn deny, và INCONCLUSIVE từ 250 rule. Nguyên nhân là
subtract hai permit-set đã phân mảnh. v0.2 index các cut theo covering prefix và
quyết định từng mảnh theo raw rule first-match; cùng boxes/verdict nhưng ops
giảm còn 96.894 (100 rule), 463.980 (250 rule) và các case 250 rule hoàn tất.

Version là một phần evidence: `acl-exact/v0.2` cho evidence mới,
`acl-exact/v0.1` được replay bằng đúng accounting cũ. SealComparison giữ version
do caller chỉ định; repair version tương ứng `acl-repair/v0.2|v0.1`.
Không thay đổi hash projection, witness selection hay scope; v0.2 được kiểm
box-for-box với full scan và verdict/obligation với v0.1 trên corpus.

## Engine v0.3: LCP residual (01/11/2026)

v0.3 căn chỉnh tiền tố chung dài nhất của các rule giống nhau về ngữ nghĩa
(cùng match + action; bỏ qua sequence, log, vị trí dòng) và chỉ tính delta
chính xác trên các rule còn lại (residual), trên phần bù của vùng covered —
hợp của các region thô của tiền tố. Packet được tiền tố quyết định thì cả
hai phía quyết định giống hệt nhau, nên không thuộc delta; permit set của
tiền tố không bao giờ được vật chất hóa (đây là điểm v0.2 chạm cap box ở
512 rule). Residual dùng lại đúng dãy cut mà vòng lặp v0.2 xây cho từng
rule, nên các box hiệu lực của residual và lost/gained giống v0.2 từng box.
Obligation must_permit/must_deny được đánh giá trên permit cover — các box
hiệu lực của đúng những rule có thể chạm tới vùng điều kiện — qua cùng phép
difference/intersection dạng index của v0.2; must_remain dùng lost/gained
như trước. Không có tiền tố chung thì v0.3 suy biến về đúng phép tính v0.2.

Đo trên cùng corpus cố định của cổng P1-1 (chi tiết:
`docs/benchmarks/p1-1-v03-2026-11-01.md`): identical 512 rule hoàn tất
EQUIVALENT ở 1.733 ops (v0.2 INCONCLUSIVE ở 261.044 ops, stop=boxes);
append_rule/change_last còn 589/589 ops ở 100 rule và 1.373/1.901 ops ở
250 rule (giảm trên 99% so với v0.2, cổng yêu cầu 50%); insert-at-head bằng
đúng số v0.2 ở cả hai cỡ. Ca swap/change-port ở 512 rule vẫn INCONCLUSIVE:
residual của chúng vẫn chứa các rule rộng mà phép hiệu từng rule tự nó đã
phân mảnh chạm cap — LCP chỉ giúp khi phần thay đổi nằm sau tiền tố dài.

Version vẫn là một phần evidence: `acl-exact/v0.3` cho evidence mới,
`acl-exact/v0.2` và `acl-exact/v0.1` được replay bằng đúng accounting cũ;
repair version tương ứng `acl-repair/v0.3|v0.2|v0.1`. Verdict, change class
và trạng thái obligation được kiểm differential với v0.2 trên corpus ngẫu
nhiên seed cố định (`TestP11DifferentialV03AgainstV02`, 600 ca kèm
obligation); witness của delta và của change class giống hệt; witness của
obligation có thể khác biểu diễn box nhưng luôn được evaluator tuyến tính
kiểm lại độc lập, như bước v0.1→v0.2 đã quy định.

Change class (`plenilune.acl-changes/v1`) phân hoạch chính xác symmetric
difference theo cặp quy tắc quyết định before/after. Mỗi class có witness được
evaluator tuyến tính kiểm lại, ví dụ vùng rời rạc (cap), và scope chỉ được gắn
khi caller cung cấp policy đã accept; ngoài phạm vi có witness riêng kiểm bằng
predicate. Budget cạn → không công bố class; không trình bày giải thích một phần.

Prefix-list IPv4 dùng partition interval theo từng prefix length 0–32, sweep hai
partition để lấy class; route count là số route canonical chính xác, không phải
số witness. Semantics: seq order, first match, implicit deny, ge/le theo tài liệu
Cisco; host bits bị từ chối; empty list unsupported; `len < ge <= le <= 32`.
`ge == le` được chấp nhận như giả định cần xác nhận thiết bị. Oracle exhaustive
trên reduced domain (mọi route trong /20 ở length 20–30) đối chiếu linear evaluator.

Phân tích cấu hình tách structural/behavioral: unsupported và unanalyzed parts
không có verdict; `limitations` nêu rõ không mô hình application binding. Một
`DIFFERENT` không tự là "nguy hiểm"; `EQUIVALENT` không chứng minh intent fidelity.

## Prefix-list model and replay hardening — 31/10/2026

Current prefix parser/engine v0.2 implement the [declared canonical IPv4 profile](docs/semantics/prefix-list-v02.md).
Public boundaries validate IR and source-order implicit sequences. Sequence max
is4294967294; model inclusivity is explicit, not a blanket firmware claim.
Frozen prefix v0.1 parsing/accounting remain available only for version-pinned
analysis replay. TIMING_SENSITIVE does not exempt unrelated result fields or
metadata from replay checks. It still withholds proof completion for the affected
ACL component; hash validation alone remains a producer-content check.
