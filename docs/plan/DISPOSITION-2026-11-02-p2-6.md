# DISPOSITION 2026-11-02 — Xử lý cổng pháp lý P2-6

Người quyết định: **Võ Mạnh Cường** (Chooky) — chủ dự án, chủ sở hữu bản
quyền duy nhất của Plenilune (© Võ Mạnh Cường / Quorix Việt Nam).
Ngày: 02/11/2026. Hạng mục gốc: P2-6 trong `docs/plan/STATUS.md` và
`docs/plan/04-phase-2-contract-integration.md`.

Văn bản này là **disposition của chủ sở hữu** ghi lại cách dự án xử lý cổng
P2-6. Đây không phải ý kiến pháp lý, không do luật sư soạn, và không cấp
phép cho bất kỳ hành động phân phối nào.

## 1. Bối cảnh

- Plenilune được phát hành dưới GPL-3.0-only — giấy phép do chính chủ sở
  hữu chọn cho code của mình.
- MoonStorm là dự án riêng của cùng chủ sở hữu. Tích hợp giữa hai dự án
  chỉ qua ranh giới subprocess + JSON có version
  (`plenilune.analysis/v2`, đặc tả P2-7); không import Plenilune như thư
  viện Go.
- Tính đến ngày ký văn bản này: Plenilune do chủ sở hữu (và các công cụ
  làm việc dưới sự chỉ đạo của chủ sở hữu) viết toàn bộ; **chưa từng nhận
  đóng góp code từ bên thứ ba**. Chưa có bất kỳ đợt phân phối nào của
  MoonStorm kèm Plenilune.

## 2. Cơ sở của disposition

GPL là giấy phép chủ sở hữu bản quyền cấp cho *người khác*; nó không ràng
buộc chính chủ sở hữu đối với tác phẩm của mình. Khi tác phẩm có một chủ
sở hữu duy nhất, chủ sở hữu giữ toàn quyền dùng tác phẩm theo cách khác —
gồm việc dùng Plenilune cùng MoonStorm — mà không cần tự xin phép chính
mình, và việc phát hành công khai dưới GPL cho cộng đồng không thu hẹp
quyền đó. Ranh giới subprocess + JSON có version được giữ nguyên như lớp
phòng thủ kỹ thuật và là mẫu thuận lợi nhất theo FSF FAQ, độc lập với
disposition này.

## 3. Quyết định

- P2-6 đóng ở trạng thái **đã xử lý theo disposition của chủ sở hữu**:
  không thuê rà soát pháp lý ở thời điểm hiện tại, vì đối tượng rà soát
  (nghĩa vụ của chủ sở hữu đối với chính code của mình khi chưa phân
  phối) hiện không tồn tại.
- **Phân phối MoonStorm kèm Plenilune vẫn bị chặn** như trước: disposition
  này không phải giấy phép phân phối. Trước bất kỳ đợt phân phối nào cũng
  phải có rà soát pháp lý bằng văn bản, hoặc một disposition mới thay thế
  văn bản này.

## 4. Ba mốc kích hoạt — gặp một mốc là thuê luật sư trước khi đi tiếp

1. Nhận đóng góp code từ bên thứ ba vào Plenilune (khi đó tác phẩm không
   còn một chủ sở hữu duy nhất, trừ khi có CLA/chuyển nhượng bản quyền đi
   kèm từng đóng góp).
2. Có thêm co-founder, nhà đầu tư, hoặc đồng sở hữu ở Plenilune hoặc
   MoonStorm.
3. Phân phối thương mại cho bên thứ ba ở quy mô thật.

## 5. Điều kiện vô hiệu

Nếu bất kỳ sự kiện nào ở mục 1 thay đổi — đặc biệt là tính duy nhất của
chủ sở hữu bản quyền — văn bản này tự hết hiệu lực, P2-6 mở lại ở trạng
thái `todo`, và mốc kích hoạt tương ứng ở mục 4 áp dụng.
