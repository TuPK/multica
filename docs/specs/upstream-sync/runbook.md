# Runbook: sync upstream vào fork

Đọc file này trước khi sync hoặc merge `upstream` (`multica-ai/multica`) vào fork `TuPK/multica`. Lý do và số liệu nằm ở [spec.md](spec.md); đây là các bước để làm đúng. Chỉ sync khi người dùng yêu cầu. Trả lời người dùng bằng tiếng Việt.

## Quy tắc đã chốt (không hỏi lại)

- Giữ billing và platform-management của fork; entitlement là hợp của hai bên; loại `server/ee` và `packages/ee` (license); dùng mô hình project private/guest của upstream.
- Chữ hiển thị là **Delegora**; định danh, biến môi trường, URL, binary giữ `multica` (`docs/fork-branding.md`).
- Quy trình: tạo nhánh sync, đẩy nhánh lên `origin`, fast-forward `main`, đẩy `main`. Không mở PR vào `multica-ai/multica`. Không dùng kiro/agy trừ khi được yêu cầu.
- Test DB-backed của Go chỉ chạy với `DATABASE_URL` trỏ `multica_test`. **URL mặc định của test là DB `multica`: không bao giờ để trống biến này.**

## Các bước

1. **Một lần cho mỗi checkout:** `bash scripts/sync/install.sh`, rồi `pnpm sync:doctor` phải xanh. Thiếu bước này git quay về merge thường (nhiều conflict giả, không mất dữ liệu).
2. **Xem trước quy mô (tùy chọn):** `git fetch upstream`, rồi `pnpm sync:simulate --fork HEAD --upstream upstream/main`. Số file conflict theo loại, không đụng checkout.
3. **Bắt đầu:** từ `main` sạch, `pnpm sync:upstream`. Nó tạo nhánh `chore/sync-upstream-<ngày>`, merge, gỡ `server/ee` và `packages/ee`, rebrand dòng upstream thêm vào file chỉ upstream sửa, cảnh báo nếu fork và upstream cùng thêm một đường dẫn (chuyển bản fork sang `*_fork.go` trước khi giải), rồi in danh sách conflict còn lại. Nếu nhánh sync của ngày đã có, nó thêm hậu tố `-2`. File ghi `[recurring: seam ...]` là file hay conflict lặp lại.
4. **Giải conflict thật bằng tay.** Nguyên tắc: lấy logic, định danh, giao thức của upstream; giữ chữ Delegora và tính năng fork. Không xóa mã upstream nếu gỡ nối được. Mã fork mới đặt trong file fork (`*_fork.go`, `*.fork.ts`, `router_fork.go`, `fork_index_cleanups.go`), file upstream chỉ thêm một dòng gọi. Nếu `client.ts` mất dòng `class ApiClient extends ForkApiMethods`, hoặc `router.go` mất `registerForkWorkspaceRoutes`, khôi phục ngay (`sync:contract` sẽ báo).
5. **Sau mỗi conflict giải xong:** `git add` đúng file đó. Đừng `git add -A` khi còn marker.
6. **Kết thúc:** `pnpm sync:upstream --finish`. Nó kiểm: không còn marker, tái sinh file sinh tự động (`pnpm install`, reserved slugs, `catalog:build`, `make sqlc`), `brand:check`, khóa `vi` thiếu, `scripts/sync-audit.mjs`, `sync:contract`, và các test hay vỡ im lặng sau khi di chuyển file (golden route table, kiểm kê tiền tệ, fixture entitlement; khai báo ở `finishChecks` trong `policy.json`). Dừng ở lỗi đầu tiên và nói rõ.
7. **Khóa `vi` thiếu:** `pnpm sync:vi --list --out <file>`, thêm trường `"vi"` (bản dịch tiếng Việt) cho từng mục, `pnpm sync:vi --apply <file>`. Số nhiều: `vi` chỉ cần `_other`.
8. **Đọc `sync-audit`:** mỗi dòng nó liệt kê là ứng viên mất mã, không phải bằng chứng; kiểm từng cái (thường là dòng đã viết lại hoặc chuyển chỗ).
9. **Kiểm thử đầy đủ**, chạy nối tiếp khi máy yếu: `pnpm typecheck`, `pnpm lint`, `pnpm exec turbo run test --concurrency=1`, `pnpm test:scripts`, `cd server && go vet ./... && go test -p 1 ./...` (với `DATABASE_URL` trỏ `multica_test`; xuất `export DATABASE_URL=...` trước). Test đỏ từ trước, không do sync: `platform-management/console`, `TestAgentTaskHistoryIndexMigrationAndPagePlans`.
10. **Migration mới của upstream:** cái nào tạo index `CONCURRENTLY` phải có mục trong `concurrentIndexCleanups` (`server/cmd/migrate/main.go`); của fork nằm ở `fork_index_cleanups.go`. Thử cài mới: tạo DB trống, `cd server && DATABASE_URL=<DB trống> go run ./cmd/migrate up`, phải `Done.`, rồi xóa DB đó. Runner sắp xếp migration theo chuỗi (spec 3.12).
11. **Commit, đẩy nhánh sync lên `origin`, fast-forward `main`, đẩy `main`** (cần người dùng đồng ý cho hành động đẩy). Thêm một dòng vào [ledger.md](ledger.md): ngày, merge, số commit upstream, số conflict gốc và còn lại, thời gian, tính năng mất nếu có; chạy `pnpm sync:surface --write` nếu bề mặt đổi có lý do, và `pnpm sync:coverage --write` để cập nhật hợp đồng inline. Cập nhật bộ nhớ `upstream-sync-progress.md`.

## Khi công cụ sai

- Driver làm kết quả lạ: `pnpm sync:simulate --merge <commit cũ> --compare` so với merge thật; lỗi công cụ thì sửa trong `scripts/sync/` kèm test (`pnpm test:scripts`).
- Cần tắt tạm: `bash scripts/sync/install.sh --remove` (git quay về merge thường).
- Tính năng fork mới không được mất: thêm mục vào `scripts/sync/fork-contract.json`. Phần sửa xen trong file upstream được bao phủ tự động bởi `scripts/sync/fork-inline.json` (sinh bằng `pnpm sync:coverage --write` từ cây đã xác nhận, ví dụ `main` sau khi push; không sinh lại để làm một lỗi biến mất). `pnpm sync:coverage` liệt kê file upstream có sửa lớn của fork mà chưa có kiểm tra nào.
- Lỗi biên dịch sau khi merge sạch là do fork đã đổi chữ ký hàm lõi (thêm tham số `scope`, `guard`, `hook`...). Truyền giá trị đúng một cách có chủ đích, không truyền `nil` hay `Hidden{}` cho xong: các tham số đó mang kiểm tra quyền, audit hay quota, và lỗi biên dịch là chốt chặn duy nhất.
- File nào vẫn conflict ở nhiều lần sync: xem `scripts/sync/policy.json` (`hotFiles`) và spec 3.9 để quyết định có tách seam hay không.
