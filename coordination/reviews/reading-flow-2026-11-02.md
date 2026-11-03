# Audit đọc hiểu & chất lượng sản phẩm — 02/11/2026

Người thực hiện: OpenCode (mục C) · Độc lập với Codex/ZCode, chỉ đọc và đo.
Bản dựng đo ban đầu: `dist` phục vụ tại `http://127.0.0.1:8084` lúc **23:38–23:44
02/11/2026** (`web/dist/index.html` ghi 23:38:41 — đã có P2 + P5, chưa có gate
P2/P5). Re-probe P3/P4 dùng `dist` build **00:36:54 03/11/2026**.
Phạm vi: quick + deep, VI, 1440/390/320; các trạng thái mẫu `budget-exhausted`,
`unsupported-syntax`, `blank`; watch drawer mở/đóng; keyboard/focus.
Không sửa mã nguồn, không chạy build. Số liệu thô ở `coordination/reviews/`,
ảnh ở `D:\Quorix\plenilune\night-20261102\audit\`.

## Kết luận ngắn

- **WCAG A/AA (axe): 0 vi phạm** ở quick, deep, quick+watch mở, 1440 và 390.
  Best-practice: **1 vi phạm** — `.mode-bar` nằm ngoài landmark (F12).
- **Tương phản: đạt toàn bộ** 19 mẫu, thấp nhất **5.08:1** (badge verdict),
  cao nhất 13.35:1. Không có mẫu nào dưới 4.5:1.
- **Không tràn ngang** ở 1440/390/320, cả quick và deep (`scrollWidth = clientWidth`).
- **P2 đã sửa đúng 2 lỗi P1:** measure quick từ 1134px → 517px (≈62ch) và route
  mobile đã xoay mũi tên ↓, xếp dọc gọn. Ghi nhận, không còn trong danh sách.
- 18 phát hiện còn lại chủ yếu là **hệ thống thị giác, ngữ nghĩa trạng thái và
  cấu trúc heading** — đúng chất "study card" chứ chưa thành "sản phẩm".
  Nặng nhất: F1 (partial bị giấu), F14 (record 519px lạc trong khung 928px),
  F2 (banner warn trung tính), F3 (lỗi parser tiếng Anh, không có CTA chính).

## Bảng ưu tiên

| # | Phát hiện | Mức | Đề xuất vào |
|---|---|---|---|
| F1 | Danh sách "Phần chưa quyết định" bị giấu trong details đóng | Cao | P2.1 |
| F2 | Banner `--warn` trông trung tính, không khác banner thường | Cao | P3 |
| F3 | Trạng thái unsupported: chuỗi parser tiếng Anh, không CTA chính, không live region | Cao | P3/P4 |
| F4 | Touch target 21px/40px/38px dưới chuẩn 44px | Vừa | P3 |
| F5 | Watch drawer đặt trước H1; tiêu đề "Theo dõi & thông báo" lặp 2 lần | Vừa | P4 |
| F6 | Mobile ẩn hoàn toàn trạng thái máy chủ/AI | Vừa | P3 |
| F7 | Prose deep 804px (~100ch) ở `.witness-detail` | Vừa | P4 |
| F8 | Cổng nguồn wildcard hiển thị `:0` | Vừa | P4 |
| F9 | Rail mobile tự cuộn, giấu hẳn bước 01–02, không affordance | Vừa | P3/P4 |
| F10 | Form inbox thưa, một hàng ngang, thiếu nhóm điều kiện | Vừa | P4 |
| F11 | Chọn mẫu xong vùng kết quả trống, phải bấm chạy | Thấp | P4 |
| F12 | `.mode-bar` ngoài landmark (axe best-practice) | Vừa | P3 |
| F13 | Skip link đưa focus về BODY, không vào `main` | Vừa | P2.1/P3 |
| F14 | Record quick 519px lệch trái trong khung 928px, gutter trống 223px | Cao | P2.1/P3 |
| F15 | Deep lặp H2 "Gói tin minh chứng" ở section và detail | Thấp | P4 |
| F16 | Error panel không có "Thử lại", copy hướng dẫn dev, lệch cột 928 | Vừa | P4 |
| F17 | Hàng đối tượng overview chỉ có nhãn nhìn thấy "Xem chi tiết", không click cả hàng | Thấp | P4 |
| F18 | Badge verdict là enum tiếng Anh; "Đã tìm hết phạm vi: Có" cạnh banner "chưa phân tích đầy đủ" | Thấp | P4 |

## Trạng thái sau đêm P3–P4 (03/11/2026)

Đo lại trên `dist` **00:36:54**; P3 10/10 PASS, npm test 19 file/141 PASS,
browser suite 35/35, axe A/AA 0, best-practice 0, contrast min 5.08:1, overflow 0
tại 320/390/720/721/960/961/1024/1280/1440 và EN/200%/reflow 720.

| # | Kết quả | Ghi chú kiểm chứng độc lập |
|---|---|---|
| F1 | **PASS** | partial `visible:true`, ngoài `details`; INCONCLUSIVE không-partial vẫn đóng |
| F2 | **PASS** | banner `--warn-soft` khác `--surface-2`, có icon/role |
| F3 | **Một phần** | clause Việt + code, raw message chỉ còn `title`/technical; vẫn **không có live region**, CTA là summary "Xem / sửa cấu hình" sẵn có chứ không focus thẳng dòng lỗi |
| F4 | **Một phần** | quick 390/320 sạch; **deep còn 6 target nhỏ**: button "Thử gói này" 96×40, 5 summary 21px, checkbox 16×16 |
| F5 | **Một phần** | H2 trùng đã thành `sr-only`; **drawer vẫn nằm trước H1** (y≈138 vs H1 y≈621) |
| F6 | **PASS** | 390/320 còn tín hiệu tin cậy dạng text |
| F7 | **PASS** | prose deep 513px (~68ch), không còn 804px |
| F8 | **PASS** | "192.0.2.10 (any)" thay `:0` |
| F9 | **Một phần** | có scroll-snap; **không có affordance thấy được** (bước 01–02 vẫn ngoài màn hình, không cue "2/5") |
| F10 | **Hoãn** | ZCode ghi rõ hoãn sang P-M; fieldset vẫn thiếu `legend` |
| F11 | **Hoãn** | chọn sample `budget-exhausted` vẫn trống vùng kết quả, không hint |
| F12 | **PASS** | axe best-practice 0, mode bar có region |
| F13 | **PASS** | skip link focus vào MAIN |
| F14 | **PASS** | 720px, `axisDelta=0` tại 1440/1024; 721–959 record chạm trục |
| F15 | **PASS** | còn 1 H2 "Gói tin minh chứng" |
| F16 | **Một phần** | "Thử lại" chạy đúng, copy hết hướng dẫn dev; **panel vẫn 1344px**, chưa cap 928 như RELEASE ghi |
| F17 | **Hoãn** | nhãn thị giác vẫn "Xem chi tiết"; chưa click cả hàng |
| F18 | **PASS** | badge "Khác biệt"/"Tương đương"; enum gốc còn trong technical detail + inbox |

Tổng: **10 PASS · 5 một phần · 3 hoãn**. Không có hồi quy P3. Các mục một phần
và hoãn đủ nhỏ để gom vào P-M, trừ F5/F9/F16 nên sửa trước taste gate vì ảnh
hưởng trực tiếp cảm giác "sản phẩm" của màn vàng.

## Chi tiết

### F1 — "Phần chưa quyết định" vẫn bị giấu sau details đóng (Cao, P2.1)
Trạng thái `budget-exhausted`: banner "Engine dừng vì hết ngân sách…" hiện đúng,
nhưng `.partial-result` (`role="note"`) nằm trong `.quick-outcome__details` đang
đóng — `detailsOpen: false`, `partialVisible: false`. Người đọc thấy "không phải
kết luận an toàn" nhưng **không thấy phần nào chưa được chứng minh** trừ khi tự
mở disclosure.
Đề xuất: render `<PartialNotice>` **ngoài** details khi `partial != null` (ngay
dưới banner), hoặc auto-open details riêng cho trạng thái INCONCLUSIVE + partial;
giữ danh sách "Hành vi bị mất / Hành vi mới — chưa quyết định" trong vùng nhìn
thấy. Không đổi semantics dữ liệu.
Bằng chứng: `night-20261102/audit/reprobe-p2.json` → `budget`; ảnh
`budget-partial-1440.png`, `verify-budget.png`.

### F2 — Banner cảnh báo trông như banner thường (Cao, P3)
Đo được: `.banner--warn` → `background: rgb(238,232,220)` = `--surface-2`,
`border-left: rgb(223,215,200)` = `--line`; trong khi `.partial-result` dùng
`rgb(240,230,209)` = `--warn-soft`. Hai mức cảnh báo khác nhau nhưng banner
"nguy hiểm" lại nhạt hơn.
Đề xuất trong `app.css`:
```css
.banner--warn {
  background: var(--warn-soft);
  border-left: 2px solid var(--warn);
  color: var(--ink-2);
}
```
Thêm icon `alert` 16px (đã có primitive `Icon`) và giữ `role="status"`. Cân nhắc
`--warn-soft` cho nền + `--warn` cho icon/border, không tô chữ `--warn` toàn câu
(để giữ tương phản > 7:1). Kiểm lại `badge--warn` đang đúng.

### F3 — Unsupported là "ngõ cụt kỹ thuật" (Cao, P3/P4)
Màn quick, mẫu `unsupported-syntax`, đoạn hiển thị nguyên văn:
`Cấu hình Sau, dòng 5: expected IPv4 address and wildcard, host, or any
(PARSE_INVALID_IP)`.
Vấn đề: (a) tiếng Anh giữa UI VI; (b) "Cấu hình Sau" viết hoa giữa câu; (c)
không có `role="status"`/`aria-live` cho thông báo ngoài phạm vi; (d) CTA duy
nhất là "Xem chi tiết" (button quiet) — không có hành động chính "Xem / sửa cấu
hình" để sửa dòng lỗi.
Đề xuất: map mã lỗi parser đã biết sang i18n (`parse.invalidIp`, `parse.*`) và
giữ mã gốc trong `<code>` cạnh đó; thêm CTA primary `Xem / sửa cấu hình` cuộn tới
`#after-acl` và focus textarea; bọc notice trong `role="status"`.
**Tái dùng có sẵn:** `web/src/lib/errors.ts` đã có `errorCopy()` map mọi mã
`PARSE_*`/`INPUT_INVALID` sang `error.parse.title/body/hint` và
`errorLocation()` cho `{side, line}`; i18n VI/EN đã có `error.parse.at`
("{side}, dòng {line}"). Không cần bảng mã mới hay đổi contract — chỉ dùng lại
các key này cho notice ngoài phạm vi, đặt raw message trong disclosure kỹ thuật.
Bằng chứng: `verify-states.json` → `unsupported`; ảnh `p2-unsupported-1440.png`.

