# Giảm conflict khi sync upstream: kế hoạch triển khai

Đặc tả: [spec.md](spec.md). Kế hoạch này chỉ nói **làm gì, theo thứ tự nào, xong khi nào**. Lý do và số liệu nằm ở đặc tả; mục nào trích "mục X" là mục của đặc tả.

Ngôn ngữ: tiếng Việt; tên tệp, lệnh, mã giữ tiếng Anh.

## 0. Quy ước làm việc

- Tôi làm trực tiếp, không giao kiro/agy (chỉ khi người dùng yêu cầu). Làm trên `main`, commit nguyên tử, tiền tố `chore(sync):` cho công cụ và `refactor(fork):` cho seam. Không đẩy lên `origin` khi người dùng chưa đồng ý.
- Công cụ mới viết bằng Node thuần, **không phụ thuộc gói ngoài** (CI chạy chúng không cần `node_modules`, giống `scripts/brand/sync.mjs`). Mọi script có test `*.test.mjs` cạnh nó; chạy bằng `pnpm test:scripts`.
- Mỗi bước xong khi: test của bước xanh, `pnpm test:scripts` xanh, `git diff --check` sạch. Bước đụng mã sản phẩm thêm `pnpm typecheck`, `pnpm lint`, test vùng đó, rồi `make test`/`pnpm test` nối tiếp (máy quá tải khi chạy song song).
- DB-backed Go test chỉ dùng `multica_test`, không đụng DB `multica`.
- Trước mỗi bước đụng file có sẵn, đọc file; không sửa ngoài phạm vi bước.
- Ước lượng công (S ≈ dưới nửa ngày, M ≈ nửa đến một ngày, L ≈ một đến hai ngày) là **ước tính của tôi**, chưa kiểm.

## Trạng thái (2026-11-24)

- **Xong:** P0, P1, P2 (T2.1 đến T2.4), P3 (T3.1 S1 đủ; T3.2 S2, T3.4 S4, T3.5 S5 một phần hoặc đủ như ghi ở spec 3.9), P4 (T4.1, kết quả ở spec 3.12), P5 (T5.1; T5.2 phụ thuộc có commit mới của upstream).
- **Không làm theo quy tắc dừng:** T3.3 (S3, registry UI) và T3.6 (S6, paths và routes): phần fork trong từng file chỉ là vài chục dòng sửa nhỏ giữa mã upstream, không có khối nguyên để tách; lý do đo được ghi ở spec 3.9. T2.5 (CI định kỳ) bỏ theo quyết định của người dùng.
- **Còn lại:** T5.2, tức lần sync thật đầu tiên bằng quy trình mới. Chỉ làm khi người dùng yêu cầu sync. Sau lần đó điền `ledger.md` và xem file nào trong danh sách "hot" còn lặp lại.
- Số đo bề mặt (`pnpm sync:surface`): 7.582 thêm, 1.732 sửa (758 chỉ-brand, 974 sửa thật), 94 xóa.
- Lệch so với kế hoạch: `.github/ci-paths.json` nhóm `scripts` có thêm `scripts/sync/**` và `.gitattributes`; `shard-timings.json` dùng `ours`; driver JSON phủ thêm mọi `package.json` và hợp mảng chèn; `brand:check` dùng tệp nợ `brand-debt.json` (66 file, 240 dòng) và chưa vào CI; `--finish` kiểm marker conflict; S4 dùng kế thừa thay cho gộp khai báo; S1 gộp bằng biến khởi tạo thay vì `init()`.
- Driver đã được định nghĩa trong cấu hình git của checkout này (`bash scripts/sync/install.sh`).

## 1. Tổng quan thứ tự

