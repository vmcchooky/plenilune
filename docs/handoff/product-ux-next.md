# Product UX tiếp theo — handoff thiết kế

> Snapshot thiết kế trước triển khai; D0–D4 đã được thực hiện sau đó. Trạng thái và kiểm tra hiện hành: [README](../../README.md) / [handoff](contributor-runtime.md).
Ngày 30/10/2026. **Chỉ thay tài liệu; chưa sửa runtime.** Thiết kế/acceptance tại
[product-ux](../design/product-ux.md); wire owner [CONTRACTS](../../CONTRACTS.md),
package owner [ARCHITECTURE](../../ARCHITECTURE.md), thứ tự [ROADMAP](../../ROADMAP.md).
Handoff này là bản bàn giao, không authority semantics thứ hai.

## Implemented — đã đối chiếu code

- UI gọi Go parse/normalize/compare/session/repair/replay; `samples.ts` chỉ input
  synthetic. Không đúng nếu mô tả UI hiện tại chỉ có fixture verdict.
- Exact comparison, obligations riêng, typed patch, full-region collateral,
  receipt/replay đã có. Policy nil scope khác explicit []; collateral dùng
  original-before → candidate. Core không cần provider/MoonStorm.
- VI/EN dictionary và raw-response export, source/policy generation guard đã có;
  chưa bảo vệ đủ manual edit revision. AI completed không phải human approval.

## Proposed — nhận việc theo thứ tự

1. **D0:** khóa verify/replay vào edit revision/selected receipt; deferred-response
   regression. `useReview.addEdit/removeEdit` hiện clear receipt nhưng không tăng
   generation/abort, remove edit còn dùng được khi verify busy. Finding từ code,
   chưa tái hiện browser trong lượt thiết kế; không tự coi tests hiện tại đã phủ.
2. **D1–D2:** delta trước policy (`conditions:null`); canonical full-policy preview
   rồi acceptance. Backend AI có thể làm wrapper `/api/policy/normalize` theo
   CONTRACTS §7; frontend owner tích hợp hook. Không đổi proof/patch v1.
3. **D3:** phiếu nêu hành vi SSH cụ thể, ba track kết luận, nhận/giữ lại/từ chối;
   mismatch tạo disputed và chặn nhận. AI khác làm presentation/VI–EN templates.
4. **D4:** export đúng bytes, standalone receipt, hướng dẫn/import fresh review;
   test CRLF/BOM/no-final-newline và parser rejection, không silently đổi nguồn.
5. **D5:** offline full path và browser QA mới; qualification live riêng sau này.

Một owner tích hợp `web/src/hooks/useReview.ts`; chia adapter/templates/artifact
UX cho AI khác theo paths trong thiết kế, reviewer độc lập kiểm identity/evidence.
Packet editor ba phiên bản và multi-candidate để sau hackathon. Không cần DB,
service, queue, framework mới. Model dự kiến chỉ `muse-spark-1.3`.

## Chưa kiểm chứng / không được claim

- Live Muse + real engine/repair/UI end-to-end, latency/cost/model quality. Mock
  HTTP test có engine thật; live agent smoke hiện dùng fakeEngine. Nhập key chưa
  chứng minh tool loop hay completed-model UI. Không live call trong nhiệm vụ này.
- UI và endpoint **mới** trong thiết kế chưa implemented/QA. Historical browser
  QA không chứng minh canonical preview, candidate guard hoặc import mới.
- Hash/acceptance bool không xác thực tác giả/người phê duyệt; current tool commit
  là unversioned. Không chứng nhận intent fidelity hay thiết bị/network thực tế.

## Kiểm tra trong lượt thiết kế

Đã chạy, đều PASS: `gofmt -l cmd internal` (không file lệch), `go vet ./...`,
`go build ./...`, `go test -count=1 -skip '^TestLiveMeta' ./...`,
`go test -race -count=1 -skip '^TestLiveMeta' ./...`, `npm test` (5 file/34 tests).
Skip riêng hai live smoke để bảo đảm không gọi provider hoặc đọc key, bất kể
opt-in môi trường; không thay môi trường server. Không chạy browser QA hay build
lại frontend (tránh ghi đè assets của server đang chạy).

Hai reviewer độc lập đã kiểm backend semantics/evidence và frontend journey/state.
Đã làm rõ full-policy compare qua acceptance flip (không so hai P trực tiếp),
replay mismatch chặn nhận, và byte-preserving import. Các checks trên xác nhận
baseline không phải chứng cứ các tính năng proposed đã hoạt động.