### F4 — Touch target dưới 44px (Vừa, P3)
Đo hiện tại (quick 320/390 + deep):
- `.technical-details > summary`: **115×21**, 184×21, 182×21, 188×21.
- `.button--small` / `.mode-switch button`: **40px** cao (nhiều nút).
- `.skip-link`: **143×38**.
- checkbox trần 16×16 (trong label — cần đo label, chưa kết luận).
Đề xuất: token `--tap: 44px`; đặt `min-height: var(--tap)` cho `.button`,
`.button--small`, `.mode-switch button`, `.technical-details > summary`,
`.review-disclosure > summary`, `.skip-link` (padding-block 12px). Với summary
trong văn bản, dùng pseudo-element mở rộng vùng chạm thay vì tăng chiều cao thị
giác nếu muốn giữ nhịp.
Bằng chứng: `coordination/reviews/touch-2026-11-02.json`.

### F5 — Watch drawer đứng trước H1 và lặp tiêu đề (Vừa, P4)
Khi mở drawer ở 1440: H2 "Theo dõi & thông báo" (y=200) → H3 "Đối tượng đang
theo dõi" (y=435) → **H1 "Duyệt thay đổi cấu hình" (y=670)**. DOM đặt
`.watch-drawer` trước nội dung chính trong `<main>`; đồng thời summary và H2
trong panel cùng chữ "Theo dõi & thông báo".
Đề xuất: chuyển `.watch-drawer` xuống **sau** header của quick/deep (hoặc vào
`<aside>` sau H1); bỏ H2 trùng, dùng `aria-labelledby` trỏ tới summary hoặc đổi
thành H3; panel chỉ giữ một tiêu đề nhìn thấy.
Bằng chứng: `p2-drawer-1440.png`, `drawer-open-1440.png`, log `p2-shots.mjs`.