```
P0 Công cụ đo      T0.1 simulate ─ T0.2 surface ─ T0.3 sổ + baseline
P1 Driver merge    T1.1 brand-rules ─ T1.2 driver văn bản ─ T1.3 driver JSON ─ T1.4 gắn vào repo
                   T1.5 generated/ours ─ T1.6 nghiệm thu bằng mô phỏng ba merge cũ
P2 Tự động hóa     T2.1 orchestrator ─ T2.2 brand:check + rebrand ─ T2.3 vi-worklist
                   T2.4 contract (T2.5 CI định kỳ: đã bỏ)
P3 Seam            T3.1 (S1) ... T3.6 (S6), làm theo tần suất conflict
P4 Migration       T4.1 điều tra (độc lập, có thể làm song song với P1–P2)
P5 Chốt            T5.1 tài liệu và quy tắc ─ T5.2 sync thật đầu tiên bằng quy trình mới
```

P0 → P1 → P2 theo thứ tự. P3 chỉ bắt đầu sau khi P1 và P2 xong, vì nhiều hot file chỉ lộ ra là thật sự lặp lại sau khi conflict giả (brand, locale, generated) biến mất. P4 độc lập. P5.2 là bài kiểm cuối.

## P0. Công cụ đo

### T0.1 `scripts/sync/simulate.mjs` (M)

Làm gì: tự động hóa phương pháp mục 2.1.

- Đầu vào: `--merge <commit>` (dùng `^1`, `^2` của nó) hoặc `--fork <ref> --upstream <ref>`; `--drivers on|off`; `--compare <merge-commit>`.
- Tạo worktree tạm ngoài repo, `merge --no-commit --no-ff` với `rerere` tắt; nếu `--drivers on` thì truyền cấu hình driver qua `-c`, không ghi vào `.git/config`.
- In danh sách file chưa merge, đếm theo loại (locales, docs/md, generated/lock, ts, go, test, config, other) bằng cùng quy tắc phân loại ở mục 2.2; luôn `merge --abort` và dọn worktree, kể cả khi lỗi.
- `--compare`: với file đã tự merge và là JSON thì so (chuẩn hóa thứ tự key và Delegora→Multica) với merge commit thật; với file văn bản thì so sau chuẩn hóa Delegora→Multica; in tập file khác nhau và tóm tắt.

Test: dựng repo tạm bằng `git init`, ba nhánh nhỏ với một conflict cố định, một file chỉ upstream sửa, một file chỉ fork sửa; kiểm đếm, kiểm worktree được dọn, kiểm không đụng repo gốc.

Xong khi: chạy với `--merge 0ecf290b7 --drivers off` ra đúng **85** và các số 97 (`7b6a1ab8f`), 75 (`ce85c3b61`).

### T0.2 `scripts/sync/surface.mjs` (S)

Làm gì: mục 3.8. Đếm file A/M/D của `main` so với `upstream/main`, theo thư mục cấp hai, và "chỉ khác brand" (cùng quy tắc ở 2.6). Ghi/kiểm `docs/specs/upstream-sync/surface-baseline.json`.

Xong khi: chạy ra gần số ở 2.6 (7.544 thêm, 1.732 sửa gồm 758 chỉ-brand và 974 sửa thật, 94 xóa) và `--check` báo xanh khi chưa đổi gì.

### T0.3 Sổ và baseline (S)

Tạo `docs/specs/upstream-sync/ledger.md` với bốn dòng ở bảng 2.1 và baseline `surface-baseline.json`. Cột: ngày, merge, commit upstream, conflict gốc, conflict còn lại, giờ người, mất tính năng.

Xong khi: người dùng có thể mở sổ và thấy số liệu hôm nay.

## P1. Driver merge

### T1.1 `scripts/sync/brand-rules.mjs` (S)

Làm gì: một module xuất `canon(text)`, `brand(text)`, danh sách glob bề mặt người dùng, danh sách ngoại lệ. Quy tắc `brand()` ở mục 3.1.

Test (bảng ca bắt buộc): `Multica` trong câu → đổi; `multica` thường → giữ; `@multica/core`, `X-Multica-Workspace`, `Multica-Agent`, `multica.ai`, `github.com/multica-ai/multica`, `MULTICA_X`, `Multica Canary`, `© Multica`, `Copyright 2026 Multica` → giữ; chữ Hàn dính liền sau `Multica` (kiểm kết quả mong muốn: chữ Hàn không phải ký tự `\w`, nên vẫn đổi; xác nhận bằng test trên `apps/docs/content/docs/*.ko.mdx`).

