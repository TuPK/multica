# Giảm conflict khi sync upstream mà không mất tính năng fork: đặc tả

Trạng thái: **đặc tả đã chốt hướng, chưa có mã.** Kế hoạch từng bước: [plan.md](plan.md). Hai tài liệu này là nguồn duy nhất cho việc triển khai.

Ngày rà soát: 2026-11-24. Đọc mã ở `main` = `origin/main` = `0b5302a8d` (sync #4), `upstream/main` = `6368504e1`. Mọi con số dưới đây do tôi đo bằng lệnh ở mục 9; số nào là ước tính thì ghi "ước tính".

Ngôn ngữ: tiếng Việt. Tên tệp, lệnh, mã giữ tiếng Anh. Chú thích trong mã viết tiếng Anh.

## 1. Vấn đề, mục tiêu, không mục tiêu

**Vấn đề.** Bốn lần sync gần nhất (2026-11-22 đến 2026-11-24) tốn nhiều công giải conflict. Mô phỏng lại đúng bốn lần merge đó (mục 2) cho 301, 97, 75 và 85 file conflict. Sau mỗi lần, việc kiểm tra "có mất tính năng fork không" còn làm tay (`scripts/sync-audit.mjs` chỉ liệt kê ứng viên, mỗi hit phải đọc).

**Mục tiêu.**

- G1. Số file conflict mỗi lần sync giảm rõ rệt, đo được bằng mô phỏng lặp lại (mục 2, 5).
- G2. Mất tính năng fork thì bị phát hiện tự động ngay sau merge, không dựa vào việc nhớ (mục 3.7).
- G3. Một lần sync là một lệnh chạy được, các bước máy làm được thì máy làm (mục 3.5).
- G4. Phần mã fork sửa chồng lên file upstream không phình thêm; chỗ hay conflict dần được tách ra (mục 3.8, 3.9).

**Không mục tiêu.**

- Đổi tên định danh (`multica`, `MULTICA_*`, `@multica/*`): đã chốt giữ nguyên (`docs/fork-branding.md`).
- Bỏ sync upstream (hard fork), dùng patch-stack, hoặc đưa upstream thành submodule/vendor cộng lớp phủ. Đã cân nhắc, bị loại (mục 7).
- Tự giải conflict ngữ nghĩa (hai bên cùng sửa một logic). Việc đó vẫn do người làm.
- Giảm số tính năng fork hoặc đổi quyết định đã chốt: giữ billing và platform-management của fork, entitlement là hợp của hai bên, loại `server/ee` và `packages/ee`, dùng mô hình project private/guest của upstream.

## 2. Dữ liệu đo (đã chạy thật)

### 2.1 Bốn lần sync gần nhất

Phương pháp: với mỗi merge commit `M` của fork, tạo worktree tại `M^1`, chạy `git merge --no-commit --no-ff M^2` với `rerere` tắt, ghi danh sách file chưa merge (`git diff --diff-filter=U`), rồi `merge --abort`. Đây đúng là trạng thái người dùng gặp lúc đó.

| Merge | Ngày | Commit upstream | Conflict (gốc) | Còn lại sau 3 driver (mục 3.1–3.3) |
|---|---|---|---|---|
| `e4f7bf2ad` | 11-22 | 588 | 301 | chưa mô phỏng |
| `7b6a1ab8f` | 11-22 | 53 | 97 | 31 |
| `ce85c3b61` | 11-23 | 27 | 75 | 41 |
| `0ecf290b7` | 11-24 | 114 | 85 | 35 |

Số commit upstream không tỉ lệ thuận với số conflict: 27 commit vẫn cho 75 file. Nguyên nhân là loại file, không phải khối lượng.

### 2.2 Conflict gốc theo loại (ba lần cuối)

| Loại | 7b6a1ab8f | ce85c3b61 | 0ecf290b7 |
|---|---|---|---|
| `locales/**/*.json` | 67 | 24 | 14 |
| docs, README, `.md(x)` | 0 | 0 | 32 |
| sinh tự động và lockfile (`generated/`, `pnpm-lock.yaml`) | 3 | 12 | 10 |
| mã TS | 10 | 20 | 10 |
| mã Go | 2 | 5 | 9 |
| test | 9 | 10 | 6 |
| cấu hình và build | 4 | 1 | 3 |
| khác | 2 | 3 | 1 |

Locale, docs và file sinh tự động chiếm 70/97, 36/75, 56/85 file. Đây là loại có thể máy hóa.

### 2.3 Tác động của ba driver (nguyên mẫu, chạy trên đúng ba merge trên)

Ba driver: (a) merge văn bản có nhận biết brand, (b) merge JSON theo từng key cho locale và `meta*.json`, (c) `ours` cho `generated/**` và `pnpm-lock.yaml`. Kết quả ở bảng 2.1: 97→31, 75→41, 85→35.

Riêng brand+`ours` không đủ (97→94, 75→64, 85→44). Locale chỉ hết conflict khi có driver JSON theo key: lý do là locale conflict do hai bên thêm hoặc xóa key sát nhau trong cùng khối dòng, không phải do chữ Delegora. Đây là phát hiện quan trọng: đề xuất ban đầu "đổi brand lúc chạy" sẽ không giải quyết được phần lớn conflict locale.

### 2.4 Driver có làm sai kết quả không

Với file JSON locale và `meta*.json` mà driver tự merge xong, so với tệp trong merge commit thật (chuẩn hóa thứ tự key và Delegora→Multica):

- `0ecf290b7`: 62/62 giống.
- `ce85c3b61`: 48/48 giống.
- `7b6a1ab8f`: 173/192 giống, 19 khác. Hai nhóm đã xem:
  - Dấu `…` (driver, theo upstream) so với `...` (tệp thật, do người sửa sau merge). Test `copy-hygiene` của fork bắt loại này.
  - `es/agents.json` có 14 khóa `tab_body.composio_mcp.*` driver giữ, tệp thật không có. **Chưa giải thích được** (có thể người merge cố ý bỏ, có thể bỏ sót). Là việc rà soát ở bước T1.6 của kế hoạch, không được bỏ qua.
- Độ phủ: mẫu này chỉ gồm JSON. Driver văn bản (3.1) chưa được so với merge thật; bước T1.6 làm việc đó.

### 2.5 File hay conflict lặp lại (sau ba driver, xuất hiện ≥2 trong 3 merge)

`server/internal/handler/issue.go` (3/3), `server/cmd/migrate/main.go` (3/3), `packages/views/issues/components/issue-detail.tsx` (3/3), `server/internal/service/task.go` (2), `packages/views/settings/components/billing-tab.test.tsx` (2), `packages/views/layout/panels/work-panel.test.tsx` (2), `packages/views/chat/chat-page.tsx` (2), `packages/core/api/client.ts` (2), `apps/docs/content/docs/meta.json` (2, mảng nên driver JSON nguyên mẫu chưa xử lý), `apps/desktop/src/renderer/src/routes.tsx` (2).

Còn lại 31–41 file/lần phần lớn là conflict thật do cùng một logic bị sửa ở hai phía. Ngoài danh sách trên còn các nhóm lặp theo thư mục: `packages/core/paths/*`, `packages/views/settings/components/*`, `e2e/*`.

### 2.6 Bề mặt fork chồng lên upstream (so `main` với `upstream/main`)

- Đo bằng `pnpm sync:surface` (`HEAD` = `316f1aa63`, `upstream/main` = `6368504e1`): **7.544 file thêm mới** (không bao giờ conflict), **1.732 file sửa**, 94 file xóa. Trong 1.732 file sửa, **758 chỉ khác chữ brand** (so sánh chặt: sau khi đổi Delegora thành Multica thì tập dòng đổi hai phía bằng nhau) và **974 là sửa thật**. Bốn thư mục có nhiều file sửa thật nhất: `packages/views/locales` (122), `server/internal/handler` (114), `apps/docs/content` (105), `server/pkg/db` (58).
- Lần đo thô đầu tiên (lệnh `git diff --name-status` loại thư mục locale) cho 7.460/1.576/86; con số đúng là số ở trên vì đã tính cả locale.
- `packages/core/api/client.ts`: 249 khối thay đổi rải khắp một class 9.909 dòng (+4.193/−887). `schemas.ts`: 90 khối (+2.306/−319). `server/cmd/server/router.go`: 94 khối (+676/−53). `handler/issue.go`: 70 khối (+594/−31). `service/task.go`: 58 khối. Fork **chèn xen kẽ**, không nối đuôi, nên mỗi lần upstream sửa gần đó là một conflict.
- Các số "−" lớn (client.ts −887) cho thấy fork cũng **xóa** mã upstream. Xóa trong khi upstream sửa chính đoạn đó là loại conflict modify/delete.

### 2.7 Kết quả nghiệm thu P1 (công cụ trong repo, chạy ngày 2026-11-24)

`pnpm sync:simulate --merge <commit> --compare`, driver lấy từ `scripts/sync/`:

| Merge | Gốc | Với driver (nguyên mẫu) | Với driver (trong repo) | Cổng ở plan T1.6 |
|---|---|---|---|---|
| `7b6a1ab8f` | 97 | 31 | **29** | ≤ 34 |
| `ce85c3b61` | 75 | 41 | **37** | ≤ 45 |
| `0ecf290b7` | 85 | 35 | **30** | ≤ 39 |

(Số của `ce85c3b61` và `0ecf290b7` giảm so với lần đo đầu nhờ thêm `package.json` và hợp mảng chèn vào driver JSON.)

Kiểm tính đúng, hai phép so:

1. Với file git tự merge được cả khi không có driver (781, 415 và 857 file), kết quả có driver và không driver **giống nhau** sau khi coi Delegora = Multica và bỏ qua thứ tự key JSON: 0 khác biệt ở cả ba merge. Nghĩa là driver không làm đổi kết quả ở nơi git vốn đã đúng.
2. Với file chỉ nhờ driver mới hết conflict (68, 38 và 49 file), so với merge commit thật: `ce85c3b61` và `0ecf290b7` 0 khác biệt; `7b6a1ab8f` 14 khác, **toàn bộ là file locale JSON**. Nguyên nhân đã rõ:
   - Khóa `tab_body.composio_mcp.*` (14 khóa mỗi ngôn ngữ ở các file `agents.json`): merge thật `7b6a1ab8f` **bỏ mất khóa này ở cả bản `en`**; ở `main` hiện nay khóa này có đủ (đã khôi phục sau). Driver giữ đúng ngay từ đầu. Đây là ví dụ thật về việc merge tay làm mất nội dung và phải vá sau.
   - Dấu `…` (upstream, driver giữ) so với `...` (người sửa sau merge theo `copy-hygiene`).

Giới hạn: 59 file khác nhau tổng cộng trong `--compare` còn lại là file git tự merge sạch nhưng merge commit thật có sửa thêm (sửa ngữ nghĩa sau merge, ví dụ thêm import, đổi chữ ký hàm). Đó là việc của người, không phải lỗi driver.

Phân loại các dòng fork giữ `Multica` có chủ ý (kiểm trên HEAD với `brand-rules.mjs`; 69 file, 246 dòng không thỏa `brand(canon(x)) == x`): tên giấy phép `Multica License` (ngoại lệ trong quy tắc), dòng ví dụ `@Multica /help` và vài đoạn docs ngôn ngữ ko/ja/fr/zh mà fork chưa đổi (nợ brand, `brand:check` sẽ báo), và các tệp trong `docs/` tự mô tả quy tắc. Đường dẫn `Delegora.app` được quy tắc xử lý (`/Multica.app` → `Delegora.app`).

### 2.8 Chạy thử đầu cuối (P2)

Tái hiện sync #4 trong bản clone: lấy `c7603c040` (fork trước sync #4), phủ công cụ lên trên, rồi `pnpm sync:upstream --no-fetch` với `upstream` = `6368504e1`. Kết quả: 114 commit, **36 conflict** (lần chạy đầu, trước khi thêm `package.json` và hợp mảng chèn; mô phỏng sau đó cho 30). Danh sách trùng với các file sync #4 thật phải giải tay (`daemon.go`, `dashboard.go`, `issue.go`, `task.go`, `billing-tab.tsx`, `issue-detail.tsx`, ...) và các file hot được gắn nhãn seam. Bước `--finish` ra đúng **131 khóa `vi` thiếu** (sync #4 thật phải điền 132). Lần thử này cũng lộ một lỗ hổng: `git add` file còn marker conflict làm git coi như đã giải; nay `--finish` kiểm marker ở bước đầu, và `sync:contract` báo lỗi thay vì sập khi JSON hỏng.

## 3. Thiết kế

Nguyên tắc chung: mọi công cụ **an toàn khi nghi ngờ**. Gặp điều không chắc chắn thì thoát mã 1, git để nguyên conflict kiểu thường. Không có đường nào cho công cụ tự ý "đoán" rồi ghi kết quả.

### 3.1 Driver văn bản có nhận biết brand (`merge=brand`)

Áp cho `*.md, *.mdx, *.go, *.ts, *.tsx, *.mjs, *.yml, *.yaml, *.sh, *.jsonc, *.txt` (không phải JSON locale, không phải file sinh tự động).

Thuật toán (đã chạy thử ở nguyên mẫu):

1. Chuẩn hóa ba phiên bản `O` (gốc), `A` (fork), `B` (upstream): `Delegora`→`Multica`.
2. `git merge-file` ba bản đã chuẩn hóa.
3. Với mỗi dòng kết quả, nếu chuẩn hóa của một dòng trong `A` bằng dòng đó thì **lấy nguyên dòng gốc của `A`** (giữ lựa chọn brand của fork, kể cả chỗ cố ý giữ `Multica`); ngược lại là dòng mới từ upstream, áp `brand()`.
4. Có marker conflict thì thoát 1 (giữ marker, người giải).

Lý do thiết kế "giữ dòng gốc của A": đo trên HEAD hiện tại cho thấy 141/1.269 file có chữ Delegora không thỏa `brand(canon(x)) == x`, vì fork cố ý giữ `Multica` ở những chỗ theo `docs/fork-branding.md` (tiêu đề, bản quyền, Canary, header). Áp `brand()` cho cả tệp sẽ phá các chỗ đó.

`brand()` chỉ thay `Multica` (đúng chữ, phân biệt hoa thường) khi: không dính `-`, `@`, `/`, `.`, ký tự chữ-số-gạch dưới ở hai bên, không là `© Multica`, không là `Multica Canary`. Quy tắc nằm trong **một** module `scripts/sync/brand-rules.mjs` mà driver, bước rebrand và `brand:check` dùng chung.

Giới hạn đã biết: git chỉ gọi driver khi **cả hai phía** sửa file. File chỉ upstream sửa được git lấy thẳng bản upstream, dòng mới mang chữ `Multica`. Nên cần bước 3.1b.

**3.1b Bước rebrand sau merge.** Với mỗi file mà upstream sửa mà fork không sửa kể từ merge-base: lấy tập dòng upstream thêm (`git diff base upstream -U0`), trong kết quả merge dòng nào trùng chính xác một dòng trong tập đó thì áp `brand()`. Chỉ đụng dòng trùng, nên không phá chỗ fork cố ý giữ. Kết quả được kiểm bằng `brand:check` (3.1c).

**3.1c `brand:check`.** Quét các bề mặt người dùng đọc (tập glob trong `brand-rules.mjs`: `packages/views/locales/**`, `apps/web/features/landing/i18n/**`, `apps/mobile/locales/**`, `README*.md`, `SELF_HOSTING*.md`, `apps/docs/content/**` phần văn xuôi) và báo mọi `Multica` không thuộc ngoại lệ. Chạy trong CI và sau merge. Thay cho việc người rà bằng mắt như sync #4.

### 3.2 Driver JSON theo key (`merge=jsonmerge`)

Áp cho `packages/views/locales/**/*.json`, `apps/mobile/locales/**/*.json`, `apps/docs/content/**/meta*.json`, `server/internal/handler/reserved_slugs.json` và mọi `package.json` (phát hiện ở lần chạy thử đầu: `apps/desktop/package.json` conflict vì fork đổi `productName` ngay dòng trên `version` mà upstream nâng) (danh sách trong `.gitattributes`).

Quy tắc, theo từng đường dẫn khóa, ba bản `O`/`A`/`B` (so sánh sau chuẩn hóa brand):

- `A==B`: giữ. `O==A`: lấy `B`. `O==B`: lấy `A`.
- Cả hai đổi và cả hai là object: đệ quy.
- Mảng toàn giá trị đơn (chuỗi, số): merge theo dòng, mỗi phần tử một dòng, bằng `git merge-file`. Nếu hai bên **chỉ chèn thêm** phần tử (bản gốc là dãy con của cả hai) và vướng nhau tại cùng một chỗ, giữ cả hai, bản fork trước (ví dụ `pages` của `meta.json`). Mọi trường hợp khác có conflict thì rơi về merge dòng.
- Còn lại (cả hai sửa cùng giá trị, hoặc một bên xóa một bên sửa): thoát 1.
- Thứ tự key: giữ thứ tự của `A`, chèn key mới của `B` ngay sau key đứng trước nó ở `B`.
- Chuỗi từ `B` đi vào kết quả được áp `brand()`.
- **Cổng ổn định định dạng:** nếu `stringify(parse(A), null, 2) + "\n"` khác `A`, driver thoát 1 (đo: 331/337 file JSON đang ổn định; 6 file `developers/meta*.json` thì không, và sẽ rơi về merge dòng).

Test bắt buộc (mục 5): thêm key hai phía cạnh nhau, xóa một phía, một phía xóa một phía sửa (conflict), mảng, thứ tự key, brand, định dạng không ổn định.

### 3.3 File sinh tự động và khóa (`merge=ours` + tái sinh)

`server/pkg/db/generated/**`, `pnpm-lock.yaml`, `packages/views/test/shard-timings.json`, `packages/core/paths/reserved-slugs.ts` dùng driver `ours` (git không có sẵn: `install.sh` định nghĩa `merge.ours.driver = true`, giống cấu hình đã dùng khi mô phỏng), **rồi bắt buộc tái sinh** bằng `make sqlc`, `pnpm install`, `pnpm generate:reserved-slugs`, `pnpm catalog:build` (đúng như `AGENTS.md`). Bước tái sinh nằm trong orchestrator (3.5), không để người nhớ. `generated/**` chỉ đúng sau khi cả `server/pkg/db/queries/**` lẫn `server/migrations/**` đã merge xong, nên tái sinh chạy **sau** khi mọi conflict của hai thư mục đó được giải.

Lưu ý: `ours` chỉ được gọi khi cả hai phía sửa file; nếu chỉ upstream sửa, git lấy bản upstream, và tái sinh vẫn chạy để chắc chắn.

### 3.4 Đường dẫn bị loại theo chính sách

`server/ee` và `packages/ee` (license) không tồn tại ở fork. Khi upstream sửa chúng, git báo modify/delete (đã thấy `packages/ee/eslint.config.mjs`, `packages/ee/project-folders/locales/zh-Hans.json` trong mô phỏng `7b6a1ab8f`). Orchestrator chạy `git rm -r` các đường dẫn trong `scripts/sync/policy.json` (`excluded`), và nếu code đang import chúng thì typecheck bắt.

### 3.5 Orchestrator `pnpm sync:upstream`

Một lệnh, không tự commit và không tự push:

1. Tiền kiểm: cây sạch, đang ở `main`, `pnpm sync:doctor` báo driver đã cài, `git fetch upstream`.
2. Tạo nhánh `chore/sync-upstream-<ngày>`.
3. `git merge --no-commit --no-ff upstream/main` với driver bật.
4. Xóa đường dẫn loại theo chính sách (3.4).
5. Rebrand file chỉ upstream sửa (3.1b).
6. In danh sách conflict còn lại, gắn nhãn theo bảng hot file (2.5) và theo loại.
7. Dừng để người (hoặc tôi) giải conflict thật.
8. `pnpm sync:upstream --finish`: tái sinh (3.3), `brand:check`, danh sách khóa `vi` thiếu so với EN (3.6), `scripts/sync-audit.mjs`, `sync:contract` (3.7), in checklist kiểm tra (`pnpm typecheck`, `pnpm lint`, `pnpm test`, `make test`, kiểm tra migration, chạy bằng nối tiếp khi máy yếu như đã gặp).

Orchestrator là tập lệnh Node không phụ thuộc gói ngoài, có test, chạy được trên repo giả trong thư mục tạm.

### 3.6 Locale `vi` do fork sở hữu

`vi` là ngôn ngữ chỉ có ở fork, test parity bắt buộc đủ mọi khóa EN. Mỗi lần sync, upstream thêm khóa EN thì `vi` thiếu. `scripts/sync/vi-worklist.mjs` xuất danh sách khóa thiếu (đường dẫn khóa + chữ EN) ra JSON, và `--apply <file>` ghi bản dịch đã điền vào đúng chỗ, giữ thứ tự. Việc dịch vẫn do người hoặc tôi làm; công cụ chỉ lo phần cơ học (sync #4 phải làm 132 khóa bằng script tạm).

### 3.7 Hợp đồng tính năng fork (`sync:contract`)

Tệp `scripts/sync/fork-contract.json` liệt kê kiểm tra tĩnh, mỗi mục một tính năng và lý do:

- `file-exists`, `file-matches` (regex phải có), `file-lacks` (regex không được có), `json-has` (đường dẫn khóa), `path-absent` (ví dụ `server/ee`).

Danh sách đầu tiên do tôi dựng từ mã hiện tại và duyệt cùng người dùng (mục 8): billing (`server/internal/handler/cloud_billing.go`, `packages/views/settings/components/billing-tab.tsx`), entitlement hợp của hai bên, `platform-management/`, workflow theo room (`server/internal/issueworkflow`, `packages/core/issue-workflows`), công ty (`packages/views/company/components/company-board-dialog.tsx`), Blue White mặc định (`packages/core/appearance`), locale `vi`, `server/ee` và `packages/ee` vắng mặt, tên hiển thị Delegora ở desktop (`apps/desktop/package.json`, `electron-builder.yml`). Lệnh chạy được độc lập và trong CI. Nó bổ sung chứ không thay `sync-audit.mjs` (cái đó tìm dòng bị mất, cái này khẳng định tính năng còn).

### 3.8 Ngân sách bề mặt (`sync:surface`)

`scripts/sync/surface.mjs` đếm file mà fork sửa so với `upstream/main` (trạng thái A/M/D), theo nhóm thư mục, và so với `docs/specs/upstream-sync/surface-baseline.json`. CI báo (cảnh báo, không chặn ở giai đoạn đầu) khi số file sửa tăng mà baseline chưa cập nhật có lý do. Mục tiêu là đo xu hướng. Baseline ban đầu là số ở 2.6.

### 3.9 Đường nối (seam) cho file hay conflict

Quy tắc cho mã fork mới và cho việc dọn dần mã cũ:

1. Mã mới của fork nằm trong file do fork sở hữu (`*_fork.go`, `*.fork.ts`, thư mục fork). Chỉ sửa file upstream để thêm **một dòng gắn** (gọi hàm hoặc đăng ký).
2. **Gỡ nối, không xóa** mã upstream khi có thể (không đăng ký route, không gọi hàm), vì xóa trong vùng upstream đang sửa là conflict modify/delete.
3. Mỗi file hot được dọn theo quy trình: liệt kê khối fork (khối nối đuôi → chuyển sang file fork; khối nằm trong hàm → tách hàm fork + một dòng gọi; khối xóa → gỡ nối), rồi kiểm lại bằng mô phỏng và test. Không refactor lan sang chỗ khác.

Ưu tiên theo tần suất conflict (mục 2.5) rồi theo kích thước bề mặt (2.6). Mỗi mục là một thay đổi nhỏ, có test, commit riêng:

| # | Mục tiêu | Cách tách | Ghi chú |
|---|---|---|---|
| S1 | `server/cmd/migrate/main.go` (3/3) | map `concurrentIndexCleanups` của fork sang `fork_index_cleanups.go`, gộp lúc khởi tạo | 4 khối, +187 dòng; đơn giản nhất |
| S2 | `handler/issue.go` (3/3), `service/task.go` (2) | tách logic flow/workflow của fork ra `issue_fork.go`, `task_fork.go`; chỗ còn lại là một dòng gọi | cần đọc từng khối; có test DB, chạy trên `multica_test` |
| S3 | `issue-detail.tsx` (3/3), `chat-page.tsx`, `work-panel.tsx`, `settings-layout.tsx`, `settings-page.tsx` | đăng ký phần fork vào registry (hàng thuộc tính, tab) thay vì chèn JSX | thiết kế registry cần thử trên `issue-detail.tsx` trước, rồi mới nhân rộng |
| S4 | `packages/core/api/client.ts`, `schemas.ts` | phương thức và schema của fork sang `client.fork.ts`, `schemas.fork.ts`; ghép bằng khai báo `interface ApiClient` + gán prototype | cần kiểm `sideEffects`, kiểu `this`, và mọi chỗ gọi (~746 phương thức `async` trong class; bao nhiêu cái của fork **chưa đếm**, T3.4 đếm trước) |
| S5 | `server/cmd/server/router.go` | route fork vào `router_fork.go`, upstream chỉ gọi `registerForkRoutes` | 94 khối; làm sau S2 |
| S6 | `packages/core/paths/*`, `routes.tsx` | đường dẫn và route fork vào file fork | |

Lợi ích của S1–S6 **không đo được bằng mô phỏng quá khứ** (lịch sử cũ không có seam); chỉ xác nhận được qua các lần sync sau. Vì vậy mỗi lần sync ghi lại số conflict vào sổ (mục 3.10) để thấy xu hướng, và S1–S6 chỉ làm cho các file thực sự lặp lại.

### 3.10 Mô phỏng, sổ số liệu và chạy định kỳ

- `pnpm sync:simulate [--against <ref>] [--compare <merge-commit>]`: làm đúng phương pháp ở 2.1 trong worktree tạm, in số conflict theo loại. `--compare` so từng file tự merge với merge commit thật (cách ở 2.4).
- Sổ `docs/specs/upstream-sync/ledger.md`: mỗi lần sync thêm một dòng (ngày, số commit upstream, conflict gốc nếu biết, conflict còn lại, thời gian người bỏ ra, mất tính năng nào sau đó).
- **Không có chạy định kỳ trên CI** (quyết định của người dùng, 2026-11-24): chỉ sync khi người dùng yêu cầu. `sync:simulate` chạy theo yêu cầu, và là bước đầu của mỗi lần sync để biết trước quy mô. Khoảng cách giữa hai nhánh vẫn ảnh hưởng số conflict (bảng 2.1: sync 588 commit gây 301 file), nên người dùng có thể chủ động yêu cầu sync sớm hơn.

### 3.11 `rerere`

Bật `rerere.enabled` trong `scripts/sync/install.sh`. Kỳ vọng thấp: nó chỉ giúp khi cùng một cặp hunk quay lại, mà upstream luôn đổi. Chi phí gần bằng 0 nên vẫn bật.

### 3.12 Số hiệu migration (điều tra, chưa quyết)

Dữ kiện đã kiểm:

- Upstream đang ở `742` (sync #4 thêm 732–742). Fork có 358 số riêng trong dải 595–1040; 125 số trùng số của upstream nhưng khác tên tệp (ví dụ `604_office_floor_workspace_position_idx` của fork và `604_wecom_install_session_begin_rate_idx` của upstream).
- Phiên bản migration là tên tệp bỏ hậu tố (`migrations.ExtractVersion`), nên hai tệp cùng số vẫn khác phiên bản; `migrations.go` sắp tên theo **chuỗi** (`sort.Strings`), không theo số.
- Test `TestMigrationNumericPrefixesAreUnique` của upstream bắt số trùng từ 129; fork đã sửa `migrations_lint_test.go` (+238 dòng) để chạy được.
- Upstream chạy ~4 migration mỗi ngày trong đợt sync này (11 trong 114 commit); dải 743–1040 của fork sẽ bị upstream đi vào trong vài tháng (ước tính, chưa theo dõi tốc độ dài hạn).

Chưa biết: thứ tự áp dụng thực tế khi một DB đã chạy migration fork `800_x` rồi nhận migration upstream `743_y` sau đó; test lint đã sửa cho phép gì; số `1000+` sắp xếp trước `7xx` theo chuỗi có gây vấn đề không. T4.1 điều tra và đưa ra quyết định có kèm thử nghiệm trên `multica_test`, không đoán ở đây. Không đổi tên migration đã chạy ở bất kỳ DB nào.

## 4. Rủi ro và cách chặn

| Rủi ro | Chặn bằng |
|---|---|
| Driver ghi sai im lặng | Thoát 1 khi nghi ngờ; test đơn vị; `--compare` với merge thật (2.4); sau merge luôn chạy test, `brand:check`, `sync:contract`, `sync-audit.mjs` |
| `brand()` đổi nhầm định danh (ví dụ `Multica` đứng một mình trong mã) | Ngoại lệ trong `brand-rules.mjs` có test; `go build`/`tsc` và test bắt; chỉ áp cho dòng mới của upstream |
| Driver không được cài trên máy hoặc worktree mới | `.gitattributes` có trong repo; định nghĩa driver ở cấu hình chung của repo (`.git/config` dùng chung giữa worktree) do `install.sh` ghi; `sync:doctor` kiểm trước khi merge; thiếu driver thì git quay về merge thường (an toàn, chỉ nhiều conflict hơn) |
| `ours` làm mất thay đổi upstream ở file sinh tự động | Tái sinh bắt buộc ở `--finish`; `git diff` sau tái sinh so với upstream để thấy phần chỉ ở fork |
| Seam làm đổi hành vi | Mỗi seam một commit, chạy test vùng đó và `make test`/`pnpm test` nối tiếp; không gộp với tính năng mới |
| Công cụ đồng bộ phình thành dự án riêng | Mỗi phần có tiêu chí "xong" và ngưỡng đo; không làm phần nào không có số liệu chống lưng |

## 5. Chiến lược kiểm chứng

1. Test đơn vị cho từng driver và `brand-rules` (`scripts/sync/*.test.mjs`, chạy trong `pnpm test:scripts`; kiểm `.github/ci-paths.json` để nhóm `scripts` chạy chúng).
2. Test tích hợp trên repo git giả trong thư mục tạm (tạo base/fork/upstream, merge thật với driver) cho mỗi tình huống ở 3.1 và 3.2.
3. Mô phỏng lặp lại ba merge cũ làm cổng nghiệm thu: sau T1 phải đạt số ở bảng 2.1 (hoặc tốt hơn) và `--compare` không có khác biệt chưa giải thích.
4. `pnpm sync:contract` xanh trên `main` hiện tại trước khi dùng cho sync sau.
5. Kiểm hồi quy toàn bộ khi đụng mã sản phẩm (seam): `pnpm typecheck`, `pnpm lint`, `pnpm test`, `make test`; DB test chỉ dùng `multica_test`.

## 6. Triển khai và hoàn tác

- Làm trực tiếp trên `main`, commit nguyên tử theo quy ước (không worktree/nhánh dài, theo ghi nhớ đã có), một commit cho mỗi bước nhỏ. Không đẩy lên `origin` trước khi người dùng đồng ý.
- Công cụ mới không đổi hành vi sản phẩm (T0–T2) nên hoàn tác = xóa cấu hình driver và `.gitattributes`; merge quay về như cũ.
- Seam (T3) đổi mã sản phẩm: mỗi seam hoàn tác bằng `git revert` của commit riêng.
- Cập nhật tài liệu cùng thay đổi: `docs/fork-branding.md` (quy trình sync dùng driver và `brand:check`), `AGENTS.md` (quy tắc 3.9 cho mã fork mới; đường dẫn lệnh sync), bộ nhớ `upstream-sync-progress.md`.

## 7. Phương án đã loại

| Phương án | Vì sao loại |
|---|---|
| Đổi brand lúc chạy (loader `applyBrand`) thay cho sửa chữ trong file | Đo cho thấy locale conflict là do thêm/xóa key sát nhau, không phải do chữ Delegora (2.3). Đổi brand lúc chạy không xử lý được phần này, lại đụng vào mã sản phẩm và phải xử lý mobile, landing, docs riêng. Có thể xem lại sau khi ba driver đã chạy. |
| Patch-stack (rebase fork lên từng bản upstream) | 1.732 file sửa; mỗi lần rebase phải giải lại cả chồng patch |
| Upstream làm vendor/submodule + lớp phủ | Tái cấu trúc cả repo, rủi ro lớn, không tương thích cách `catalog/`, `packages/*` tham chiếu nhau |
| Hard fork (ngừng sync) | Mất mọi tính năng và bản vá bảo mật mới của upstream |
| `merge=union` của git cho các map đăng ký | Có thể ghép sai ngoặc hoặc trùng dòng; seam S1 an toàn hơn |

## 8. Quyết định cần người dùng xác nhận (mặc định đề xuất)

1. **Phạm vi brand trong mã (Go, comment, test):** hiện fork đã đổi Multica→Delegora ở cả comment và test fixture của Go (178 trong 413 file gần như chỉ khác brand là file `.go`, trong đó 85 là `_test.go`). Mặc định: **giữ nguyên chính sách hiện tại**, driver chỉ tự động hóa nó. Việc hoàn nguyên comment Go về Multica cho bề mặt nhỏ hơn là phương án phụ, quyết sau khi xem số liệu.
2. **Chu kỳ chạy thử định kỳ:** ĐÃ CHỐT, không có; chỉ chạy khi người dùng yêu cầu sync.
3. **Danh sách hợp đồng tính năng (3.7):** ĐÃ CHỐT, người dùng giao tôi dựng đầy đủ, không cần duyệt từng mục.
4. **Seam S1–S6:** mặc định làm theo thứ tự, mỗi mục xin xác nhận trước khi đụng mã sản phẩm.
5. **Migration (3.12):** chờ kết quả điều tra T4.1.

## 9. Lệnh đã dùng để đo (lặp lại được)

- Số merge, commit upstream: `git log --merges --first-parent`, `git rev-list --count <base>..<p2>`.
- Conflict: worktree tại `M^1`, `git -c rerere.enabled=false merge --no-commit --no-ff M^2`, `git diff --name-only --diff-filter=U`, `git merge --abort`.
- Có driver: thêm `-c core.attributesFile=<tệp attrs>` và `-c merge.<tên>.driver="node <script> %O %A %B %L %P"`.
- Bề mặt: `git diff --name-status --no-renames upstream/main main`, `git diff --numstat`, `git diff -U0 | grep -c '^@@'`.
- Chỉ-đổi-brand: với mỗi file `M`, lọc dòng đổi, thay `Delegora`→`Multica`, `sort | uniq -u`, đếm ≤2.
- Ổn định JSON: `JSON.stringify(JSON.parse(s), null, 2) + "\n" === s`.
- Migration: so tập số tiền tố của `server/migrations` ở `main` và `upstream/main`.

Giới hạn của phép đo: đếm **file conflict**, không đếm số hunk hay thời gian người giải. Nguyên mẫu driver chạy trong thư mục tạm, chưa phải mã trong repo.