### F6 — Mobile ẩn sạch tín hiệu tin cậy (Vừa, P3)
`@media (max-width:720px) .runtime-indicator { display:none }` và
`@media (max-width:560px) .ai-indicator { display:none }`. Ở 390/320 người dùng
chỉ còn logo + VI/EN, không biết máy chủ cục bộ còn sống hay AI đang tắt — đây là
hai lời hứa tin cậy của sản phẩm.
Đề xuất: giữ một dòng trạng thái gọn dưới mode bar ở ≤560px:
`Máy chủ cục bộ · AI tắt` (12px, `--muted`, có chấm màu), hoặc một dot + popover
accessible; không ẩn hoàn toàn. Nếu sợ chật, đưa vào `.mode-bar` bên trái thay vì
header.
Bằng chứng: `touch-390-quick.png`, `p2-quick-320.png`; CSS `app.css:1108,1125`.

### F7 — Measure deep quá dài (Vừa, P4)
`.witness-detail` rộng **804px** ở 1440 (14px/22.4 → ~100ch); `.verdict-plate`
đã đúng (543px, `max-width:543.375px`). Quick đã đúng sau P2 (517/453px).
Đề xuất: `max-width: 68ch` cho các đoạn prose/ghi chú trong `.witness-detail`
(không áp cho bảng, `pre`, route), hoặc `--measure: 68ch` trong tokens và dùng
nhất quán. Giữ nguyên phần code/rule full-width.
Bằng chứng: `coordination/reviews/contrast-2026-11-02.json` → `deepMeasure`.