Việc kèm theo (quan trọng): chạy `brand(canon(x)) == x` trên mọi file văn bản của HEAD; với 141 file vi phạm đã đo, phân loại **từng lý do** (cố ý giữ Multica hay thiếu ngoại lệ). Thiếu ngoại lệ thì thêm vào module. Ghi kết quả vào spec (thêm mục 2.7).

Xong khi: test xanh và bảng phân loại 141 file có mặt trong ghi chú.

### T1.2 `scripts/sync/merge-brand.mjs` (M)

Làm gì: driver văn bản mục 3.1: `node merge-brand.mjs %O %A %B %L %P`.

Chi tiết: không dùng thư viện; gọi `git merge-file` trên ba bản chuẩn hóa ghi vào tệp tạm cạnh tệp gốc; giữ dòng gốc của `A` bằng hàng đợi theo văn bản chuẩn hóa; tệp có byte NUL thì thoát 1; thoát 1 khi `git merge-file` báo conflict (marker ở lại trong `%A`); giữ kiểu xuống dòng của `A`.

Test tích hợp (repo tạm, merge thật): (1) fork giữ `Multica` có chủ đích, upstream sửa dòng khác → kết quả giữ nguyên dòng fork; (2) upstream thêm dòng có `Multica` → kết quả `Delegora`; (3) hai bên sửa cùng dòng → conflict thật, thoát 1; (4) fork đổi brand cả dòng mà upstream sửa dòng đó → conflict thật; (5) tệp nhị phân → thoát 1; (6) dòng trùng nhau trong file (hai dòng giống nhau, fork brand một, giữ một) → hàng đợi giữ thứ tự.

Xong khi: test xanh.

### T1.3 `scripts/sync/merge-json.mjs` (M)

Làm gì: driver mục 3.2, `node merge-json.mjs %O %A %B`.

Chi tiết: cổng ổn định định dạng; đệ quy theo key; mảng toàn giá trị đơn qua merge dòng; thứ tự key; áp `brand()` cho chuỗi từ `B`; mọi trường hợp còn lại thoát 1 và **không ghi** vào `%A`. Giữ `\n` cuối tệp như `A`.

Test: ngoài các ca ở mục 3.2, thêm: key chỉ ở `B` chèn đúng chỗ khi `A` đã xóa key đứng kề; giá trị lồng sâu năm cấp; chuỗi có Unicode và `\u`; tệp không ổn định định dạng (`developers/meta.json`) → thoát 1 và không đổi `%A`; JSON lỗi → thoát 1.

Xong khi: test xanh; chạy tay trên ba tệp mẫu của `7b6a1ab8f` cho kết quả như nguyên mẫu (chạy lại `--compare`).

### T1.4 `.gitattributes` và cài đặt (S)

Làm gì:

- Thêm vào `.gitattributes` (giữ các dòng hiện có): `merge=brand` cho các đuôi ở mục 3.1; `merge=jsonmerge` cho locale, `meta*.json`, `reserved_slugs.json` (đặt **sau** dòng `merge=brand` vì thuộc tính dòng sau thắng); `merge=ours` cho file sinh tự động (T1.5).
- `scripts/sync/install.sh`: ghi `merge.brand.driver`, `merge.jsonmerge.driver`, `merge.ours.driver`, `rerere.enabled` vào cấu hình chung của repo bằng `git config`; idempotent; in cái đã làm.
- `scripts/sync/doctor.mjs` (`pnpm sync:doctor`): kiểm cấu hình có đủ, đường dẫn script tồn tại, `git check-attr merge <mẫu file>` đúng; thoát 1 nếu thiếu.
- Thêm script vào `package.json`: `sync:doctor`, `sync:simulate`, `sync:surface`.

Test: `doctor` trên repo tạm chưa cài → 1; sau `install.sh` → 0; `check-attr` đúng cho một locale, một mdx, một file sinh tự động, một ảnh (không có driver).

Xong khi: `pnpm sync:doctor` xanh trên máy này.

### T1.5 File sinh tự động dùng `ours` (S)

