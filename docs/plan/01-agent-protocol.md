# Plenilune — Protocol cho agent thực thi kế hoạch

Áp dụng cho mọi AI agent (và người) thực hiện các hạng mục trong `docs/plan/`.
Ngày: 31/10/2026. Protocol này không thay `ENGINEERING_GUARDRAILS.md`; khi xung
đột, guardrails thắng và agent phải báo lại để sửa kế hoạch.

## 1. Đọc trước khi làm (theo thứ tự)

1. `AGENTS.md` và `ENGINEERING_GUARDRAILS.md` (invariant + cổng kiểm chứng).
2. `docs/plan/00-master-plan.md` (đích, thứ tự phase, do-not-do, ngân sách AI).
3. `docs/plan/STATUS.md` — hạng mục đầu tiên chưa xong là việc của bạn.
4. File phase chứa hạng mục đó; rồi tới tài liệu owner liên quan (`CONTRACTS.md`,
   `ARCHITECTURE.md`, `VERIFICATION_DESIGN.md`, `AI_AGENT_DESIGN.md`,
   `SECURITY.md` tùy hạng mục).

## 2. Vòng làm việc cho một hạng mục

1. **Đối chiếu baseline.** Các finding trong kế hoạch được kiểm chứng trên bản
   public `main` ngày 31/10/2026. Repo bạn đang đứng có thể khác (nhánh local,
   việc đã làm dở). Trước khi sửa, kiểm tra finding còn đúng trên code hiện tại
   không; nếu đã được sửa hoặc không còn đúng, ghi vào STATUS là
   `already-done`/`invalid` kèm bằng chứng (file:dòng), rồi chuyển hạng mục kế.
   Không sửa lại thứ đã đúng.
2. **Một hạng mục, một lượt thay đổi.** Không gộp hai hạng mục vào một commit/
   một lượt sửa. Không sửa tiện thể ngoài phạm vi (thấy lỗi khác: ghi vào mục
   "Phát hiện ngoài phạm vi" cuối STATUS, không tự xử lý).
3. **TDD ở mức dự án đang làm:** viết hoặc xác định test/oracle trước khi đổi
   semantics; giữ nguyên mọi assertion hiện có — cấm nới assertion, tăng
   timeout, hay đánh dấu flaky để test xanh (guardrails).
4. **Chạy cổng kiểm chứng đầy đủ của guardrails** sau mỗi hạng mục:

   ```text
   gofmt -l cmd internal
   go vet ./...
   go test ./...
   go test -race ./...
   go build ./...
   cd web && npm test && npm run build   (khi hạng mục đụng frontend)
   ```

   Cộng tiêu chí nghiệm thu riêng của hạng mục (trong file phase). Live test
   với provider chỉ chạy khi hạng mục yêu cầu và Chooky cho phép riêng; không
   được coi mock/fake là bằng chứng live.
5. **Cập nhật tài liệu owner trong cùng lượt** nếu contract/schema/hành vi đổi
   (CONTRACTS/ARCHITECTURE/README/SECURITY tùy loại thay đổi), và cập nhật
   STATUS: trạng thái, commit/nhánh, lệnh đã chạy + kết quả, giới hạn còn lại.
6. **Báo cáo kết quả theo mẫu mục 5** rồi dừng hoặc xin giao hạng mục tiếp.
   Không tự nối tiếp sang hạng mục `DECISION` hoặc phase kế tiếp khi phase
   hiện tại chưa đạt điều kiện ra.

## 3. Điều kiện dừng — phải dừng và hỏi Chooky

- Hạng mục (hoặc một lựa chọn trong hạng mục) gắn nhãn `DECISION`.
- Finding không còn đúng trên code hiện tại và hướng sửa không hiển nhiên.
- Cổng benchmark/nghiệm thu đã đăng ký trước **không đạt** (ví dụ LCP không giảm
  ops/time như kỳ vọng): dừng, báo số đo thật, không tự chuyển sang phương án
  khác (atomic predicates, tăng cap…) khi chưa được duyệt.
- Cần đổi license, push/publish, gọi live provider, dùng dữ liệu cấu hình thật,
  hay bất kỳ thao tác nào lên thiết bị.
- Cần kết luận pháp lý (GPL/tích hợp/phân phối): ghi trạng thái chặn, không tự
  kết luận.

## 4. Kỷ luật bằng chứng và diễn đạt

- Phân biệt rõ trong mọi báo cáo: **implemented** (đã có code + test), **đã đo**
  (số liệu kèm môi trường/corpus/median), **thiết kế** (chưa code), **chưa kiểm
  chứng**. Không biến thiết kế thành claim production.
- Mọi số đo hiệu năng phải ghi môi trường (OS/CPU/Go version), corpus, số lần
  chạy và median — theo đúng cách `docs/research/` đang làm. Không suy rộng từ
  cache nóng hay một lần chạy.
- Hash là kiểm tra nội dung, không phải chữ ký/xác thực tác giả — giữ đúng cách
  diễn đạt của dự án ở mọi tài liệu mới.
- Không log/in vào báo cáo: secret, raw config, API key. Dữ liệu test chỉ dùng
  synthetic (dải địa chỉ tài liệu), như quy định hiện hành.

## 5. Mẫu báo cáo sau mỗi hạng mục

```text
Hạng mục: <ID + tên> — <done / blocked / already-done>
Thay đổi: <files, tóm tắt 2–4 dòng>
Kiểm chứng đã chạy: <lệnh + kết quả, gồm cả nghiệm thu riêng của hạng mục>
Số đo (nếu có): <môi trường, corpus, trước/sau>
Giới hạn còn lại: <điều chưa chứng minh được, việc chưa làm>
Quyết định cần Chooky (nếu có): <câu hỏi cụ thể + lựa chọn>
```

## 6. Ngân sách rủi ro AI khi thực thi

Áp dụng bảng xanh/vàng/đỏ trong `00-master-plan.md`. Cụ thể khi dùng AI hỗ trợ
viết code cho chính dự án này: AI được soạn code/test/tài liệu nháp tự do
(xanh), nhưng mọi thay đổi chỉ được coi là xong khi cổng kiểm chứng mục 2.4
xanh và — với thay đổi đụng parser/algebra/evidence/repair — có review độc lập
(theo guardrails: review độc lập ưu tiên chính các vùng này). AI không tự tuyên
bố một hạng mục "đạt Production".
