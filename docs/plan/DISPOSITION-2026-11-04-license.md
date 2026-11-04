# DISPOSITION-2026-11-04 — Thay đổi license: GPL-3.0-only → Apache-2.0

Ngày: 04/11/2026 · Người quyết định: chủ dự án (Võ Mạnh Cường, chủ sở hữu
bản quyền duy nhất của Plenilune).

## Quyết định

Từ commit chứa văn bản này, Plenilune được phát hành theo **Apache License,
Version 2.0 (Apache-2.0)** thay cho GNU General Public License, version 3
only (GPL-3.0-only).

## Phạm vi hiệu lực

- Chủ dự án là chủ sở hữu bản quyền duy nhất (theo DISPOSITION-2026-10-31
  §4 và DISPOSITION-2026-11-02-p2-6), không có đóng góp bên thứ ba nào cần
  xin đồng ý lại; việc đổi license do chủ sở hữu thực hiện trên chính tác
  phẩm của mình.
- Các bản sao Plenilune đã nhận theo GPL-3.0-only trước thời điểm này giữ
  nguyên điều khoản GPL đối với người nhận chúng; quyết định này áp dụng
  cho các bản phát hành từ commit này trở đi.
- `LICENSE` chứa nguyên văn Apache License 2.0 (nguồn apache.org, 11.358
  byte, SHA-256 cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30);
  `NOTICE`, `README.md`, `README.vi.md` và các tài liệu vận hành đã cập
  nhật theo.

## Hệ quả đối với P2-6 và tích hợp MoonStorm

- Cổng rà soát pháp lý GPL của P2-6 **hết hiệu lực**: rào cản từng chặn
  phân phối MoonStorm kèm Plenilune là hệ quả của copyleft GPL, không còn
  tồn tại dưới Apache-2.0.
- Ranh giới tích hợp **subprocess + JSON có version** (P2-7,
  `docs/integration/moonstorm.md`) được **giữ nguyên** — từ nay là quyết
  định hợp đồng/kỹ thuật (cách ly tiến trình, pin schema/phiên bản), không
  còn là yêu cầu pháp lý. Mọi thay đổi ranh giới này là quyết định riêng,
  không tự động suy ra từ disposition này.
- Các nhắc tới GPL-3.0-only trong tài liệu mang tính lịch sử (kế hoạch,
  disposition cũ, bản ghi review trước ngày này) là bản ghi đúng tại thời
  điểm viết và không sửa ngược.

Văn bản này là ghi chép quyết định của chủ dự án, không phải tư vấn pháp lý.