Làm gì: chốt danh sách trong `.gitattributes` và `scripts/sync/policy.json` (`regenerate`): `server/pkg/db/generated/**`, `pnpm-lock.yaml`, `packages/views/test/shard-timings.json`, `packages/core/paths/reserved-slugs.ts`, kèm lệnh tái sinh cho từng nhóm và thứ tự (queries và migrations merge xong rồi mới `make sqlc`).

Việc kiểm tay trước khi chốt: đọc `AGENTS.md` và `Makefile` để xác nhận cách sinh từng tệp; xác nhận `shard-timings.json` do công cụ nào sinh (nếu do CI ghi tay thì **không** dùng `ours`, ghi quyết định vào policy).

Xong khi: policy có đủ lệnh tái sinh và đã xác nhận từng tệp là sinh ra được.

### T1.6 Nghiệm thu bằng mô phỏng ba merge cũ (M)

Làm gì: chạy `pnpm sync:simulate --merge <m> --drivers on` cho `7b6a1ab8f`, `ce85c3b61`, `0ecf290b7` (và `e4f7bf2ad` nếu thời gian cho phép), cùng `--compare`.

Cổng nghiệm thu (nếu trượt thì sửa, không hạ ngưỡng):

- Số conflict không vượt nguyên mẫu quá 10%: ≤ 34, 45, 39 (nguyên mẫu 31, 41, 35).
- Không còn conflict ở `locales/**`, `apps/mobile/locales/**`; `meta*.json` chỉ còn khi thật sự xung đột phần tử.
- `--compare`: không có khác biệt chưa giải thích. Mỗi khác biệt phải rơi vào một nhóm đã biết (dấu `…` so với `...`, lựa chọn thủ công của người merge) hoặc được điều tra. Riêng 14 khóa `tab_body.composio_mcp.*` của `es/agents.json` (spec 2.4): đối chiếu với `0b5302a8d` xem fork hiện có hay không có khóa này ở các ngôn ngữ khác, rồi kết luận driver đúng hay merge thủ công bỏ sót; nếu bỏ sót thì sửa trực tiếp ở HEAD.
- Cho driver văn bản: chọn ngẫu nhiên 20 file mdx/ts/go đã tự merge trong ba merge, so tay với merge commit thật sau khi chuẩn hóa brand.

Xong khi: ba số đạt cổng, `--compare` giải thích hết, kết quả ghi vào `ledger.md` và vào spec mục 2.7.

## P2. Tự động hóa quy trình

### T2.1 `scripts/sync/upstream.mjs` (`pnpm sync:upstream`) (L)

Làm gì: mục 3.5, chia hai pha `start` và `--finish`. Đọc `policy.json` (đường dẫn loại `server/ee`, `packages/ee`; tệp tái sinh). Không tự commit, không tự push.

Chi tiết quan trọng:

- Từ chối chạy khi cây bẩn, khi không ở `main`, hoặc `sync:doctor` đỏ.
- Sau merge, in bảng conflict còn lại, gắn nhãn: loại, có thuộc danh sách hot file (spec 2.5), và seam nào (S1–S6) sẽ xử lý nó nếu có.
- `--finish` chạy theo thứ tự: tái sinh theo `policy.json` → `brand:check` → `vi-worklist` → `scripts/sync-audit.mjs --fork <pre-merge>` → `sync:contract` → in checklist lệnh kiểm thử. Dừng ở bước đầu tiên đỏ và nói rõ.
- Ghi trạng thái (`pre-merge sha`, `upstream sha`, ngày) vào `.git/sync-state.json` (không commit) để `--finish` không phải đoán.

Test: repo tạm có đủ các tình huống (file chỉ upstream sửa với `Multica`, file hai bên sửa, `server/ee` bị modify/delete, file sinh tự động); kiểm từng bước và kiểm từ chối khi cây bẩn.

Xong khi: chạy thử `pnpm sync:upstream` trên bản sao repo ở `0b5302a8d` với `upstream/main` hiện tại (nếu chưa có commit mới thì dùng `--upstream <ref cũ>` để tái hiện) cho ra danh sách conflict còn lại đúng loại.

