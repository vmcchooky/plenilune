# Phase 3 — Mở rộng miền & hoàn thiện Production

Điều kiện vào: Phase 2 đạt điều kiện ra. Phase này có hai nửa độc lập tương đối:
3A (hoàn thiện Production — bắt buộc cho tiêu chí ra E8/E9) và 3B (mở rộng
miền — theo nhu cầu corpus thật, mỗi miền là một hạng mục riêng có cổng riêng).
Nguồn: nghiên cứu Q3/Q5/Q6/Q8 + ROADMAP hiện tại của dự án.

## 3A — Hoàn thiện Production

### P3-1 — Release train
- Version chính thức (semver cho binary + schema versions giữ nguyên cơ chế
  pin hiện tại), CHANGELOG, tag phát hành; thay mục "Supported versions" trong
  SECURITY.md (hiện ghi chưa có release train) bằng chính sách hỗ trợ cụ thể.
- Build phát hành từ checkout sạch, có VCS stamp (gắn với P2-5); quy trình
  public-snapshot + secret-scan hiện tại giữ nguyên và là cổng bắt buộc.
- Ma trận kiểm chứng trước release: cổng guardrails đầy đủ + fuzz ngắn + browser
  QA offline + replay của mọi receipt version còn hỗ trợ.

### P3-2 — Qualification còn thiếu (trung thực, không thổi)
- Live UI với model: chạy qualification riêng như dự án đã định (synthetic
  end-to-end qua UI thật), ghi kết quả/limits/latency thật; chưa đạt thì giữ
  nhãn chưa qualify.
- Screen-reader pass thật (repo tự ghi chưa làm) + thiết bị thật cho UI;
  sửa finding theo mức độ, ghi lại phạm vi đã phủ.
- Nghiên cứu người dùng nhỏ cho luồng review (đề xuất từ nghiên cứu Q6:
  5–8 operator, 3 ca seed, UI hiện tại làm đối chứng) — chỉ khi Chooky muốn đầu
  tư; không chặn release.

### P3-3 — Oracle khác họ trong CI (SMT differential)
Dùng Batfish (hoặc Z3 trực tiếp) **ngoài runtime** làm oracle đối chiếu: cùng
input, so verdict/witness của Plenilune với engine khác họ trên corpus ACL tổng
hợp sinh tự động; bất đồng → điều tra, không tự tin bên nào đúng sẵn. Đây là
công cụ bắt lỗi engine, không phải tính năng người dùng; không thêm dependency
vào core/bản phát hành.

### P3-4 — Stale-decision invalidation (UX)
Mở rộng cơ chế bind của receipt sang **quyết định của người**: input, obligation
hoặc policy thay đổi sau khi quyết định (nhận/giữ/từ chối) được ghi → quyết
định thành `stale`, UI nói rõ và chặn dùng quyết định stale làm căn cứ nhận
candidate. (Mượn mẫu hình stale-approval của GitHub/GitLab.)

## 3B — Mở rộng miền (thứ tự cố định; mỗi miền qua cổng chung)

**Cổng chung cho mọi miền mới:** parser fail-closed (không đoán semantics),
corpus golden files trước khi code chạy thật, oracle reduced-domain hoặc
differential, inventory trung thực (unsupported ghi rõ), tài liệu semantics
riêng ghi giả định cần xác nhận thiết bị. Không miền nào được lặng lẽ mở rộng
scope của verdict cũ.

| Thứ tự | Miền | Ghi chú |
|---|---|---|
| P3-5 | Standard ACL (Cisco IOS) | Rẻ nhất; hoàn thiện miền Cisco hiện tại. |
| P3-6 | Object-groups | Thiết kế theo dữ liệu prototype: ưu tiên hợp nhất trong IR (union) thay cho bung expansion nếu đo cho thấy expansion nổ tổ hợp; quyết định ghi vào tài liệu semantics. |
| P3-7 | IPv6 ACL | Thay đổi độ rộng đại số (cube 128-bit), không phải semantics mới; implicit rules của IPv6 phải xác nhận theo tài liệu/thiết bị, không suy từ IPv4. |
| P3-8 | Thư viện obligation CIS | Chỉ phần CIS Cisco IOS Benchmark biểu diễn được bằng obligation ACL; mã hóa obligation + trích ID khuyến nghị, **không phân phối lại full text** (có license CIS); công bố bảng encode được/không encode được. |
| P3-9 | Route-maps | Chỉ có ý nghĩa đầy đủ khi có application binding; trước đó, nếu làm, chỉ được trình bày là diff hàm lọc, không phải tác động routing. DECISION riêng của Chooky. |
| P3-10 | Vendor thứ hai | DECISION riêng. Ứng viên rẻ hơn nhiều khả năng là MikroTik (cú pháp đều, tài liệu mở) hơn FortiGate. Yêu cầu trước khi bắt đầu: corpus synthetic + người/tài liệu xác nhận semantics; không bắt đầu khi corpus Cisco chưa hoàn chỉnh. |

## Việc chỉ làm khi dữ liệu buộc phải (không lên lịch sẵn)

- **Coalescing/streaming sâu hơn cho engine:** sau LCP (P1-1), nếu corpus đo
  được vẫn còn ca INCONCLUSIVE vì live-box ở thay đổi đầu ACL, mới mở việc này
  với cổng benchmark riêng.
- **Atomic predicates:** chỉ khi profiling sau LCP cho thấy fragmentation vẫn
  là nút thắt chính; prototype bounded, tiêu chí giết đăng ký trước (giảm peak
  live boxes ≥30% hoặc hoàn tất ca 512 rule còn dở), không đạt thì bỏ.
- **Parse cache / prefix-list common-prefix:** chỉ khi benchmark trên dữ liệu
  lớn (ví dụ prefix-list 10.000 entry) chứng minh cần.
- **OSS-Fuzz:** sau khi fuzz local (P1-6) sạch ổn định và Chooky muốn công khai
  quy trình bảo mật ở mức đó.

## Điều kiện ra của toàn kế hoạch (đối chiếu master plan)

Đạt E1–E10 ở mức đã công bố trung thực trong tài liệu dự án: có release train
chạy thật, contract tích hợp đóng băng, qualification ghi đúng phạm vi, và —
nếu MoonStorm phân phối kèm Plenilune — cổng pháp lý đã qua. Tới đó, ROADMAP.md
của dự án được viết lại từ kế hoạch này và `docs/plan/` chuyển thành lịch sử.
