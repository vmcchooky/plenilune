# Plenilune — thứ tự phát triển tiếp theo

Ngày 30/10/2026; cập nhật 31/10/2026. Mục tiêu: **nhận thay đổi → hiểu hành vi →
duyệt điều kiện → kiểm chứng bản sửa → người khác phát lại được**. D0–D4 đã
implemented; D5 offline browser QA đã chạy ngày 31/10. Live UI với Muse vẫn là
qualification riêng. Trạng thái hiện hành thuộc [README](README.md), thiết kế sản
phẩm chi tiết và acceptance từng slice tại [product UX](docs/design/product-ux.md),
handoff tại [product-ux-next](docs/handoff/product-ux-next.md).

## Baseline

Exact comparison, independent obligations, typed patch, collateral toàn vùng,
comparison/repair replay và Meta tool loop đã implemented. UI VI/EN gọi runtime,
sample là input synthetic. Một ca backend Contributor live đã completed và
repair replay matched; đây không phải UI-live mới. Lượt 31/10 đã chạy 142 browser
checks offline (Brave, VI/EN, 320/390/1440, keyboard, dark) và soát ảnh public;
live UI/Muse, screen reader và thiết bị thật chưa qualified. Public prep có
walkthrough, SECURITY, LICENSE/NOTICE và manifest; license đã chốt GPL-3.0-only,
contact đã gán; remote push vẫn là hành động của chủ dự án.

## Các slice và nghiệm thu tiếp

| Thứ tự | Owner | Đầu ra |
|---|---|---|
| D0: candidate identity | Frontend state/repair owner | Late response sau sửa/xóa edit hoặc đổi candidate không publish receipt/replay cũ. |
| D1: khám phá trước policy | Frontend flow/presentation | Hai ACL đủ xem delta thật, không cần intent/conditions trước; headline nêu hành vi cụ thể. |
| D2: preview rồi duyệt | HTTP adapter + frontend policy | Canonical full policy được xem trước approval; endpoint implemented ở CONTRACTS §7; input/policy revisions rõ, nil khác []. |
| D3: phiếu quyết định | Frontend result/repair/report | Global/obligation/collateral riêng; nhận/giữ lại/từ chối candidate là quyết định người dùng, mismatch chặn nhận. |
| D4: hồ sơ review độc lập | Frontend artifact UX | Exact input bytes, receipt đúng loại, hướng dẫn và fresh-session import/replay, không provider. |
| D5: chốt claims và QA | Integration/reviewer | Offline synthetic path, failure/race cases, Go/UI checks và browser QA mới; live Muse end-to-end là qualification riêng sau này khi được phép. **Trạng thái 31/10:** offline browser QA 142/142 PASS; live UI chưa chạy; public snapshot sẵn sàng review, license GPL-3.0-only, chưa push. |

Một owner tích hợp state; AI khác có thể làm adapter, templates VI/EN và artifact
UX theo paths ở thiết kế. Review semantics/evidence độc lập. Không bắt đầu packet
editor tự do hay nhiều candidate khi đường SSH chính chưa hoàn tất.

## Sau hackathon

Ưu tiên theo phản hồi: packet probe ba phiên bản, typed diagnostics tại HTTP
adapter, so hai candidate cùng inputs/policy/options, intent ambiguity corpus và
screen-reader/real-device QA. Giữ core deterministic, Go stdlib và boundaries.
Không thêm service/database/queue/framework/plugin system nếu chưa có nhu cầu.

Chỉ `muse-spark-1.3-contributor`; không đổi model để che thiết kế hoặc qualification lỗi.
Nếu provider chưa đạt vẫn có demo offline/manual đầy đủ, gắn nhãn trung thực.
Không tự push/publish/deploy, apply thiết bị hay triển khai MoonStorm; license đã
chốt GPL-3.0-only và mọi thay đổi license cần chủ dự án quyết định.