### T2.2 `brand:check` và bước rebrand 3.1b (M)

Làm gì: `scripts/sync/brand-check.mjs` (`pnpm brand:check`) và `scripts/sync/rebrand.mjs` (gọi từ `upstream.mjs`). Bề mặt kiểm là danh sách glob trong `brand-rules.mjs`.

Trước khi coi là cổng CI: chạy trên HEAD hiện tại. Số hit hiện có là **nợ có sẵn**; phân loại từng nhóm (cố ý hay sót). Chỉ bật làm cổng sau khi đã dọn hoặc đưa vào danh sách ngoại lệ có lý do, để cổng không đỏ ngay ngày đầu.

Test: rebrand chỉ đụng dòng trùng tập dòng upstream thêm; không đụng dòng khác trong cùng file; idempotent (chạy hai lần không đổi gì).

Xong khi: `brand:check` xanh trên `main` và test rebrand xanh.

### T2.3 `scripts/sync/vi-worklist.mjs` (S)

Làm gì: mục 3.6. `--list` xuất JSON `[{file, path, en}]` cho khóa EN thiếu ở `vi`; `--apply <file>` ghi bản dịch vào đúng đường dẫn khóa, giữ thứ tự theo EN, báo khóa thừa ở `vi` (không có trong EN). Không tự dịch.

Test: thiếu, thừa, lồng nhau, plural (`_one`/`_other`), thứ tự.

Xong khi: chạy trên HEAD cho danh sách rỗng, và test parity hiện có vẫn xanh.

### T2.4 `scripts/sync/contract.mjs` (`pnpm sync:contract`) (M)

Làm gì: mục 3.7. Bước 1: tôi đọc mã và soạn `fork-contract.json` bản đầu với từng mục kèm lý do và đường dẫn đã mở ra kiểm. Bước 2: người dùng đã giao tôi dựng đầy đủ, không duyệt từng mục. Bước 3: viết `contract.mjs` và test (mỗi loại kiểm tra: xanh, đỏ).

Mỗi mục phải đỏ khi tính năng biến mất. Kiểm bằng cách tạm xóa/sửa đường dẫn tương ứng trong thư mục tạm và chạy; không sửa repo thật.

Xong khi: xanh trên `main` hiện tại, và thử đỏ có chủ đích trên bản sao cho ít nhất năm mục.

### T2.5 (đã bỏ)

Người dùng quyết định không chạy định kỳ trên CI; chỉ sync khi được yêu cầu. Không có workflow mới, không đẩy gì lên `origin` cho việc này.

## P3. Seam (chỉ làm cho file thực sự lặp lại)

Mỗi mục theo cùng một khuôn:

1. Liệt kê khối fork trong file: `git diff -U0 upstream/main main -- <file>`, phân loại nối đuôi, trong hàm, xóa (spec 3.9).
2. Viết (hoặc xác nhận) test bao phủ vùng sắp tách, chạy xanh **trước** khi sửa.
3. Tách theo cách của mục; không đổi hành vi.
4. Chạy lại test vùng đó, `typecheck`/`go build`, rồi `make test`/`pnpm test` nối tiếp.
5. Cập nhật bảng "hot file" trong `policy.json` và `ledger.md`; commit riêng, một seam một commit.

| Task | Mục tiêu | Công | Điều kiện để làm |
|---|---|---|---|
| T3.1 (S1) | `server/cmd/migrate/main.go` → `fork_index_cleanups.go` | S | luôn làm (3/3 merge); test `go test ./cmd/migrate ./internal/migrations` trên `multica_test` |
| T3.2 (S2) | `handler/issue.go`, `service/task.go` | L | sau T3.1; làm `issue.go` trước, `task.go` sau |
| T3.3 (S3) | registry cho `issue-detail.tsx`, rồi `chat-page.tsx`, `work-panel.tsx`, `settings-layout.tsx`, `settings-page.tsx` | L | thử registry trên **một** file; nếu chi phí quá lớn so với lợi ích thì dừng và ghi vào sổ |
| T3.4 (S4) | `client.ts`, `schemas.ts` | L | **trước** đó đếm phương thức fork và dựng thử cơ chế khai báo gộp trên 3–5 phương thức; kiểm `sideEffects`, `this`, tree-shaking, typecheck mọi workspace |
| T3.5 (S5) | `router.go` → `router_fork.go` | M | sau T3.2 |
| T3.6 (S6) | `packages/core/paths/*`, `routes.tsx` | M | sau T3.3 |