### F8 — Wildcard port hiển thị `:0` (Vừa, P4)
Route quick: `192.0.2.10:0 → 198.51.100.10:22`. Với TCP, cổng nguồn 0 là
wildcard/không xác định, nhưng `:0` đọc như lỗi dữ liệu.
Đề xuất: khi `src_port === 0` hiển thị "mọi cổng" (hoặc bỏ hẳn phần `:port`),
tương tự `packetPort` đang xử lý non-TCP. Chỉ đổi presentational format, không
đổi packet.
Bằng chứng: `p2-quick-1440.png`, `p2-quick-320.png`; `lib/format` `packetPort`.

### F9 — Rail mobile giấu bước đầu, không chỉ dấu cuộn (Vừa, P3/P4)
Ở 390 deep: rail `scrollWidth 769 / clientWidth 390`, tự cuộn `scrollLeft 210`
để đưa bước 03 vào giữa; bước 01 nằm ở `x=-194` (mất hẳn), bước 02 `x=-46`.
Chỉ có mask gradient 14px, không có nút/đếm "2 bước trước".
Đề xuất: thêm affordance rõ — chip `‹ 01–02` ở mép trái hoặc hiện số bước
(`2/5`), hoặc dùng progress dots 5 bước cố định dưới mode bar. Giữ auto-scroll
nhưng đừng để mất dấu vị trí.
Bằng chứng: `night-20261102/audit/rail-390.png`, log `rail390.mjs`; CSS
`app.css:1069–1102`.

### F10 — Form inbox thưa, chưa thành khối (Vừa, P4)
Mở drawer: hai field + select + 3 checkbox + nút "Theo dõi" dàn một hàng ngang
ở 1440, không có legend/nhóm; empty state "Chưa có đối tượng nào được theo dõi"
đứng riêng phía trên form. Tổng thể giống form debug hơn form sản phẩm.
Đề xuất: grid 2 cột cho name/type, fieldset có `legend` "Điều kiện kích hoạt"
cho 3 checkbox, nút primary căn phải; empty state đặt trong khung list có
min-height để form không "lơ lửng". Chỉ CSS/markup presentational.
Bằng chứng: `drawer-open-1440.png`, `probe-drawer2.mjs`.

