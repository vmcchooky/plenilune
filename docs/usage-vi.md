# Cách dùng Plenilune (VI)

> Hai walkthrough synthetic đầy đủ, có ảnh và lệnh replay:
> [ACL đổi ngoài ý muốn và prefix-list đổi vùng route](walkthrough-vi.md).
> Bản tiếng Anh: [walkthrough-en.md](walkthrough-en.md).

## 1. Review hai phiên bản cấu hình

Mở giao diện local (`scripts/start-local.ps1`), ở bước **Nguồn & ý định**:

1. Dán cấu hình trước vào ô **Cấu hình trước**, cấu hình sau vào **Cấu hình sau**.
   Hoặc bấm **Mở tệp…** để đọc nguyên byte từ running-config/backup (UTF-8, tối đa
   4 MiB mỗi bên). Có thể dán một ACL riêng như trước đây.
2. Giữ hoặc sửa **ý định** nếu bạn muốn dùng AI/điều kiện sau này. Ý định không
   ảnh hưởng tới việc so sánh hành vi.
3. Bấm **Xem hành vi thay đổi**.

**Tổng quan thay đổi cấu hình** xuất hiện trước: đối tượng nào đổi hành vi, đối
tượng nào chỉ đổi văn bản/không đổi, đối tượng ngoài phạm vi hỗ trợ, và phần cấu
hình **không được phân tích**. Không có kết luận nào cho phần ngoài phạm vi.

Nếu chỉ có một đối tượng ACL phân tích được, Plenilune tự mở chi tiết như luồng
cũ. Với nhiều đối tượng, bấm **Xem chi tiết** cạnh đối tượng cần review.

## 2. Đọc kết quả ACL

- **Mới bị chặn / Mới được phép**: mỗi nhóm là toàn bộ gói tin được quyết định
  bởi cùng một cặp quy tắc trước/sau. Số hiển thị là số nhóm, không phải số packet
  hay phần trăm an toàn.
- **Trước / Sau**: quy tắc quyết định, kèm dòng trong đúng tệp cấu hình (không
  phải dòng trong block đã cắt).
- **Gói ví dụ** đã được evaluator độc lập kiểm lại.
- **Thử gói này** mở form probe. Sửa IP/cổng/giao thức rồi bấm **Kiểm tra gói**
  để xem quyết định trên trước/sau/candidate. Probe chỉ giải thích một điểm,
  không chứng minh tương đương.
- Nếu đã duyệt policy/scope ở bước Điều kiện, mỗi nhóm có thêm nhãn **Trong /
  Ngoài phạm vi được phép** và witness ngoài phạm vi. Nhãn này chỉ xuất hiện khi
  policy đã được operator chấp nhận và bind đúng input.

## 3. Đọc kết quả prefix-list

Chọn một đối tượng **Prefix-list**. Giao diện nói theo route:

- **Route mới được chấp nhận / bị lọc**, số route canonical chính xác.
- Độ dài bị ảnh hưởng, dải `first → last`, seq quyết định trước/sau, route ví dụ.
- Prefix-list hiện **chưa hỗ trợ intent/condition/repair**; màn hình nói rõ điều
  này thay vì giả lập packet/rule ACL.

## 4. Bàn giao cho đồng nghiệp

Bước **Bằng chứng**:

- **Cấu hình trước/sau nguyên byte** (`before.cfg`, `after.cfg`), nếu đang review
  cấu hình đầy đủ; cùng `before.acl`/`after.acl` cho block ACL.
- **analysis.json** (`plenilune.analysis/v1`) là artifact gốc; report dịch chỉ là
  view.
- **replay-instructions.md** chứa lệnh replay.
- **Phát lại hồ sơ phân tích** chạy lại trên máy; trạng thái `ANALYSIS_REPLAYED`
  chỉ có nghĩa nội dung/phiên bản khớp, không chứng minh tác giả.
- Người nhận có thể mở phiên sạch, kéo hai tệp cấu hình + `analysis.json` vào
  **Nhập hồ sơ để phát lại**, không cần AI.

CLI tương đương:

```sh
plenilune analyze --before before.cfg --after after.cfg
plenilune analyze-replay --before before.cfg --after after.cfg --evidence analysis.json
```

Với caller tích hợp (MoonStorm tương lai), dùng JSON qua stdin:

```sh
plenilune analyze --request - < request.json
plenilune analyze-replay --request - < replay-request.json
```

`ref` trong request chỉ là nhãn tương quan do caller đặt, không phải identity.
Client mẫu: `examples/moonstorm-client/client.mjs`.

## 5. Điều không được suy ra

- `DIFFERENT` không tự động là “nguy hiểm”; `EQUIVALENT` không chứng minh đúng ý
  định; lỗi không tự động là “không an toàn”.
- Phần **unsupported** hoặc **không phân tích** không có verdict.
- Plenilune không mô hình nơi ACL/prefix-list được gắn (interface, BGP neighbor).
- Budget cạn hoặc `INCONCLUSIVE` không phải kết luận an toàn.
- Không dùng số witness/nhóm làm số packet bị ảnh hưởng hoặc phần trăm an toàn.

## 6. AI

AI mặc định tắt. Chỉ khi bạn bấm gửi và server đã có key, Plenilune gửi **ACL đang
xem** và ý định — không gửi toàn bộ running-config, không gửi key về browser.
Model cố định `muse-spark-1.3-contributor`, không fallback. AI đề xuất; người dùng
duyệt; engine kiểm chứng.
