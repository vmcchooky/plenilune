# Brief cải tiến giao diện Plenilune — chuẩn thế giới (luồng việc UI, Mục A sở hữu)

Ban hành: 2026-11-03 (Chí Nhân, theo ủy quyền của Chooky cùng ngày). Tài liệu
này sống trên branch coord; bản thi hành nằm ở các PR web trên repo chính.

## Mục tiêu

Đưa giao diện web Plenilune đạt chuẩn sản phẩm thế giới ở các mặt đo được:
khả năng truy cập, tính nhất quán hệ thống thiết kế, mật độ thông tin, hiệu
năng cảm nhận — đồng thời giữ nguyên những gì chủ sở hữu đã khóa.

## Bất biến (không được phá)

1. **Một họ gradient duy nhất Neo 1+**: `#E8F5FD → #F2ECF5 → #FCEBE1`, điểm
   giữa lavender `#F2ECF5` chỉnh tay; accent `#2E6484` (strong `#245269`).
   Ngữ nghĩa verdict nằm ở badge/accent, KHÔNG bao giờ đổi nền theo verdict.
2. **Luồng verdict-first** theo fast-flow đã duyệt: màn hình chính trả lời
   "khác hay không khác, vì sao" trước; chi tiết qua progressive disclosure.
   Không thay đổi luồng, không thay đổi engine/API/hợp đồng dữ liệu.
3. **Không trông AI-generated**: cấm polish SaaS generic (font hệ thống mặc
   định cho mọi thứ, icon hình tròn tô đặc, bóng mềm, bo góc phồng). Thiết bị
   nghệ thuật phải đặc thù sản phẩm — bản redesign đang ở main đã có
   ChangeRecord / MoonMark / receipt hash-seal: XÂY TIẾP trên đó, không thay
   toàn bộ, không quay lại các concept muse-ui đã bị bác bỏ (branch đã xóa).
4. Hệ thống sẵn có trong `web/DESIGN.md` (thang chữ 6 bậc 12/13/14/16/20/28,
   spacing 4/8, một easing, motion 160/240 ms) là nền; mọi lệch chuẩn phải
   ghi lý do trong PR.

## Chuẩn đích (đo được)

- **WCAG 2.2 AA**: tương phản chữ ≥ 4.5:1 (chữ lớn 3:1), focus visible trên
  mọi phần tử tương tác, điều hướng bàn phím trọn vẹn, `prefers-reduced-motion`
  được tôn trọng, axe-core 0 vi phạm trên các màn chính.
- **Không tràn ngang** ở 8 bề rộng kiểm tra (360 → 1920) trên mọi màn —
  overflow = 0 như các vòng QA trước đã đo.
- **Mục tiêu chạm** ≥ 44×44 px cho hành động chính trên mobile.
- **Hiệu năng cảm nhận**: màn verdict không layout shift khi dữ liệu về;
  skeleton/placeholder giữ nguyên khung.
- Mọi trạng thái đều có mặt: loading / empty / error / partial
  (INCONCLUSIVE) — nhất quán giữa các màn.

## Hai khiếm khuyết P0 (từ verdict của chủ sở hữu, 2026-11-02)

1. **Editor cấu hình cắt dòng ACL dài giữa token và cuộn ngang** ("xấu dã
   man"). Cần xử lý dòng dài tử tế: soft-wrap có số dòng, hoặc pan có kiểm
   soát + minimap/ruler — chọn theo bằng chứng screenshot, ghi lý do.
2. **Trang verdict inbox phần lớn là khoảng trắng chết**, form watch chật và
   wrap xấu. Cần mật độ + bố cục lại: danh sách verdict là nhân vật chính,
   form lùi về phụ.

## Lát cắt (mỗi lát = 1 PR riêng, gate npm + screenshot trước/sau)

- **U1 — AUDIT (chỉ đo, không sửa code)**: chụp inventory mọi màn (quick,
  review, verdict inbox, audit, chat) ở 8 bề rộng; chạy axe; lập bảng lệch
  chuẩn theo mục "Chuẩn đích" + hai P0; báo cáo vào Mục B kèm link/ảnh.
  Chí Nhân duyệt thứ tự ưu tiên sau U1.
- **U2 — Dòng dài trong editor** (khắc phục P0-1) trên quick + review.
- **U3 — Verdict inbox**: mật độ, bố cục, form watch (khắc phục P0-2).
- **U4 — Hierarchy pass**: chữ/khoảng trên quick + review theo thang đã khóa,
  dọn lệch chuẩn còn lại từ bảng U1.

## Kỷ luật PR UI

- Chỉ đụng `web/` (+ test web). KHÔNG đụng `internal/`, `docs/plan/`,
  STATUS.md, manifest.
- PR kèm: screenshot trước/sau (cùng viewport), kết quả axe, overflow 8 bề
  rộng, npm test/build xanh.
- Merge chỉ khi Mục A có dòng **UI-MERGE OK** của Chí Nhân cho PR đó (cổng
  vị giác ở lại với Chí Nhân theo phân vai chủ sở hữu đã giao).