Cổng chung của P3: sau mỗi seam chạy lại `sync:surface`; số file sửa không được tăng. Hiệu quả thật chỉ đo được ở các lần sync sau (spec 3.9), nên sau mỗi lần sync thật điền sổ và xem hot file nào còn lặp.

Quy tắc dừng: nếu hai lần sync liên tiếp sau P1–P2 mà một mục seam không còn nằm trong danh sách conflict, không làm mục đó nữa.

## P4. Migration

### T4.1 Điều tra số hiệu migration (M, độc lập)

Làm gì: trả lời bằng thử nghiệm các câu "chưa biết" ở spec 3.12 và ghi quyết định vào cuối spec:

1. Runner áp dụng migration chưa chạy theo thứ tự nào trên một DB đã có `800_x`, khi xuất hiện `743_y` của upstream (dùng DB tạm hoặc `multica_test` sao lưu, không đụng `multica`).
2. Sắp xếp theo chuỗi với `1000_`…`1040_` so với `7xx_`: tác dụng thực tế.
3. `TestMigrationNumericPrefixesAreUnique` của fork đang cho phép gì (đọc `migrations_lint_test.go`, +238 dòng).
4. Phương án dải riêng cho fork (ví dụ tiền tố riêng hoặc thư mục riêng) và chi phí; cái gì sẽ bị đụng (`server/cmd/migrate/main.go`, `scripts/check-migration-numbers.mjs`, `.github/workflows/migration-numbers.yml`).

Đầu ra: một đoạn quyết định (giữ như hiện nay, hoặc đổi theo phương án nào) cùng bằng chứng. **Không đổi tên migration đã áp dụng.** Nếu quyết định là đổi quy ước cho migration mới, đó là bước riêng sau khi người dùng đồng ý.

## P5. Chốt

### T5.1 Tài liệu và quy tắc (S)

- `docs/fork-branding.md`: thay mục "Rules for future syncs" bằng quy trình dùng driver, `brand:check`, `sync:upstream`.
- `AGENTS.md`: thêm một quy tắc ngắn về mã fork mới (spec 3.9) và dòng lệnh `pnpm sync:*`; theo quy ước của file, không thêm lịch sử sự cố. Cập nhật mọi nơi liệt kê script nếu `scripts/` có danh sách (đọc `CONTRIBUTING.md`).
- Bộ nhớ: cập nhật `upstream-sync-progress.md` (quy trình mới, cách chạy, driver, quyết định) và thêm một ghi nhớ ngắn "cách sync".

Xong khi: tài liệu khớp với lệnh thật; một người chưa biết gì làm theo được.

### T5.2 Sync thật đầu tiên bằng quy trình mới (M, phụ thuộc có commit mới từ upstream)

Làm gì: chạy `pnpm sync:upstream`, giải conflict còn lại, `--finish`, rồi ghi vào `ledger.md` số conflict thật, thời gian người bỏ ra, và mọi lần công cụ làm sai hoặc thiếu. Sửa công cụ theo đó trước khi coi là xong.

Xong khi: một sync kết thúc mà `sync:contract` xanh và sổ có dòng mới. So với bảng 2.1, số conflict còn lại nên ở mức của nguyên mẫu hoặc thấp hơn; nếu cao hơn đáng kể thì mở điều tra, không báo là thành công.

## 2. Tiêu chí thành công (đo được)

