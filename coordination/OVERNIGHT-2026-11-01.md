# Kênh phối hợp phiên đêm 2026-11-01 — Plenilune (UI stream + Phase 2)

File này là kênh liên lạc chính thức giữa **Chí Nhân** (phía Muse, làm +
mở PR) và **agent local** (phía máy Chooky, chạy cổng Windows + merge),
theo ủy quyền trọn đêm của Chooky ngày 01/11/2026. Prompt phiên trực đêm
Chooky đã chuyển vẫn có hiệu lực; file này bổ sung cơ chế liên lạc và là
nơi hai phía ghi trạng thái cho nhau. Nếu file này và trạng thái PR trên
GitHub mâu thuẫn, **GitHub thắng**.

## Giao thức ghi (tránh xung đột)

- Mục **A** chỉ Chí Nhân ghi. Mục **B** chỉ agent local ghi. Không phía
  nào sửa chữ của phía kia.
- Mỗi lần ghi: commit chỉ file này, push lên đúng nhánh
  `coord/overnight-2026-11-01`. Trước khi ghi phải pull/fetch bản mới
  nhất. Nếu push bị từ chối vì nhánh đã đi trước: pull --rebase rồi
  push lại.
- Agent local: mỗi vòng chờ (~3 phút) kiểm tra cả hai thứ — PR mới/head
  mới trên GitHub VÀ commit mới trên nhánh coord này.
- Chí Nhân cập nhật mục A mỗi khi: mở PR mới, push head sửa lỗi, hoặc
  đổi trạng thái hàng.

## Hàng dự kiến (thứ tự xử lý)

1. `feat/ui-s1-tokens` → 2. `feat/ui-s2-s3-primitives-hierarchy` →
3. `feat/p2-1-dual-digest` → 4. `feat/p2-2-capabilities` →
5. `feat/p2-3-error-taxonomy` → 6. `feat/p2-4-bundle` →
7. `feat/p2-8-triggers` → 8. `feat/p2-5-signing` →
9. `docs/p2-7-moonstorm-spec` → cuối: manifest regen (mở PR, không merge).

Quy tắc cổng/merge như prompt phiên trực đêm: cổng đầy đủ mới merge;
FAIL thì ghi nguyên văn vào mục B, chờ head mới trên cùng nhánh rồi chạy
lại cổng; không tự sửa code.

## A — Chí Nhân → agent local

- 23:10 — PR **#25** `feat/ui-s1-tokens` @ `9bc94ec6` (base `main`) ĐÃ MỞ:
  UI lát 1 (token Neo 1+). Chờ cổng + kiểm tra hình ảnh như prompt.
- 23:10 — PR **#26** `feat/ui-s2-s3-primitives-hierarchy` @ `93fe463e`
  (base `feat/ui-s1-tokens`) ĐÃ MỞ: UI lát 2+3 gộp (giảm thẻ duyệt).
  Sau khi #25 merge, retarget PR này về `main` rồi mới chạy cổng.
- 23:10 — `feat/p2-1-dual-digest` ĐANG LÀM (chưa mở PR). Các mục sau
  chưa bắt đầu. Khi mở PR từng mục, tôi sẽ thêm dòng mới tại đây kèm
  head SHA.
- Tín hiệu kết thúc hàng: khi mục 9 đã mở PR, tôi sẽ ghi dòng
  "HÀNG ĐÃ ĐẦY — sau merge mục 9, làm manifest" tại đây.

## B — Agent local → Chí Nhân

(trống — agent local ghi kết quả cổng từng PR tại đây: PR, head đã kiểm,
PASS/FAIL + tóm tắt, merge SHA nếu đã merge; FAIL thì kèm nguyên văn
lệnh + output lỗi chính.)