### F11 — Chọn mẫu xong vùng kết quả trống (Thấp, P4)
Đo 4s sau khi đổi `#sample-select` sang `budget-exhausted`: không có
`.quick-outcome`, không có thông báo; chỉ có nút "Xem hành vi thay đổi" nằm dưới
hai editor. Người dùng phải tự hiểu là phải bấm chạy.
Đề xuất: giữ mô hình chạy tường minh (an toàn), nhưng đặt một placeholder trong
vùng kết quả: "Đã nạp mẫu — bấm Xem hành vi thay đổi để so sánh" kèm nút primary
tại chỗ; hoặc auto-run **chỉ** khi đổi sample (không auto-run khi gõ tay).
Bằng chứng: `verify-states.json` → `budget` (`hasOutcome:false` sau 4s).

### F12 — `.mode-bar` ngoài landmark (Vừa, P3)
Axe best-practice: `region` — "All page content should be contained by
landmarks", node `.mode-bar`, impact moderate. Topbar là `<header>`, nội dung là
`<main>`, nhưng mode bar là `<div>` trần giữa hai landmark.
Đề xuất: bọc `.mode-bar` bằng `<nav aria-label="Chế độ duyệt">` (switch bên trong
đã là `role="group"`), hoặc đưa vào đầu `<main>`. Không đổi hành vi.
Bằng chứng: `coordination/reviews/axe-best-practice-2026-11-02.json`.

### F13 — Skip link không đưa focus vào main (Vừa, P2.1/P3)
Tab → Enter trên "Bỏ qua điều hướng": `document.activeElement` = **BODY**;
`main#main` không có `tabindex`. Người dùng bàn phím mất vị trí, phải Tab lại từ
đầu vùng nội dung.
Đề xuất: `<main id="main" tabIndex={-1}>` trong `App.tsx`; thêm
`main:focus { outline: none }` (hoặc `:focus-visible` ring nếu muốn). Kiểm lại
bằng `probe-focus.mjs`.
Bằng chứng: `probe-focus.mjs` (P1, App.tsx không đổi phần này ở P2).

### F14 — Record quick 519px lạc trong khung 928px (Cao, P2.1/P3)
Ở 1440: header/watch/mode bar rộng **928px** (x≈272→1200), H1 và các disclosure
căn trái tại x≈277; nhưng `.record-refined` là flex item với `width:auto` +
`margin:auto` nên co **shrink-to-fit còn 519px** (token `--study-record-width`
là 928px, không phải 519), card nằm giữa khoảng x≈495→1047, chừa gutter
trống ~223px bên trái và lệch hẳn trục với H1. Trông như card study được dán vào
trang sản phẩm.
Đề xuất (chọn 1, ZCode quyết):
1. Nhanh: đặt toàn cục `.quick-outcome { width:100%; max-width:720px; margin-inline:0 auto; }`
   để card thẳng trục H1, vẫn giữ measure ~75ch; hoặc
2. Đúng bài: cho quick một grid 2 cột `minmax(0,1fr) 240px` — card 640–680px +
   rail phụ (nguồn, hash, phạm vi, hành động), dùng hết 928px; hoặc
3. Nếu muốn giữ card 519px, căn giữa **toàn bộ** quick screen (header, watch,
   mode bar) trong cùng một cột 519–560px để trục thống nhất.
Không đổi component, chỉ CSS/layout. Đây là điểm "feel" lớn nhất còn lại ở màn vàng.
**Nguyên nhân đo được (quan trọng khi sửa):** `.step.quick-screen` là `display:flex`
(`components.css:1861`), `.record-refined` là flex item với `width:auto` +
`margin:auto` nên bị **shrink-to-fit = 519.125px**; `max-width` hiện là 928px chứ
không phải 519px. Vì vậy chỉ thêm `max-width:720px; margin-inline:0 auto` **sẽ
không đủ** — phần tử vẫn co còn ~519px và dính lề trái của phần co. Cần thêm
`width:100%` (hoặc `flex: 0 1 720px`) cho `.quick-outcome`, rồi mới
`max-width:720px; margin-inline:0 auto`. Đo thêm: ở 720 record 381px trong header
688px (lệch 153px); ở 721 record 519 trong header 673 (lệch 77); ở 960 lệch 196.
Chỉ ≤~455px record mới chạm mép. Vì thế áp dụng **toàn cục** (mobile hẹp không
đổi), không chỉ ≥960. Đo lại `record.width ≈ 720` và `record.x ≈ h1.x` ở 1440,
1024, 720 **và** 390 (390 vẫn đầy bề rộng).
Bằng chứng chuỗi flex: `night-20261102/audit/probe-f14-chain.mjs`.
Bằng chứng: `p2-quick-1440.png`, `contrast-2026-11-02.json` (`outcome.width 519`,
`header.width 928`); CSS `change-record.css:2`.