| Chỉ số | Hiện tại | Mục tiêu |
|---|---|---|
| Conflict mô phỏng (7b6a1ab8f / ce85c3b61 / 0ecf290b7) | 97 / 75 / 85 | ≤ 34 / 45 / 39 sau P1 |
| Conflict ở `locales/**` và file sinh tự động | 70 / 36 / 56 file | 0 sau P1 (trừ conflict thật ở một key) |
| Phát hiện mất tính năng | làm tay | `sync:contract` + `sync-audit` chạy trong `--finish` |
| Khóa `vi` thiếu sau sync | làm tay (132 khóa ở sync #4) | danh sách tự sinh |
| Hot file lặp lại (≥2/3 merge) | 10 file | giảm sau P3, ghi vào sổ mỗi lần sync |
| Bề mặt file sửa thật so với upstream | 974 | không tăng (baseline T0.2) |

Mục tiêu ở bảng này do mô phỏng quá khứ cho phép kiểm chứng (P1). Mục tiêu ở P3 chỉ kiểm chứng được qua các lần sync sau; tôi sẽ không báo cáo P3 là "đã giảm conflict" khi chưa có số của một lần sync thật.

## 3. Rủi ro của kế hoạch và cách xử lý

- **Driver đúng trên mô phỏng nhưng sai trên bản merge tương lai.** T1.6 chỉ chứng minh trên ba merge. Chặn: sync thật đầu tiên (T5.2) chạy thêm `sync:simulate --compare` sau khi người dùng giải xong, và `sync-audit`, `contract`, test toàn bộ.
- **Công sức vượt lợi ích.** P1 và P2.1–P2.2 có số liệu chống lưng (spec 2.3) nên làm đầy đủ. P2.4, P2.5, P3 làm tiếp chỉ khi sổ cho thấy còn đáng; mỗi bước độc lập nên dừng ở bất kỳ điểm nào mà vẫn có giá trị.
- **Quy trình mới làm phát sinh bảo trì công cụ.** Chặn bằng test, `sync:doctor`, và cổng "không phụ thuộc gói ngoài".
- **Phần kiểm tay còn tồn đọng từ sync #4** (ghi trong bộ nhớ): 16 test `platform-management/console` đỏ có sẵn, `TestAgentTaskHistoryIndexMigrationAndPagePlans` đỏ có sẵn, ghi chú billing trial/add-on chưa kiểm. Không thuộc kế hoạch này; nên xử lý riêng để cổng "test xanh" của sync sau có nghĩa.

## 4. Việc cần người dùng quyết định để bắt đầu

Đã chốt 2026-11-24: không chạy định kỳ trên CI, hợp đồng tính năng do tôi dựng đầy đủ, commit tài liệu lên `main`. P0 và P1 bắt đầu ngay.


## Sau sync #5 (2026-11-24): việc chờ điều kiện

Sync #5 còn 27 conflict: ~11 là hai bên cùng thêm vào một chỗ (không đáng tách), ~10 ở file fork sửa nặng, 3 là phần dư của seam S2/S4, 4 là va chạm mới (đã xử lý bằng đổi tên `*_fork.go`). Đã làm: cảnh báo trùng đường dẫn trong `sync:upstream`, và `finishChecks` chạy golden route, kiểm kê tiền tệ, fixture entitlement.

Kết quả (2026-11-24, ngay sau sync #5):
- S8 **đã làm**: `entitlement/types.go` và `client.go` giờ chỉ lệch upstream 3 và 1 dòng (hook `forkGateValid`, `normalizeForkGates`); cổng của fork nằm ở `gates_fork.go`. Test entitlement, kiểm kê tiền tệ, golden route đạt.
- S7 **không làm**, theo đúng quy tắc dừng: (a) `billing-tab.tsx` của fork là một thành phần khác hẳn (+1.471/−1.259 so với upstream) và dựa trên kiểu và hook billing đã phân kỳ trong `core` (`planChange`, `trial`, `checkPlanChange`...), nên không thể giữ nguyên file upstream bên cạnh: thử rồi, upstream `billing-tab.tsx` và test của nó không biên dịch được trong cây fork; (b) `squad-detail-page.tsx` của fork đổi cấu trúc (`SquadDetailPage` → `SquadDetailView`, thêm props) và xen hook lẫn JSX của upstream, nên không có khối nguyên để chuyển. Hai file này đánh dấu trong `policy.json` (`hotFiles`) để lần sau báo rõ là phải mang diff của upstream sang bằng tay.
