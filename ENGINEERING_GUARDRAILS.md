# Plenilune — quy tắc làm việc

Áp dụng code mới tại root; `Sơ khai/` là lịch sử chỉ đọc. Ngày: 30/10/2026.

## Làm nhanh, giữ đúng contract

- Viết luồng nhỏ chạy được trước; mở rộng subset sau, không thêm framework/service khi chưa cần.
- Go stdlib core, không cgo. Interface khi có caller/test seam thật; không package `utils/common`.
- Parser/evaluator/algebra/verifier thuần, không đọc file, gọi mạng hay global mutable state.
- CLI/adapter sở hữu I/O và context; không goroutine nền nếu không có lifecycle rõ.
- Không copy code MoonStorm, không thay file gốc `Sơ khai/`, không tự đổi license/publish.
- Contract/version đổi cùng docs, fixture và caller; một owner tích hợp JSON DTO.

## Những invariant cần giữ

- First-match + implicit deny; unsupported token không được bỏ qua để lấy kết quả đẹp.
- Point evaluation không được ghi thành equivalence; complete search mới được EQUIVALENT trong scope.
- AI không tự tạo verdict/witness; obligation hợp lệ không chứng minh đã hiểu đủ intent.
- Hash bind obligation definitions, scope, input digests và deterministic results.
- Obligation violation có witness riêng kể cả hai ACL không khác nhau.
- Repair là all-or-nothing trong bộ nhớ; kiểm base digest/refs và collateral theo original-before.
- Budget/cancel/unsupported không được nâng thành an toàn; giữ evidence có giới hạn rõ.

## Input, output, network

- Bound bytes/lines/rules trước cấp phát lớn; UTF-8, reject NUL/control không hợp lệ.
- Error typed code/stage/source line; không log secrets hoặc cả config.
- API key từ environment; AI off không đọc key/gọi mạng; live tests opt-in synthetic.
- Không thực thi nội dung config/AI; model tool không nhận shell/path tùy ý.
- HTML dùng escaping mặc định; render từ evidence, không tính lại semantics trong view.
- Không SSH/apply thiết bị. Patch proposal chỉ là dữ liệu để review và verify.

## Verification vừa đủ cho thay đổi

```text
gofmt -l cmd internal
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Parser: golden valid/invalid/unsupported và fuzz. Algebra: reduced-domain exhaustive oracle + boundary tests.
Verifier: witness recheck, reorder, implicit deny, independent obligation violation, budget cancellation.
Evidence: tampered definitions/scope/input/results bị phát hiện khi kiểm digest và replay; không claim authentication.
AI: fake provider cho failures; một live synthetic path đo thật. Fake không thay bằng chứng Meta.
Không tăng timeout, skip assertion hay gán “flaky” chỉ để đóng báo cáo.

Review độc lập ưu tiên parser/algebra, evidence contract và repair correctness.
Không cần thêm vòng phê duyệt cho refactor/helper nhỏ trong phạm vi đã giao.
Báo kết quả đã chạy và giới hạn thực tế; không biến thiết kế thành implemented/production claim.