### F15 — Deep lặp H2 "Gói tin minh chứng" (Thấp, P4)
Deep 1440: H2 "Gói tin minh chứng" tại y=1211 (section) và lặp y=1326 (title
trong `WitnessDetail`). Hai heading cùng cấp, cùng chữ, không phân biệt cấp bậc.
Đề xuất: trong `WitnessDetail` đổi title thành H3 (hoặc bỏ heading, dùng
`aria-labelledby` region); giữ H2 section. Kiểm lại axe heading-order sau đó.
Bằng chứng: `coordination/reviews/deep-1440.json`; ảnh `deep-1440.png`.

### F16 — Error state thiếu đường phục hồi (Vừa, P4)
Chặn `/api/**`: `.error-panel` (`role="alert"`) hiện đúng, chữ Việt, có nút
"Đóng" — nhưng **không có "Thử lại"**; CTA duy nhất còn lại là "Xem hành vi thay
đổi" nằm dưới editor, không được trình bày như hành động phục hồi. Copy
"Khởi động lại plenilune-demo rồi thử lại" là hướng dẫn cho dev (tên package),
không phải câu cho người dùng. Panel rộng full main (x=48→1384) trong khi cột
quick là 928px (x=272→1200) → lệch trục.
Đề xuất: thêm nút primary "Thử lại" trong panel (gọi lại run hiện tại), đổi copy
thành "Khởi động lại máy chủ Plenilune rồi thử lại."; đặt panel trong cùng cột
928px hoặc thành banner toàn cục có nhãn rõ. Giữ raw message trong "Chi tiết kỹ
thuật" (đã đúng).
Bằng chứng: `night-20261102/audit/error-loading.json`, `state-error-1440.png`.
Trạng thái loading đạt: `.busy-panel role="status"` + spinner + "Hủy", không
nhảy layout lớn (`state-loading-1440.png`).

### F17 — Hàng đối tượng overview khó chọn (Thấp, P4)
Mẫu nhiều đối tượng (`full-regression`, `full-routing`, `full-refactor`): quick
hiện panel "Tổng quan thay đổi cấu hình" với 2 hàng, mỗi hàng có nút chữ
"Xem chi tiết" ở cuối — **tên đối tượng/trạng thái không click được**, cả hàng
không phải vùng chọn; nhãn nhìn thấy của hai nút giống nhau.
**Đính chính 00:02:** accessible name **không trùng** — P2 đã đặt
`aria-label="Xem chi tiết: ACL MGMT"` và `"…: Prefix-list CUSTOMER"` (đo lại
trong `p4-verify`, `names` khác nhau). Vấn đề còn lại chỉ là nhãn thị giác và
vùng chọn, mức **Thấp**.
Đề xuất: biến cả hàng thành `<button>` (hoặc link phủ) với `aria-pressed`/trạng
thái chọn, hover + focus ring; đổi nhãn nhìn thấy thành "Xem ACL MGMT" hoặc giữ
aria-label hiện có; nêu rõ hàng "Đổi hành vi" được ưu tiên.
Bằng chứng: `samples.json`, `sample-full-regression.png`, `p4-verify-2026-11-03.json`
(`F17.names`).

### F18 — Nhãn verdict/coverage còn ngôn ngữ kỹ thuật (Thấp, P4)
Badge chính hiển thị nguyên enum `DIFFERENT` / `EQUIVALENT` (tiếng Anh, chữ
hoa) trong UI VI; headline bên dưới đã Việt hóa. Ngoài ra trạng thái EQUIVALENT
hiện đồng thời banner "Có phần cấu hình chưa được phân tích đầy đủ." và dòng
"Đã tìm hết phạm vi: **Có**" — đúng về mặt mô hình (search complete trong phạm
vi) nhưng đọc lên như mâu thuẫn.
Đề xuất: badge dùng nhãn Việt ("Khác biệt"/"Tương đương") và giữ mã enum trong
`title` hoặc "Chi tiết kỹ thuật"; đổi dòng thành "Đã tìm hết trong phạm vi mô
hình: Có" (hoặc "Tìm kiếm trong mô hình: hoàn tất"). Chỉ i18n/copy, không đổi
verdict.
Bằng chứng: `samples2.json` (`full-refactor`, `full-routing`); ảnh
`sample2-full-refactor.png`.

