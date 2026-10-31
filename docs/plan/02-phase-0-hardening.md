# Phase 0 — Hardening dữ liệu & local API

Điều kiện vào: Chooky chốt kế hoạch + `DECISION P0-D1`. Điều kiện ra: cả ba hạng
mục đạt nghiệm thu. Nguồn: báo cáo audit Qoder mục 1/5 + báo cáo đối chiếu mục 6
(toàn bộ đã kiểm chứng trên public `main` 31/10/2026; vẫn phải đối chiếu lại
baseline theo protocol trước khi sửa).

## DECISION P0-D1 — Schema cho field đã redact (Chooky quyết trước P0-1)

Thay `SectionChange.Header` bằng dạng đã redact là đổi schema artifact
`plenilune.analysis/v1`. Lựa chọn: (a) bump schema version mới; (b) giữ version,
field cũ để rỗng/deprecated. Khuyến nghị của kế hoạch: **(a) bump version**,
vì caller như MoonStorm pin schema theo version — thay nội dung lặng lẽ dưới
cùng một version làm vỡ giả định pin. Agent không tự chọn.

## P0-1 — Redact secret khỏi artifact và UI

**Vấn đề (đã kiểm chứng):** `SectionChange.Header` nhận nguyên `b.header.text`
(`internal/analysis/analysis.go:110, 542-550`); `Reference.Text` nhận nguyên dòng
đã trim (`internal/analysis/intake.go:344`); UI render thẳng header
(`web/src/components/ConfigOverview.tsx:102`); repair receipt chứa toàn văn ACL
candidate (`internal/repair/verify.go:34, 107`). Dòng kiểu
`snmp-server community …`, `username … secret`, `enable secret` trong section
chưa phân tích sẽ đi nguyên vào artifact/ticket.

**Làm:**
1. Thay `Header` bằng `HeaderKind` + số dòng + digest; tối đa giữ hai token đầu
   (ví dụ `interface Gi0/1`, `snmp-server community` — không giữ community value).
2. `Reference` chỉ giữ `line`, `kind`, `name`; bỏ `Text`. UI hiển thị thì lấy
   text từ source local và mask dòng nhạy cảm; không lấy từ artifact.
3. Đánh dấu artifact chứa repair candidate là sensitive (nhãn trong artifact/
   docs). Mã hóa at-rest phía MoonStorm là việc Phase 2; ở đây chỉ gắn nhãn.
4. Report/ticket xuất ra dùng bản đã redact, không dùng raw header.

**Không làm:** không đổi source gốc, verdict, hay cơ chế replay.

**Nghiệm thu:**
- Test synthetic chứa `snmp-server community SECRET` và `username admin secret …`:
  chuỗi secret không xuất hiện trong artifact JSON lẫn UI render.
- Replay từ source gốc vẫn PASS.
- Cổng kiểm chứng đầy đủ (protocol mục 2.4) xanh; CONTRACTS/README cập nhật
  theo quyết định P0-D1.

## P0-2 — Siết local API

**Vấn đề (đã kiểm chứng):** chỉ endpoint AI kiểm Host/Origin (`localAIRequest`,
`internal/demohttp/session.go:54`); các route parse/analyze/compare/repair/replay
(`internal/demohttp/server.go`) không kiểm — loopback không phải security
boundary (DNS rebinding). Server chỉ có `ReadHeaderTimeout: 5s`, `IdleTimeout:
60s` (`cmd/plenilune-demo/main.go:35`); không giới hạn concurrency.

**Làm:**
1. Host allowlist (`localhost`, `127.0.0.1`, `[::1]` đúng port) cho **mọi**
   `/api/*`; kiểm thêm `Origin` và `Sec-Fetch-Site`; từ chối cross-site.
2. Thêm `ReadTimeout`, `WriteTimeout`, `MaxHeaderBytes`, `http.MaxBytesReader`;
   giữ `Content-Type: application/json`; không thêm CORS; giữ chỉ loopback.
3. Semaphore giới hạn phân tích đồng thời (bắt đầu ở 2); vượt quá trả `429/503`
   kèm error code typed, không xếp hàng vô hạn.
4. Ghi rõ trong docs: đây là giảm thiểu localhost, không phải multi-user security.

**Nghiệm thu:** request Host lạ/cross-origin tới route analyze bị chặn; request
hợp lệ từ localhost chạy bình thường; slow-body/đồng thời cao không giữ
connection/CPU vô hạn; test cho cả đường chặn và đường hợp lệ.

## P0-3 — UX vi mô (không đổi engine)

1. Panel "Được dùng bởi": render `references` đã có trong artifact (hiện chỉ ở
   `web/src/lib/types.ts:452`). Ghi rõ chỉ là tham chiếu cấu trúc; không mô hình
   hướng in/out hay binding thật.
2. Bỏ framing tỉ lệ: thay `'Conditions passed: {holds}/{total}'`
   (`web/src/i18n/messages.en.ts:177` và bản VI tương ứng) bằng dạng liệt kê
   "OB-1 violated · OB-2 holds · OB-3 holds" / "1 violated, 2 hold".
3. Ticket summary deterministic: template Markdown VI/EN từ artifact — verdict,
   witness, rule, obligations, repair, limitations, hash, lệnh replay. JSON vẫn
   là authority; summary không chứa secret/raw header.
4. Object `equivalent` ghi rõ: "Text changed; behavior equivalent in modeled
   scope", phân biệt với `unchanged`.

**Nghiệm thu:** UI test/ảnh cho 4 mục; không phát sinh request AI; JSON artifact
không đổi ngoài phần P0-1.