## Đã đạt (không cần sửa)

- **Tương phản**: 19 mẫu current, min 5.08 (`DIFFERENT` badge, 12px/500),
  mode inactive 5.88, provenance 5.88; chữ chính 12–13.35. Đạt AA cả text nhỏ.
- **Overflow ngang**: 0 ở 1440/390/320, quick+deep; `scrollWidth = clientWidth`.
- **Focus ring**: 2px `--accent` + offset 2px, nhất quán trên link/button/input/
  summary; phần tử trong details đóng không vào tab order.
- **Axe WCAG A/AA**: 0 vi phạm (quick/deep/watch-open, 1440/390).
- **Quick measure sau P2**: 517px details / 453px body (≈56–62ch) — đã sửa.
- **Route mobile sau P2**: mũi tên xoay ↓, hai đầu xếp dọc, không còn cảnh
  `:0` đứng một dòng; chỉ còn ngữ nghĩa `:0` (F8).
- **Header/P5 trong dist**: mark M2 đúng path, 20×20, fill `rgb(48,46,40)`;
  favicon 32 viewBox kem/mực — khớp mô tả mục C (chờ gate chính thức).
- **EN locale**: quick + deep ở 1440/390 không tràn ngang, không cắt nhãn; câu
  "Some access is newly blocked" / "View / edit configurations" đọc ổn.
- **200% text**: tăng root font 16→32px ở quick + deep 1440, `scrollWidth` vẫn
  1440, không phần tử nào tràn; card 519px giữ measure ngắn, không cắt chữ.
- **Reflow 720** (tương đương 200% zoom vật lý 1440): quick + deep không tràn,
  rail/mode bar chuyển đúng breakpoint.
- **Reduced motion**: đã có `@media (prefers-reduced-motion: reduce)` ở
  `tokens.css:93` và `app.css:1135`; không phát hiện animation bắt buộc.

## Cách chạy lại

Từ `D:\Quorix\plenilune\night-20261102\audit` (server :8084 phục vụ `dist`):
```powershell
node touch.mjs          # touch target + overflow 320/390, quick/deep
node contrast-save.mjs  # contrast + measure, ghi coordination/reviews/
node axe.mjs            # WCAG A/AA, ghi axe-2026-11-02.json
node axe-bp.mjs         # best-practice (region), ghi axe-best-practice-...
node reprobe-p2.mjs     # header P5, budget partial, multi-class
node p2-shots.mjs       # ảnh quick 1440/320 + drawer order
node zoom-locale.mjs    # EN 1440/390, 200% text, reflow 720
node verify-p3.mjs      # gate F1/F2/F4/F6/F12/F13/F14 -> reviews/p3-verify-2026-11-02.json
node verify-p4.mjs      # F3/F5/F7-F11/F15-F18 -> reviews/p4-verify-2026-11-03.json
node breakpoints.mjs    # 1280/1024/961/960/721, đo F14 + overflow
```
Lưu ý: `metrics-2026-11-02.json`, `states-2026-11-02.json`, `deep-1440.json`
được đo trên dist P1 (trước 23:38); các file còn lại đo trên dist P2+P5.
Không dùng số liệu P1 cho quick nếu đã có bản P2.

## Ghi chú phối hợp

- F1, F14, F13 nên vào **P2.1** (Codex vừa RELEASE P2 lúc 23:38; ZCode đang
  gate). F2/F4/F6/F12 là P3 (token/motion/touch). F3/F5/F7/F8/F9/F10/F11/F15 là P4.
- Không có thay đổi hợp đồng/dữ liệu nào được đề xuất; mọi mục chỉ là CSS,
  markup presentational, i18n hiển thị hoặc thứ tự DOM.
- P5 vẫn thuộc lock C; bản dist 23:38 đã chứa mark nhưng **chưa có gate ZCode**,
  tôi chưa ghi RELEASE.

