# Kế hoạch triển khai: engine workflow có thực thi cho squad Kit Việt

Đặc tả (thiết kế, quyết định, mã lỗi, API, tiêu chí chấp nhận): [kitviet-workflow-spec.md](kitviet-workflow-spec.md). Tài liệu này là các bước, có thứ tự, kiểm được. Cả hai là nguồn duy nhất; hai tài liệu cũ `kitviet-workflow-extend-analysis.md` và `squad-workflow-engine-plan.md` đã được gộp vào đây (chi tiết ở đầu đặc tả; tệp thứ nhất hiện có mặt lại trong cây làm việc, không phải nguồn).

Ngày 2026-11-12, đợt ba, đọc ở HEAD `4e8fc136c` (commit chỉ thêm hai tài liệu này; mã không đổi so với `ba189d733` của đợt hai; đợt một đọc `b324e4241`). **Chưa có mã nào được viết.** Đường dẫn là của HEAD; tệp "mới" chưa tồn tại, "đổi" đã tồn tại. Commit `9d3a9e081` đã chuyển nguồn template sang `catalog/` và bỏ mã sinh khỏi git: kế hoạch này theo bố cục đó (mục 0.3).

## 0. Cách đọc và quy ước

- **Hai tầng.** Tầng 1 (S0 đến S17): engine thực thi bất biến, lead gọi `advance`. Tầng 2 (S18 đến S22): runner nền gọi cùng `Advance`. **Cổng G0** (mục 0.1.1) chặn mọi bước cần DB tới khi người dùng xác nhận đã tự tạo database test theo D19 (cách test đã chốt, không còn để ngỏ). **Cổng G1** (S17) giữa hai tầng; **cổng G2** (S22) trước khi bật `flow_auto`. Lô squad (S23 đến S26) chỉ chạy sau G2.
- Mỗi bước là một commit nguyên tử, tiêu đề `feat(workflow): …`, `feat(catalog): …`, `feat(squads): …`, `test(workflow): …` hoặc `docs(squads): …`; mô tả PR theo mẫu (Problem, Goal, Proposal). PR có giao diện đính kèm ảnh chụp hoặc video.
- **Test viết trước** (đỏ, rồi xanh sau khi sửa). Test nằm cạnh mã. Test Go chạm DB dùng fixture `server/internal/testutil` (`dbfx.Issue`, `dbfx.Task`, `dbfx.Insert`) và `testutil.Call(h, req).Want(status).JSON(&out)`; **khẳng định trên dòng fixture của test**, không trên tổng của bảng (nhiều gói chạy song song trên một DB). Tệp `.test.ts` không cần DOM bắt đầu bằng `// @vitest-environment node`. Test views không mock `next/*`, `react-router-dom`.
- Test mặc định không được gọi agent thật hoặc CLI agent của người dùng. "Agent giả" = test gọi API với tư cách agent (token tác vụ giả) để đóng task.
- Trước mỗi bước: `git status --short -- <đường dẫn của bước>`; nếu tệp đã bị người khác sửa thì dừng và hỏi. Không chạm tệp của người khác (hiện có `server/internal/migrations/migrations_lint_test.go` sửa chưa commit).
- Nhãn kiểm: **[N]** = không cần DB, chạy được ngay. **[D]** = cần DB: chỉ chạy sau guard `scripts/require-test-database.sh` (S0) với `DATABASE_URL` tường minh trong lệnh (D19), và **bị chặn tới G0**. Mỗi bước ghi rõ nhãn ở dòng "Kiểm".

### 0.1 Lệnh chuẩn

| Mã | Lệnh | Ghi chú |
| --- | --- | --- |
| **[T]** | `pnpm typecheck`, `pnpm lint`, `pnpm test` (ở gốc repo, không gồm mobile) | Nhãn [N]. Turbo tự chạy `//#catalog:build` trước (`turbo.json`). Hẹp trước: `pnpm --filter @multica/core exec vitest run <mẫu>`, `pnpm --filter @multica/views exec vitest run <mẫu>`; chạy `vitest` trực tiếp thì phải chạy `node scripts/catalog/cli.mjs build` trước |
| **[C]** | `pnpm catalog:check` (= `node scripts/catalog/cli.mjs check`); `node --test scripts/catalog/*.test.mjs scripts/generate-kitviet-pack.test.mjs` | Nhãn [N]. `check` chỉ đọc tệp (chỉ `build` và `new` ghi; đã đọc mã). Dòng thứ hai là lệnh CI chạy (`.github/workflows/ci.yml:44`). Ở HEAD `check` in `catalog ok: 181 templates` kèm 3 cảnh báo về skill vendored |
| **[B]** | `node scripts/catalog/cli.mjs build` (hoặc `--target go`) | Ghi mã sinh vào `packages/core/catalog/generated/` và `server/internal/catalog/generated/*.json`; cả hai bị `.gitignore` (`.gitignore:43-46`). **Không `git add` chúng.** Cần trước `vitest`, `tsc`, `go test` chạy trực tiếp (`issueworkflow.Templates()` panic nếu chưa dựng) |
| **[Q]** | `make sqlc` (`Makefile:377`, chạy `sqlc@v1.31.1`) | Nhãn [N] (không DB). Chạy sau **mọi** sửa `server/pkg/db/queries/*.sql`; commit cả `server/pkg/db/generated/*` (tệp này được commit, khác mã catalog) |
| **[GN]** | `node scripts/catalog/cli.mjs build --target go`, rồi `cd server && go test -count=1 <gói> -run <mẫu>` cho gói **không** chạm DB: `./internal/issueworkflow/...`, `./internal/catalog/...`, `./cmd/multica`, và các test tĩnh `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'`, `./internal/migrations -run 'TestMigrationFilesHaveMatchingDirections'` | Nhãn [N]. Đã đọc: `internal/issueworkflow` và `internal/catalog` không import `testutil`, `pgxpool` hay đọc `DATABASE_URL`; `cmd/multica` `TestMain` chỉ cách ly thư mục home. **Luôn kèm `-run` khi gói có test DB** (`cmd/migrate`, `internal/migrations`) |
| **[D]** | Mọi test Go khác (`internal/handler`, `internal/service`, `internal/scheduler` phần DB, `cmd/migrate` phần DB, `internal/migrations` phần DB), và `go run ./cmd/migrate up` trên DB test | Dạng duy nhất: `DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd server && DATABASE_URL='<dsn-test>' go test -count=1 <gói> -run '<mẫu>' )` (mục 0.1.1). **Không** `make test`, `make check`, `make migrate-up`, `scripts/ensure-postgres.sh`, `go test` trần hay `go run ./cmd/migrate up` trần |
| **[E]** | `pnpm exec playwright test` | Có `e2e/workflow.spec.ts` (tab Workflow, trình thiết kế; đã tồn tại ở HEAD); không có spec cho squad. Không bước nào thêm spec. Spec đó cần máy chủ và DB (đăng nhập qua `TestApiClient`), nên **không chạy** trừ khi người dùng cho phép rõ; khi đó máy chủ e2e phải dùng DB test của D19 (DSN tường minh, qua guard), không dùng `.env` |

#### 0.1.1 D19 và cổng G0: test cần DB

**D19 (chọn bởi trợ lý, xác nhận trước tầng 2):**

1. Test DB chỉ chạy khi `DATABASE_URL` trỏ tới một database test riêng, tên **kết thúc `_test`** và **khác `multica`**.
2. Mọi lệnh cần DB đặt `DATABASE_URL` **tường minh trong chính lệnh** và không đọc `.env`: Go của `server/` không đọc tệp env (không có `godotenv`), nên chỉ cần không dùng `make` và không `source` tệp env.
3. Trước mỗi lệnh [D] chạy guard `scripts/require-test-database.sh` (tạo ở S0.1). Guard chỉ đọc biến môi trường, không mở kết nối.
4. **Không dùng** `make test`, `make check`, `make migrate-up`, `make dev`, `scripts/ensure-postgres.sh` cho việc này: chúng nạp tệp env (`Makefile:5`) và tạo DB nếu thiếu (`scripts/ensure-postgres.sh`).
5. **Tạo database test là việc của người dùng** (ví dụ `psql` hay công cụ DB của họ). Agent không chạy `CREATE DATABASE`, `createdb` hay script tạo DB. Người triển khai (hay agent) chỉ chạy `DATABASE_URL='<dsn-test>' go run ./cmd/migrate up` trên DB đó sau guard, và lại làm sau S1 khi có migration 985–987.

Lý do (đã đọc ở HEAD): `Makefile:358-362` (`test`) luôn gọi `ensure-postgres.sh` rồi `go run ./cmd/migrate up` trên DB của `ENV_FILE`, mà `Makefile:5` ưu tiên `.env` (trên máy rà soát là DB `multica` dùng chung); `handler_test.go:46-48`, `testutil/db_test.go:29-31`, `cmd/migrate/migrate_concurrent_test.go:75` và `cmd/migrate/main.go:933-936` rơi về `postgres://multica:multica@localhost:5432/multica` khi `DATABASE_URL` trống, và `TestMain` của `handler` ghi fixture vào DB đó; `testutil.IsolatedDB(t, dsn)` (`isolated_db.go:23`) tạo DB `multica_test_<uuid>` trên máy chủ của DSN truyền vào rồi xóa, nên vai trong DSN test cần quyền CREATEDB; đó là DB do test tạo và tự xóa, và người dùng chấp nhận quyền này khi xác nhận G0.

**Dạng lệnh chuẩn** (`<dsn-test>` = `postgres://<user>:<mật khẩu>@localhost:5432/<tên>_test?sslmode=disable`, do người dùng cấp, đặt trong lệnh):

```bash
DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh \
  && ( cd server && DATABASE_URL='<dsn-test>' go test -count=1 <gói> -run '<mẫu>' )

# migrate DB test (lần đầu, và sau S1)
DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh \
  && ( cd server && DATABASE_URL='<dsn-test>' go run ./cmd/migrate up )

# chạy rộng ở G1 và G2 (chưa chạy thử; test-go.sh không đọc env file và không migrate)
DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh \
  && DATABASE_URL='<dsn-test>' bash scripts/test-go.sh --race --only regular
```

**Cổng G0 (chỉ còn một việc):** người dùng xác nhận bằng văn bản (1) tên database test, kết thúc `_test`, (2) họ đã tự tạo nó, (3) vai trong DSN có quyền CREATEDB (cho `IsolatedDB`). Trước G0 các bước [D] chỉ viết test, PR ghi "chưa chạy", và S0 không được ghi "hành vi không đổi". Nếu người dùng không cấp DB test thì mọi bước [D] giữ trạng thái "chưa chạy" và kế hoạch dừng ở đó; không có đường phụ.

### 0.2 Không được làm

1. Không khóa ngoại, không `ON DELETE/UPDATE CASCADE`, không CHECK mới trên bảng `issue`.
2. Không tạo chỉ mục không `CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`; mỗi chỉ mục một tệp một câu lệnh; chỉ mục mới phải có dòng trong `concurrentIndexCleanups` (`server/cmd/migrate/main.go:142`).
3. Không sửa `server/pkg/db/queries/*.sql` mà không chạy [Q] và commit mã sinh của sqlc.
4. Không gọi agent thật, không tạo request thật, không ghi vào DB `multica` hay bất kỳ DB nào không kết thúc `_test`; không chạy `make test`, `make check`, `make migrate-up`, `scripts/ensure-postgres.sh` hay script môi trường dev cho việc test (D19); không chạy lệnh tạo database. Rehearsal chạy trên DB test của D19.
5. **Không commit mã sinh từ catalog** (`packages/core/catalog/generated/**`, `server/internal/catalog/generated/*`); không thêm chúng bằng `git add -f`; không thêm bản sao dữ liệu flow vào TS hay Go. Dữ liệu flow chỉ ở manifest `catalog/workflows/flow_*`.
6. Không đặt dữ liệu flow vào trường `squad.workflow` của manifest squad hay vào `designer_*` (spec mục 8); không sửa 21 manifest `catalog/squads/kv_*/template.json` để thêm flow (các bài kiểm `packages/core/catalog/kitviet/*.test.ts` khẳng định `workflow` là null).
7. Không đổi hằng số `reworkReturnLimit = 3` (`server/internal/handler/issue_workflow_entry.go:157`) hay hành vi workflow không có `flow_config`; test vàng S0 phải xanh sau mọi bước (khi có DB).
8. Không đặt mã phát sự kiện trạng thái ở `package service` (cần `handler.IssueResponse`, spec mục 2 dòng 16).
9. Không sửa tệp ngoài phạm vi của bước, kể cả `server/internal/migrations/migrations_lint_test.go` (bản sửa chưa commit của người khác), và không hoàn tác thay đổi của người khác.
10. Không chạy `git commit/push/stash/reset/checkout` ngoài quy trình PR của người triển khai.
11. Không mở rộng sang Research Lab, Market Research, Content Studio, Sales Loop, SDLC, BMAD Planning trong đợt này (mục 5).
12. Không thêm bản sao hoặc sửa tay workflow flow trong giao diện.

### 0.3 Quy tắc catalog cho mọi bước chạm template hay dữ liệu workflow

1. **Nguồn ở `catalog/`, mã sinh bởi build.** Manifest flow là `catalog/workflows/flow_<id squad>/template.json` (spec mục 8, D18). Kế hoạch không sửa `packages/core/squads/templates.ts` để thêm dữ liệu flow.
2. Bước nào sửa `catalog/` phải kết thúc bằng [C] xanh. Khi đổi nội dung một manifest, tăng `version` của nó (`catalog/README.md`); id không bao giờ đổi.
3. Trước `vitest`, `tsc` hay `go test` chạy trực tiếp, chạy [B]. Mã sinh không vào commit (A20).
4. Mẫu mà mã tìm theo tên khai `contracts`. Manifest flow **không** khai (không mã nào tìm flow theo tên); các hợp đồng `squad.sdlc`, `workflow.sdlc_auto`, `squad.planning`, `workflow.planning_auto`, `company.executive_office` phải vẫn được đáp ứng: `go test ./internal/catalog/...` (TestRequiredContractsProvided) và `packages/core/catalog/contracts.test.ts` chạy trong [GN] và [T].

## 1. Tầng 1: kiểm (engine thực thi bất biến)

### S0. Guard DB, preflight, đường chạy test và test vàng

- **Mục tiêu:** (1) guard tên DB chạy trước mọi lệnh [D] (D19); (2) khóa hành vi workflow hiện có; (3) xác nhận số migration và catalog xanh ở HEAD. Thứ tự: S0.1, S0.2, S0.3.
- **Phụ thuộc:** không (S0.3 cần G0).

**S0.1 Guard tên database (làm đầu tiên, trước mọi lệnh [D])**

- **Tệp:** `scripts/require-test-database.sh` (mới), `scripts/require-test-database.test.sh` (mới; theo kiểu `scripts/worktree-db.test.sh`; chưa đăng ký vào CI trong đợt này).
- **Quy tắc của guard:** đọc `DATABASE_URL` từ môi trường tiến trình; không có giá trị mặc định, không đọc tệp, không mở kết nối. Thoát 1 và in lý do cùng tên đọc được khi: biến trống hay chưa đặt; không phải URL `postgres://` hay `postgresql://` (dạng `key=value` bị từ chối); phần truy vấn có `dbname=` hoặc `database=` (các tham số ghi đè tên DB mà `testutil/isolated_db_test.go:11-17` dùng); tên DB (sau dấu `/` cuối, trước `?`) trống, chứa ký tự ngoài `[A-Za-z0-9_]`, bằng `multica` (không phân biệt hoa thường), hoặc không khớp `^.+_test$`. Hợp lệ thì in `test database: <tên>` và thoát 0.
- **Test trước (A21):** các ca: biến trống; `.../multica`; `.../multica?sslmode=disable`; `.../MULTICA`; `.../multica_test_old`; `.../_test`; `.../app_test` hợp lệ; `postgresql://…/app_test?sslmode=disable` hợp lệ; `host=… dbname=app_test` bị từ chối; `…/app_test?dbname=multica` bị từ chối; tên có ký tự lạ bị từ chối; guard không đọc `.env` dù có tệp `.env` trong thư mục hiện tại (chạy trong thư mục tạm có `.env` giả); guard không gọi `psql`, `docker` hay `pg_isready` (đặt stub báo lỗi vào `PATH`).
- **Kiểm:** `bash scripts/require-test-database.test.sh` [N].
- Guard là bước kiểm đọc, không thay cho G0 và không tạo DB.

**S0.2 Preflight (không sửa tệp, không chạm Postgres)**

- `git rev-parse --short HEAD` (ghi vào PR; kế hoạch viết ở `4e8fc136c`).
- `git status --short -- server/migrations catalog packages/core/catalog scripts/catalog` phải rỗng; `git ls-tree --name-only HEAD server/migrations/ | tail -2` phải kết thúc ở `984_…` (nếu đã có 985 trở lên thì đổi số theo thứ tự và giữ tiền tố duy nhất).
- **`pnpm catalog:check` phải in `catalog ok` ở HEAD trước mọi thay đổi** (HEAD: `catalog ok: 181 templates`). Nếu đỏ thì dừng và hỏi. Chạy thêm `node --test scripts/catalog/*.test.mjs scripts/generate-kitviet-pack.test.mjs`.
- Ghi vào PR rằng `server/internal/migrations` `TestMigrationNumericPrefixesAreUnique` **đỏ sẵn** ở HEAD (16 tiền tố trùng 600–608, 610–616; bản sửa chưa commit là của người khác) và không thuộc lỗi của kế hoạch.
- Không chạy `scripts/ensure-postgres.sh`, `make test` hay `make migrate-up`.

**S0.3 Test vàng**

- **Tệp:** `server/internal/handler/issue_workflow_golden_test.go` (mới).
- **Test trước:** `TestWorkflowGolden_SDLCAutoWrites` (handler, DB): dựng squad SDLC, dự án, workflow SDLC Auto; chuỗi ghi: tạo issue `todo` → `in_progress` → `in_review` (outcome `rejected`, ba lần liên tiếp). Khẳng định từng bước trạng thái, người nhận, mã lỗi (`status_not_in_workflow`, `workflow_step_skipped`, `workflow_outcome_mismatch`, `workflow_step_moved`) và lần trả về thứ ba ép `blocked` giao cho lead. `TestWorkflowGolden_PlanningAutoWrites` tương tự cho Planning. Chúng **phải xanh trên HEAD trước khi sửa mã**.
- **Kiểm:** S0.1 và [C] ngay; test vàng: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'Golden'` (mục 0.1.1; chờ G0).

### S1. Migration 985–987, đăng ký chỉ mục, truy vấn

- **Mục tiêu:** có cột và chỉ mục (spec mục 3.1); truy vấn sqlc.
- **Phụ thuộc:** S0.1 và S0.2, G0 cho test DB.
- **Tệp:** `server/migrations/985_issue_flow_columns.{up,down}.sql`, `986_issue_flow_task_once_idx.{up,down}.sql`, `987_issue_flow_open_idx.{up,down}.sql` (mới); `server/cmd/migrate/main.go` (đổi: hai dòng vào `concurrentIndexCleanups`, `:142`); `server/pkg/db/queries/flow.sql` (mới: `LockIssueForFlow`, `SetIssueFlow`, `ClearIssueFlow`, `SetIssueOutcome`, `ListFlowTasks`, `ListOpenFlowRequests`, `CountOpenFlowRequestsByWorkflow`, `SetIssueWorkflowFlowAuto`, `IsFlowRequestAuto`); `server/pkg/db/queries/issue.sql` (đổi: `CreateIssue` và `CreateIssueWithOrigin` nhận `flow_step`, `flow_task`, `flow_round`, `flow` tùy chọn); `server/pkg/db/queries/issue_workflow.sql` (đổi: Create/Update mang `flow_config`, `template_ref`; `flow_auto` chỉ ghi qua truy vấn riêng).
- **Test trước:** `TestEveryConcurrentUpBuildHasCleanup` (đã có, `server/cmd/migrate/migrate_mul5999_index_retry_test.go:163`, test tĩnh) chạy ngay sau khi thêm hai tệp migration và **phải đỏ** vì thiếu dòng đăng ký, rồi xanh sau khi thêm dòng. `TestMigrationFilesHaveMatchingDirections` (đã có, test tĩnh) xanh với ba cặp up/down. `TestFlowMigrations_UpDown` (mới, `server/cmd/migrate/migrate_flow_columns_test.go`, dùng `testutil.IsolatedDB`): up tạo đúng tám cột kiểu đã nêu và hai chỉ mục hợp lệ (`pg_index.indisvalid`), down xóa sạch, chạy up hai lần không lỗi.
- **Kiểm:** [Q]; `cd server && go build ./...`; [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'` và `./internal/migrations -run 'TestMigrationFilesHaveMatchingDirections'` (nhãn [N]); `TestFlowMigrations_UpDown`: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./cmd/migrate -run 'TestFlowMigrations_UpDown'` (mục 0.1.1; chờ G0); sau S1, migrate lại DB test bằng dạng lệnh migrate ở mục 0.1.1.

### S2. Mô hình bước, kiểm tra hợp lệ, hàm quyết định thuần

- **Mục tiêu:** `Task`, `Step.Tasks`, `Step.HumanOnly`, `FlowConfig`, `ValidateFlow`, `Decide` (spec mục 3.3 đến 3.7 và 4.1).
- **Phụ thuộc:** S0.
- **Tệp:** `server/internal/issueworkflow/flow.go` (mới), `issueworkflow.go` (đổi: `Step`, `storedStep`, `EncodeSteps`/`DecodeSteps`, `Definition.HasFlow`), `flow_test.go` (mới), `issueworkflow_test.go` (đổi).
- **Test trước:** `TestDecodeStepsLegacyUnchanged` (hàng cũ không có `tasks` giải mã và mã hóa lại y hệt); bảng `TestValidateFlow` (khóa task trùng; gate thiếu `outcome`; chu trình không có `rework` hữu hạn; `rework` trỏ về bước sau; hai gate cùng agent; công tắc chưa khai báo; vượt 12 bước/8 task/8 công tắc; bước plan có `tasks`; `HumanOnly` kèm task agent); bảng `TestDecide` (mỗi tổ hợp `tasks_outcome` × `rework_count` ∈ {0,1,2} → đích, cờ `rework`, `stop`; gate đóng không outcome = `rejected`; bước `human_only` không tự chuyển).
- **Kiểm:** [GN] `./internal/issueworkflow/...` (nhãn [N]).

### S3. Điều kiện và dữ kiện

- **Mục tiêu:** `Facts`, trường `tasks_outcome`, `switch.<key>`, cờ `rework` trên `Transition` (spec mục 3.5).
- **Phụ thuộc:** S2.
- **Tệp:** `server/internal/issueworkflow/transitions.go` (đổi), `transitions_test.go` (đổi).
- **Test trước:** `TestEvaluate_TasksOutcome`, `TestEvaluate_SwitchBoolEnum`, `TestValidateCondition_SwitchKey` (regexp, trường không khai báo bị từ chối), `TestLegacyTransitionsUnchanged` (mọi ca cũ của `LegacyTransitions` giữ nguyên), `TestRouteFirstMatchWins` (thứ tự điều kiện).
- **Kiểm:** [GN] `./internal/issueworkflow/...` (nhãn [N]).

### S4. Tương thích API workflow, kiểu và client phía web

- **Mục tiêu:** payload/input mang trường mới; **ghi giữ trường khi client cũ không gửi**; khóa workflow do template quản lý; chặn xóa; trường mới ở `SquadResponse`; kiểu, zod và **hàm client mới cho thư viện workflow của squad** (chưa có ở HEAD, spec mục 2 dòng 10).
- **Phụ thuộc:** S1, S2.
- **Tệp:** `server/internal/handler/issue_workflow.go` (đổi: `IssueWorkflowStepPayload` `:68`, `IssueWorkflowResponse` `:79`, `IssueWorkflowStepInput` `:513`, `resolveStepInputs` `:558`, blockers `:1103`); `server/internal/handler/squad.go` (đổi: `SquadResponse` `:26`); `server/pkg/db/queries/squad.sql` (đổi: join workflow đang dùng, đếm thư viện) – [Q]; `packages/core/types/issue-workflow.ts`, `packages/core/types/squad.ts` (đổi: `Squad` thêm `active_workflow_id`, `active_workflow_name`, `workflow_count`, `flow_enforced`); `packages/core/api/schemas.ts`, `packages/core/api/client.ts` (đổi: **thêm** `listSquadWorkflows`, `setSquadActiveWorkflow`; mỗi phản hồi qua zod và `parseWithFallback`); `packages/core/api/schemas.test.ts` (đổi).
- **Test trước (Go, [D]):** `TestUpdateIssueWorkflow_OldClientKeepsTasks` (PATCH không gửi `tasks`, `human_only`, `flow_config` → bản lưu giữ nguyên, tiêu chí A13); `TestUpdateIssueWorkflow_TemplateManagedIs409` (`workflow_managed_by_template`, A1); `TestCreateIssueWorkflow_FlowConfigNeedsAdmin` (403 `flow_workflow_forbidden`); `TestDeleteIssueWorkflow_BlockedByOpenFlowRequests` (`workflow_has_flow_requests`); `TestSquadResponse_WorkflowFields` (bốn trường mới, squad không workflow cho `workflow_count = 0`, `flow_enforced = false`).
- **Test trước (TS, `// @vitest-environment node`, [N]):** malformed – thiếu `flow`, `tasks: null`, `stop` lạ → `"stalled"`, `kind` lạ → `"work"`, outcome lạ giữ chuỗi, `workflow_count` kiểu chuỗi, squad thiếu bốn trường (A15); `listSquadWorkflows` với `active_workflow_id: null`, `workflows` thiếu hay `total` sai kiểu không làm vỡ.
- **Kiểm:** [Q]; [B] rồi `pnpm --filter @multica/core exec vitest run api`; [T] ([N]); Go handler: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'Workflow|SquadResponse'` (mục 0.1.1; chờ G0).

### S5. Lưu verdict và luật đóng task

- **Mục tiêu:** `issue.outcome` được lưu; R1, R2, R3 phần outcome (spec mục 4.2); phủ đường batch.
- **Phụ thuộc:** S1, S2.
- **Tệp:** `server/internal/handler/issue.go` (đổi: `UpdateIssue` `:4218-4225`, batch `:5140`, `IssueResponse`), `server/internal/handler/issue_workflow_entry.go` (đổi: điểm gọi), `server/internal/handler/flow_guards.go` (mới), `packages/core/types/issue.ts`/schema (đổi: `outcome`, `flow`).
- **Test trước ([D]):** `TestFlowTaskClose_GateWithoutOutcome422` (A2, trạng thái giữ nguyên), `TestFlowTaskClose_DoneWithBlockedOutcome422` (`flow_outcome_invalid`), `TestFlowTaskClose_AttachmentRequired422` (A3), `TestFlowTaskClose_AttachmentOnCommentCounts` (tệp gắn vào comment của task vẫn thỏa R2 vì mang `issue_id`; tệp do agent khác hay thành viên tải lên thì không thỏa), `TestFlowTaskClose_BatchPathCovered` (cùng quy tắc qua `:5140`), `TestOutcomeStoredAndReturned`, `TestPlainIssueUnaffected` (issue không có `flow_task` đóng như cũ).
- **Kiểm:** Go: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'FlowTaskClose|Outcome'` (mục 0.1.1; chờ G0); phần TS: [T] ([N]).

### S6. Lõi engine: EnsureFlowStarted, Plan, Advance, Resume, Detach

- **Mục tiêu:** `FlowEngine` ba pha (spec mục 5.3), khởi tạo request (spec mục 4.4), comment dừng, đồng bộ khi thành viên sửa tay, mở task bù khi thiếu.
- **Phụ thuộc:** S1 đến S5.
- **Tệp:** `server/internal/handler/flow_engine.go` (mới), `flow_engine_test.go` (mới); `server/internal/service/issue.go` (đổi: `IssueCreateParams` `:64` nhận `FlowStep/FlowTask/FlowRound`; các câu `qtx.CreateIssue` và `CreateIssueWithOrigin` ở `:390,414` truyền ba cột); `server/internal/handler/issue.go` (đổi: gọi `EnsureFlowStarted` sau `CreateIssue` ở `:3627`); `server/internal/service/squad_handoff.go` (đổi: cùng lời gọi quanh `:473`).
- **Test trước ([D], agent giả):** `TestAdvance_OpensParallelTasks` (A8), `TestAdvance_BarrierWaitsForAllTasks`, `TestAdvance_OptionalTaskSkippedAndOpenedBySwitch` (A9), `TestAdvance_ReworkTwiceThenStop3` (A10: 6 sub-issue gate, `rework = 2`, `stop = 3`), `TestAdvance_BlockedOutcomeStop1` (A11, comment có tiêu đề dừng và nhắc người yêu cầu), `TestAdvance_Idempotent` (A7), `TestAdvance_CompletesPartialCreation` (xóa một dòng task rồi `Advance`), `TestAdvance_ConcurrentCallsCreateOneSet` (8 goroutine, B2; bắt `23505` của `idx_issue_flow_task_once` và coi là đã có), `TestAdvance_CreateUsesAllowDuplicate` (hai task cùng tiêu đề chuẩn hóa ở hai vòng khác nhau không bị `ErrActiveDuplicate`), `TestAdvance_ConcurrentWritersLoseOnFlowCondition` (điều kiện `SetIssueFlow` trên `flow->>'step'`), `TestAdvance_AgentUnavailableBlocks`, `TestAdvance_ManualMoveSyncs` (B8), `TestEnsureFlowStarted_Idempotent`, `TestEnsureFlowStarted_FromFlowShow`, `TestEnsureFlowStarted_AfterHTTPCreate`, `TestEnsureFlowStarted_SkipsChildAndNonSquadIssues`, `TestEnsureFlowStarted_WithoutProject` (định nghĩa theo `flow.workflow_id`, request không dự án), `TestResume_Stop3ResetsRework`, `TestResume_Stop1ReopensOnlyBlockedTasks`, `TestDetach_RestoresPlainIssues`.
- **Kiểm:** [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'Advance|EnsureFlowStarted|Resume|Detach'` (mục 0.1.1; chờ G0).

### S7. Chốt kiểm và loại trừ khỏi định tuyến thường

- **Mục tiêu:** R3, R4, R5, R6, R7, R8; `activeWorkflowOf` bỏ qua workflow flow (spec mục 9.1).
- **Phụ thuộc:** S6.
- **Tệp:** `server/internal/handler/issue_workflow_entry.go` (đổi: `applyIssueWorkflow`), `server/internal/handler/issue.go` (đổi: `CreateIssue` `:3315`, `UpdateIssue`, batch), `server/internal/issueworkflow/issueworkflow.go` (đổi: `activeWorkflowOf` `:662`, `ForIssue` giữ chữ ký), `flow_guards.go` (đổi).
- **Test trước ([D] trừ dòng cuối):** `TestGuard_HumanOnlyStep403` (A4), `TestGuard_AgentWriteToFlowRequest409` đơn lẻ và batch (A5), `TestGuard_AgentCreateChildOfFlowRequest409` (A6), `TestGuard_GateSameAgent422`, `TestGuard_ChildTasksExemptFromProjectWorkflow`, `TestGuard_PlainSquadIssueKeepsProjectWorkflow` (A14 một nửa), `TestGoldenStillGreen` (chạy lại S0). Bảng `TestActiveWorkflowOfSkipsFlow` ở `issueworkflow` là [N].
- **Kiểm:** [GN] `./internal/issueworkflow/...` ([N]); Go handler: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'Guard|Golden'` (mục 0.1.1; chờ G0).

### S8. API HTTP, CLI, sự kiện, truy vấn client

- **Mục tiêu:** endpoint mục 7.1 của spec; lệnh `multica issue flow …`; sự kiện `issue_flow:updated`; truy vấn/mutation phía client.
- **Phụ thuộc:** S6, S7.
- **Tệp:** `server/cmd/server/router.go` (đổi, cạnh `/workflows`), `server/internal/handler/flow_api.go` (mới), `server/pkg/protocol/events.go` (đổi, cạnh `:148`), `server/cmd/multica/cmd_issue_flow.go` (mới), `cmd_issue.go` (đổi: trợ giúp `--outcome` `:728`); `packages/core/types/issue-flow.ts` (mới), `types/events.ts` (đổi), `packages/core/issue-workflows/` (đổi: `issueFlowOptions`, mutation `plan/advance/resume/detach`), tests. **Không** sửa `scripts/agent-cli-command-names.txt` (đó là tên tệp thực thi CLI agent, không phải lệnh con của `multica`).
- **Test trước:** `TestFlowAPI_Permissions` (bảng: lead agent, thành viên thường, quản lý squad, người ngoài; mỗi endpoint một ca cho qua và một ca bị từ chối với đúng mã) [D], `TestFlowAPI_StateShape` [D], `TestFlowAPI_NotManaged404` [D], `TestFlowAPI_SwitchLocked409` [D], CLI `TestIssueFlowHelp` (golden) [N]; TS: mutation chờ máy chủ rồi vô hiệu hóa (không cập nhật lạc quan), sự kiện vô hiệu hóa khóa `issueFlowKeys` [N].
- **Kiểm:** [GN] `./cmd/multica -run IssueFlow`; [B] rồi `pnpm --filter @multica/core exec vitest run issue-workflows`; [T] ([N]); `FlowAPI`: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'FlowAPI'` (mục 0.1.1; chờ G0).

### S9. Lời nhắc cho task và cho lead

- **Mục tiêu:** `FlowBrief` cho task (việc, đầu vào dạng liên kết tệp đính kèm, lệnh kết thúc kèm `--outcome`, điều máy chủ sẽ từ chối) và cho lead ở bước plan; gắn dù issue không có dự án.
- **Phụ thuộc:** S6.
- **Tệp:** `server/internal/issueworkflow/flow.go` (đổi: `FlowBrief`), `server/internal/handler/issue_workflow_entry.go` (đổi: `claimProjectWorkflow` `:562` có nhánh flow), tests.
- **Test trước:** golden `TestFlowBrief_GateTask` (chứa lệnh `multica issue status <id> done --outcome approved|rejected`, `blocked --outcome blocked`, và không chứa comment của người soạn), `TestFlowBrief_Rework` (nêu mã task gate bị `rejected`), `TestFlowBrief_LeadPlan` (chứa hướng dẫn bước plan) – các test này ở `issueworkflow` là [N]; `TestClaimFlowBrief_WithoutProject`, `TestClaimBrief_NonFlowUnchanged` ở handler là [D].
- **Kiểm:** [GN] `./internal/issueworkflow/...`; handler: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'ClaimFlowBrief|ClaimBrief'` (mục 0.1.1; chờ G0).

### S10. Workflow đang dùng của squad và gán dự án (D11)

- **Mục tiêu:** workflow flow làm workflow đang dùng không dời trạng thái issue thường; squad có flow gán dự án được; hành vi 422 cho squad chưa có workflow giữ nguyên.
- **Phụ thuộc:** S7.
- **Tệp:** `server/internal/handler/squad_workflow.go` (đổi: `squadWorkflowPlan` `:304` trả rỗng và bỏ `remapSquadIssues` `:355` khi workflow có `flow_config`), `squad_project.go` (không đổi mã nếu test xanh; điều kiện ở `:108`), tests.
- **Test trước ([D]):** `TestSetSquadActiveWorkflow_FlowWorkflowDoesNotRemap`, `TestAssignSquadProject_FlowSquadReturns200` (A14), `TestAssignSquadProject_NoWorkflowNonSDLCStill422`, `TestFlowSquadInProject_RequestUsesFlowOthersUseProject`.
- **Kiểm:** [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'SquadActiveWorkflow|AssignSquadProject|FlowSquadInProject'` (mục 0.1.1; chờ G0).

### S11. Catalog: khối `flow` cho workflow (công cụ, chưa có dữ liệu thật)

- **Mục tiêu:** catalog học loại workflow thứ ba (khối `flow`), kiểm tra nó, và sinh `packages/core/catalog/generated/flows.ts` và `server/internal/catalog/generated/flows.json` (không commit). Chưa thêm manifest flow thật (S16); sau S11 `pnpm catalog:check` vẫn in `catalog ok: 181 templates`.
- **Phụ thuộc:** S2 (Go `ValidateFlow`), D18 (đã chốt: manifest `catalog/workflows/flow_<id squad>`; phương án `catalog/_sources/kitviet/flows/` đã bị loại, spec mục 8). D18 xác nhận trước tầng 2; nếu bị bác thì làm lại S11, S12, S16, S23–S26.
- **Tệp:** `scripts/catalog/lib/build-workflows.mjs` (đổi: nhận khối `flow`, `validateFlow`, `renderFlows`, tính `rev`; hiện `validateWorkflow` báo `workflow block is missing` cho dạng lạ); `scripts/catalog/lib/validate.mjs` (đổi: tham chiếu L2 `flow.squad`, vai thuộc `squad.roles`, mỗi squad tối đa một flow); `scripts/catalog/lib/build.mjs` (đổi: ghi `flows.ts` và `flows.json`); `packages/core/catalog/types.ts` (đổi: `CatalogFlow`); `packages/core/catalog/flows.ts` (mới: tra flow theo id squad, đọc `generated/flows`); `server/internal/catalog/catalog.go` (đổi: `FlowsJSON()`); `scripts/catalog/catalog.test.mjs` (đổi); `server/internal/issueworkflow/flow_catalog_test.go` (mới).
- **Test trước:** trong `scripts/catalog/catalog.test.mjs`, nhóm `flow workflow validation` (dựng catalog tạm như các nhóm có sẵn): nhận flow đầy đủ; từ chối `flow.squad` lạ; vai không thuộc squad; vai lead làm task; vai (trừ lead) không dùng; hai manifest cho một squad; khối `flow` kèm `workflow` hoặc `designer`; chu trình không `rework` hữu hạn; gate thiếu `requires outcome`; công tắc chưa khai báo; `rev` ổn định khi đổi thứ tự khóa JSON (A18); `squad.workflow` trỏ tới manifest flow vẫn bị từ chối bởi kiểm tra có sẵn. Go: `TestCatalogFlowsPassValidateFlow` chạy `ValidateFlow` trên mọi flow của `flows.json` (A19; trống thì xanh).
- **Kiểm:** [C]; [B]; [GN] `./internal/issueworkflow/... ./internal/catalog/...`; [T] ([N]); `git ls-files packages/core/catalog/generated server/internal/catalog/generated` chỉ trả `.gitkeep` (A20).

### S12. Quickstart và client: tạo và đặt workflow flow

- **Mục tiêu:** quickstart tạo workflow flow từ `CATALOG_FLOWS`, đặt làm workflow đang dùng của squad; nhận biết trùng theo `template_ref`.
- **Phụ thuộc:** S4, S8, S11.
- **Tệp:** `packages/core/squads/quickstart.ts` (đổi: sau khối `:878-904`; đổi `role` thành `handler.id` bằng cách tra agent theo vai như `workflowSteps` `:237`), `packages/core/squads/quickstart-kitviet.test.ts` và `quickstart.test.ts` (đổi). Không sửa `templates.ts`, `catalog/squads/kv_*/template.json`.
- **Test trước ([N]):** client giả: với squad có flow, đúng 1 lần `createIssueWorkflow` (có `flow_config`, `template_ref`) rồi 1 lần `setSquadActiveWorkflow`; chạy lại → `existing` theo `template_ref`, không tạo trùng; không phải admin → lỗi `status_create_forbidden` dịch thành thông báo; squad không có flow, `sdlc_greenfield` và `bmad_planning` → hành vi hiện tại không đổi (0 lần gọi mới); các test `packages/core/catalog/kitviet/*.test.ts` (`workflow` null) vẫn xanh.
- **Kiểm:** [B]; `pnpm --filter @multica/core exec vitest run squads/quickstart catalog`; [C]; [T] ([N]).

### S13. Văn bản lead và agent, quy tắc rerun (D3), câu tự giao việc (D9)

- **Mục tiêu:** lead hai chế độ (lệnh `multica issue flow show` đầu tiên; 404 `flow_not_managed` thì theo quy trình cũ); thống nhất "chạy lại mọi gate"; bỏ 15 câu tự giao việc.
- **Phụ thuộc:** S8.
- **Tệp:** `catalog/_sources/kitviet/skills/kv-squad-lead/SKILL.md` (đổi; bước 6 ở `:24`), `packages/core/squads/kitviet-agent-instructions.ts` (đổi: `KV_LEAD_NOTES` `:211`, câu "reruns only the gate" ở `:327,486,500`; văn bản lead theo loại vai), `packages/core/squads/kitviet-previous-texts.ts` (đổi: **thêm** văn bản lead và squad trước đây; tệp này do `scripts/kitviet-update.mjs` ghi khi nâng upstream, nên thêm tay theo mẫu có sẵn và để `kitviet-previous-texts.test.ts` kiểm), **`catalog/_sources/kitviet/rewrites.json`** (đổi, D20: thêm quy tắc đặt **trước** `call-agent` ở `:182`, khớp cả hai dạng upstream "gọi/giao `kv:lap-ke-hoach-dieu-phoi`" và "gọi agent `kv:lap-ke-hoach-dieu-phoi`"; lời thay thế phải đọc được với các đuôi đã quan sát: " lập kế hoạch", " lập kế hoạch rồi người dùng duyệt", " lập kế hoạch thay vì tự viết", " lập comment Kế hoạch của lead trên issue gốc", "."), `docs/engineering/kitviet-squads.md:167`; các test `packages/core/squads/kitviet-skills.test.ts`, `kitviet-stage-status.test.ts`, `kitviet-stop-header.test.ts`, `kitviet-lead-self-mention.test.ts`, `kitviet-previous-texts.test.ts`, `scripts/generate-kitviet-pack.test.mjs`. **Không** sửa `catalog/_sources/kitviet/patches/*.json` hay skill rời cho việc này: `rewrites.json` là nguồn duy nhất và sinh lại được (thứ tự áp dụng: bản vá, rồi quy tắc theo thứ tự mảng, rồi giải tham chiếu, `generate-kitviet-pack.mjs:335-337`).
- **Test trước ([N]):** `no text says "reruns only the gate that blocked"` (3 ghi chú); đếm cụm `giao sub-issue cho vai \`<x>_lead\`` trong đầu ra sinh ra của **cả 21 squad bằng 0** (hiện 15 squad có đúng một cụm: accounting, b2b, banking, construction, ecom, edu, factory, health, hospitality, insurance, legal, marketing, realestate, talent, trade; đếm bằng `node scripts/generate-kitviet-pack.mjs --out <thư mục tạm>`, đợt một sót `b2b`); quy tắc mới khớp ít nhất một chỗ (generator báo "matched nothing (stale rule)" nếu không, `:883`) và không làm đổi `academic`, `admin`, `pr` (chỉ nêu lead ở cột Agent); `lead text starts with flow show` và chứa nhánh cũ; tiêu đề dừng không đổi; registry nhận văn bản lead cũ (nút cập nhật thay được, bản người dùng sửa được giữ).
- **Kiểm:** [B]; `pnpm --filter @multica/core exec vitest run squads catalog`; [C]; `node --test scripts/generate-kitviet-pack.test.mjs`.

### S14. Nâng cấp

- **Mục tiêu:** spec mục 8: tìm squad theo `template_key`; so `rev` qua `template_ref`; tạo hàng mới và đổi workflow đang dùng; không đụng request đang chạy.
- **Phụ thuộc:** S12.
- **Tệp:** `packages/core/squads/upgrade.ts` (đổi: `:284`, `upgradeKitvietSquad` `:238`), `upgrade.test.ts` (đổi), `packages/core/squads/mutations.ts` (đổi nếu cần).
- **Test trước ([N]):** squad đổi tên vẫn được tìm theo `template_key`; cùng `rev` → `current`, 0 lần ghi; `rev` mới → 1 `createIssueWorkflow` + 1 `setSquadActiveWorkflow`, **không** `updateIssueWorkflow` nào; workflow đang dùng không do template quản lý → `kept`; chạy hai lần không đổi gì.
- **Kiểm:** [B]; `pnpm --filter @multica/core exec vitest run squads/upgrade`; [T] ([N]).

### S15. Giao diện

- **Mục tiêu:** spec mục 9.2 (cột, mục, sơ đồ, bảng Flow của request, nút admin) và 7 locale.
- **Phụ thuộc:** S4, S8.
- **Tệp:** `packages/core/squads/stores/view-store.ts` (đổi: `SquadColumnKey` `:36`), `view-store.test.ts`; `packages/views/squads/components/squads-page.tsx` (đổi, cột quanh `:527-535`), `squad-workflows-cell.tsx` (mới), `squad-workflows-section.tsx` (mới), `workflow-graph-sheet.tsx` (mới), `squad-detail-page.tsx` (đổi, mục giữa `:708` và `:714`); `packages/views/workflow/issue-flow-state-panel.tsx` (mới), `issue-flow-card.tsx` (đổi), `packages/views/issues/components/issue-detail.tsx` (đổi `:4213`); `packages/views/workflow/manage/*`, `designer/*` (đổi: ẩn sửa/nhân bản/xóa cho `template_ref`); `packages/views/locales/{en,zh-Hans,zh-Hant,fr,es,ko,ja}/{squads,issues,workflow}.json` (đổi); các `.test.tsx`.
- **Test trước ([N]):** cột hiện đúng tên workflow và huy hiệu, ẩn được qua menu cột; mục Workflows: danh sách, trạng thái rỗng, lỗi có thử lại, hàng không giải mã hiện "Không đọc được"; sheet xếp bước theo thứ tự, task cùng bước cạnh nhau, `when` bằng lời, khóa chỉ người; nút quản trị chỉ khi `canManage`; bảng Flow: task và outcome, Resume/Detach chỉ cho quản lý, xác nhận Detach chờ máy chủ; tab Workflow không còn nút sửa cho workflow template; `parity.test.ts`.
- **Kiểm:** `pnpm --filter @multica/views exec vitest run squads workflow issues locales`; [T] ([N]); kiểm tay web và desktop (màn hẹp), chụp ảnh. `e2e/workflow.spec.ts` có thể bị chạm (nó mở danh sách và trình thiết kế): chỉ chạy nếu người dùng cho phép rõ và máy chủ e2e dùng DB test của D19, nếu không PR ghi "chưa chạy [E]".
- **Mobile:** không đổi; chỉ kiểm bằng đọc (`apps/mobile`) rằng trạng thái lạ và trường mới không làm vỡ; nếu vỡ thì ghi vào theo dõi, không sửa trong bước này.

### S16. Thí điểm Trade: manifest flow và diễn tập kịch bản (tầng 1)

- **Mục tiêu:** viết manifest flow của Trade; chạy Trade đầu-cuối trên engine với **agent giả**, trên DB test của D19; không gọi agent thật.
- **Phụ thuộc:** S6 đến S13, S11 (khối `flow`), G0.
- **Tệp:** `catalog/workflows/flow_kv_trade/template.json` (mới; `kind: workflow`, `ui: false`, `flow.squad = kv_trade`, nội dung theo spec mục 3.6 và 12.2; `role` dùng khóa vai như `trade_docs_reader`); `server/internal/handler/flow_rehearsal_trade_test.go` (mới; nạp flow từ `catalog.FlowsJSON()` (dữ liệu sinh), tạo agent fixture theo vai, **không dựng tay** định nghĩa).
- **Test trước:** `TestRehearsal_Trade_HappyPath` (A16: Plan → Collect 1 hoặc 2 task → Draft 1 → Gate 2 → Handover 1 → `in_review`; 0 sub-issue do agent tạo; thành viên chuyển `done` thì `closed_at` có giá trị), `_ReworkTwice` (A10), `_GateWithoutVerdictRejected` (A2), `_Stop1MissingInvoice` (A11), `_CustomsLookupSwitch` (A9), `_AgentCannotLeaveInReview` (A4), `_ExportOnlyAfterStop2`, `_ThirdRejectionStop3ThenResume`, `_DetachRestoresPlain`.
- **Kiểm:** [C] (manifest hợp lệ, `catalog ok: 182 templates`); [B]; test rehearsal: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'Rehearsal_Trade'` (mục 0.1.1; chờ G0).

### S17. Cổng G1 (tầng 1 → tầng 2)

- **Việc:** chạy toàn bộ phần [N]: [T], [C], [GN]; `go vet ./...` trong `server`; phần [D] bằng dạng lệnh chuẩn của mục 0.1.1 (guard S0, `DATABASE_URL` tường minh, kể cả lệnh chạy rộng `scripts/test-go.sh --race --only regular`; không `make test`); ghi biên bản `docs/engineering/kitviet-flow-rehearsal.md` (mới): lệnh đã chạy, kết quả A1–A21, chênh lệch so với đặc tả, và những bước [D] chưa chạy nếu G0 chưa xong.
- **Điều kiện qua cổng (đo được):** A1–A21 đều xanh (A1–A17 cần [D]); S0 vàng không đổi; 0 sub-issue trùng trong mọi rehearsal; người dùng **xác nhận bằng văn bản** D7, D11, D15, D17, D18, D19, D20 (đánh dấu "chọn bởi trợ lý, xác nhận trước tầng 2" ở spec mục 10). Diễn tập agent thật (một request Trade trên workspace thử, đủ đầu vào / thiếu đầu vào / bản nháp sai cố ý) **chỉ khi người dùng cho phép rõ**; kết quả ghi cùng biên bản.
- **Không qua cổng thì không bắt đầu S18.**

## 2. Tầng 2: runner nền

### S18. Móc vào luồng sự kiện sub-issue

- **Mục tiêu:** spec mục 5.2 điểm 1, 3, 4.
- **Phụ thuộc:** G1.
- **Tệp:** `server/internal/handler/issue_child_event.go` (đổi: `Handler.processChildEvents` `:14-15` gọi `advanceFlowRequests` sau `ProcessChildEvents`; **không** thêm phụ thuộc vào `service.IssueWakeupService`), `server/internal/service/issue_wakeup_system.go` (đổi: `dispatchSystem` `:411` bỏ qua `child_done` khi `IsFlowRequestAuto`), `server/internal/handler/daemon.go` (đổi cạnh `:5060`), tests.
- **Test trước ([D]):** `TestChildEvents_AdvancesAutoRequest`, `TestChildDone_NotDispatchedForAutoRequest` (B6), `TestChildDone_StillFiresForManualFlow`, `TestChildDone_ResumesWhenFlagOff` (tắt `FF_FLOW_RUNNER` hoặc `flow_auto` thì `child_done` bắn lại), `TestChildEvents_FailedAdvanceLeavesSweepToRetry`.
- **Kiểm:** [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler ./internal/service -run 'ChildEvents|ChildDone'` (mục 0.1.1; chờ G0).

### S19. Job quét, thời hạn, công tắc

- **Mục tiêu:** spec mục 5.2 điểm 2, 5.6, 5.7; quét cả request đủ điều kiện chưa có `flow` (`EnsureFlowStarted`).
- **Phụ thuộc:** S18.
- **Tệp:** `server/internal/scheduler/jobs_flow.go` (mới), `server/cmd/server/main.go` (đổi: đăng ký cạnh `:860`), `server/internal/featureflags/keys.go` (đổi: `FlowEngine` mặc định bật nơi gọi, `FlowRunner` mặc định tắt; hàm `…Enabled(ctx, flags)` như các khóa hiện có), `server/internal/handler/flow_engine.go` (đổi: thời hạn, thử lại, `stalled`; xếp lượt chạy bằng `TaskService.EnqueueTaskForIssue` cho task mở không có lượt chạy), `server/internal/handler/flow_api.go` (đổi: `PUT /api/issue-workflows/{id}/flow-auto`), tests. Ngữ nghĩa cờ đã đọc (spec mục 2 dòng 21): mặc định do nơi gọi truyền, `FF_FLOW_RUNNER=off` tắt.
- **Test trước:** `TestFlowSweepJobSpec` (cadence, timeout, lease: [N]); [D]: `TestFlowSweep_AdvancesMissedEvent`, `TestFlowSweep_StartsUnstartedEligibleRequest`, `TestFlowSweep_IdleTaskRetriesThenStalls` (B4), `TestFlowSweep_BlockedAgentTaskGetsRunWhenReady`, `TestFlowSweep_HardCapCancelsRun`, `TestFlowSweep_LeadNoPlanWakesThenStalls`, `TestFlowRunner_FlagOffIsNoop` (B5), `TestFlowRunner_RequiresWorkflowAuto`, `TestFlowSweep_Handles200InBudget` (B7), `TestFlowAuto_AdminOnly`.
- **Kiểm:** [GN] `./internal/scheduler -run FlowSweepJobSpec` ([N]); phần DB: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/scheduler ./internal/handler -run 'FlowSweep|FlowRunner|FlowAuto'` (mục 0.1.1; chờ G0).

### S20. Chịu lỗi: sập, khởi động lại, đồng thời

- **Mục tiêu:** chứng minh bất biến của spec mục 5.3 và bảng lỗi spec mục 6.
- **Phụ thuộc:** S19.
- **Tệp:** `server/internal/handler/flow_chaos_test.go` (mới); `flow_engine.go` (đổi: ghi log có cấu trúc `request_id`, `step`, `action`; không thêm hệ đếm mới).
- **Test trước ([D]):** `TestChaos_CrashBetweenPhase1And2` (B3), `TestChaos_RestartHandlerMidStep`, `TestChaos_SweepAndEventConcurrent`, `TestChaos_HumanMovesRequestDuringRun`, `TestChaos_TaskCancelledByMember`, `TestChaos_RunFailureBlocksTask` (đường `service/task.go:6489`), `TestChaos_ArchivedAgent`.
- **Kiểm:** [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'Chaos'` (mục 0.1.1; chờ G0).

### S21. Diễn tập Trade ở tầng 2 (kịch bản)

- **Phụ thuộc:** S20.
- **Tệp:** `server/internal/handler/flow_rehearsal_trade_auto_test.go` (mới).
- **Test trước ([D]):** `TestRehearsalAuto_Trade_HappyPath` (lead chỉ gọi `plan`; mọi chuyển bước do runner; B1, B6), `_NoDuplicatesAfterRestart`, `_StalledTaskBlocksRequest` (B4), `_KillSwitchFallsBackToManual` (B5).
- **Kiểm:** [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'RehearsalAuto'` (mục 0.1.1; chờ G0).

### S22. Cổng G2

- **Việc:** [T], [C], [GN], phần [D] bằng dạng lệnh chuẩn của mục 0.1.1; ghi kết quả B1–B8 vào `docs/engineering/kitviet-flow-rehearsal.md`.
- **Điều kiện:** B1–B8 xanh; người dùng xác nhận bật `flow_auto` cho Trade **trên workspace thử**. Bật trên môi trường thật là hành động riêng của người dùng, không thuộc kế hoạch này.

## 3. Lô squad (sau G2; mỗi lô một cổng riêng)

Mỗi lô gồm cho từng squad: viết manifest `catalog/workflows/flow_<id squad>/template.json` (mới, `version` `1.0.0`, `ui: false`) theo spec mục 12.2, đối chiếu từng khâu với skill `day-chuyen-*` (nguồn `catalog/_sources/kitviet/upstream/<nghề>/skills/day-chuyen-*/SKILL.md`, kèm bản vá trong `catalog/_sources/kitviet/patches/` nếu có, chỉ để đọc) và `KV_LEAD_NOTES.<id>` (`packages/core/squads/kitviet-agent-instructions.ts`), ghi nguồn trong mô tả PR; chạy [C] và [B]; viết kịch bản `TestRehearsal_<Squad>_*` (tối thiểu: đường thành công mỗi route, gate bị chặn rồi qua, dừng 1, dừng 3), chạy bằng dạng lệnh chuẩn: [D] guard S0, rồi `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'Rehearsal_<Squad>'` (trong `server`); cập nhật mục squad ở `docs/engineering/kitviet-runbook.md` §8 (dòng "Flow"); [T]. **Cổng lô:** [C] xanh, kịch bản xanh cho mọi squad của lô, người dùng xác nhận. Lô 4 còn cần người trong ngành đọc đầu ra theo runbook §8d (`docs/engineering/kitviet-runbook.md:681`) trước khi bật `flow_auto`.

| Bước | Lô | Squad (số route) | Manifest mới (`catalog/workflows/<id>/template.json`) |
| --- | --- | --- | --- |
| S16 | 0 | Trade (1) | `flow_kv_trade` |
| S23 | 1 | B2B (1), Admin (1) | `flow_kv_b2b`, `flow_kv_admin` |
| S24 | 2 | Marketing (1), PR (2), E-commerce (3, gồm `order_report`), Academic (2), Legal (2, một routine), Accounting (3, một routine), Real estate (2) | `flow_kv_marketing`, `flow_kv_pr`, `flow_kv_ecom`, `flow_kv_academic`, `flow_kv_legal`, `flow_kv_accounting`, `flow_kv_realestate` |
| S25 | 3 | Talent (1), Education (1), Teaching (2), Hospitality (2), Office Ops (1; một task `drafter` cho mọi việc) | `flow_kv_talent`, `flow_kv_education`, `flow_kv_teaching`, `flow_kv_hospitality`, `flow_kv_officeops` |
| S26 | 4 | Health (1), Construction (1), F&B (1), Insurance (2), Banking (1), Factory (2) | `flow_kv_health`, `flow_kv_construction`, `flow_kv_fnb`, `flow_kv_insurance`, `flow_kv_banking`, `flow_kv_factory` |

Tổng 21 manifest (S16 một, S23 đến S26 hai mươi). Id manifest là `flow_` cộng id template squad (đã đọc từ `catalog/squads/kv_*/template.json`: `kv_education`, `kv_ecom`, `kv_officeops`, `kv_realestate`); mỗi manifest có `flow.squad` bằng id đó, khóa vai trong `tasks[].role` và không sửa `catalog/squads/kv_*/template.json`. Sau mỗi lô `pnpm catalog:check` báo thêm đúng số template của lô (182 sau S16, rồi 184, 191, 196, 202 sau S23 đến S26).

Test dữ liệu chung (S23 thêm một lần, [N]): `packages/core/catalog/flows.test.ts` – mỗi template Kit Việt đã vào lô có đúng một flow (tra theo `flow.squad`); số giá trị của công tắc `route` khớp bảng mục 12.2; mọi vai (trừ lead) được dùng; giới hạn `rework` là 2; mọi gate `requires outcome`. Test này đọc `packages/core/catalog/generated/flows` nên cần [B] trước.

Routine: `validity_review` (Legal) và `tax_calendar` (Accounting) khởi động request có `route` định sẵn ở plan; routine vẫn tạo ở trạng thái tạm dừng (`startPaused`), không đổi (`catalog/squads/kv_legal/template.json:62`, `kv_accounting/template.json:68`).

## 4. Tài liệu, hoàn tác

### S27. Runbook, tài liệu, diễn tập hoàn tác

- **Tệp:** `docs/engineering/kitviet-runbook.md` (đổi: mục Flow – công tắc, `detach`, `resume`, đọc `IssueFlowState`, dựng trước chỉ mục theo `server/cmd/migrate/README.md`, đường test DB theo D19: DB `_test` + guard + `DATABASE_URL` tường minh, không `make test`), `docs/engineering/kitviet-squads.md` (đổi: con trỏ tới spec; sửa dòng `:167` theo D3), `docs/engineering/kitviet-sample-workspace.md` (đổi: ghi chú squad có workflow), `docs/engineering/kitviet-flow-rehearsal.md` (hoàn thiện); `server/internal/handler/flow_rollback_test.go` (mới).
- **Test trước ([D]):** `TestRollback_FlagOffTurnsFlowWorkflowIntoPlain`, `TestRollback_DetachAllThenMigrationDown` (trên DB cô lập: detach hết, chạy down 987/986/985, up lại), `TestRollback_LeadTextFallsBackWhenNotManaged`.
- **Kiểm:** `git diff --check` cho tài liệu ([N]); test rollback: [D] guard S0, rồi trong `server`: `DATABASE_URL='<dsn-test>' go test -count=1 ./internal/handler -run 'Rollback'` (mục 0.1.1; chờ G0).

### 4.1 Hoàn tác theo tầng

| Tình huống | Cách |
| --- | --- |
| Tắt runner (tầng 2) | `FF_FLOW_RUNNER=off` hoặc `PUT /api/issue-workflows/{id}/flow-auto {"auto": false}`. Request giữ nguyên bước; `child_done` hoạt động lại ngay cho request (tính tại chỗ, spec mục 5.2 điểm 4); lead gọi `flow advance` thủ công |
| Tắt engine (tầng 1) | `FF_FLOW_ENGINE=off`: guard và API flow trả 404/bỏ qua; workflow flow thành workflow thường không định tuyến issue; request flow không còn được quản |
| Xóa trạng thái đang chạy | `POST /api/issues/{id}/flow/detach {"cancel_tasks": false}` cho từng request (CLI `multica issue flow detach <id>`); task thành sub-issue thường; lead dùng chế độ cũ vì văn bản lead hai chế độ |
| Hoàn tác migration | Sau khi detach hết: chạy down theo thứ tự `987`, `986`, `985` (`DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd server && DATABASE_URL='<dsn-test>' go run ./cmd/migrate down )`; trên môi trường thật của người dùng thì do người dùng tự chạy với DSN của họ, không phải việc của kế hoạch này). `985` down xóa tám cột và mất dữ liệu `flow`, `outcome`; `986`/`987` down là `DROP INDEX CONCURRENTLY IF EXISTS` |
| Quay lại văn bản agent cũ | Không cần: lead hai chế độ; nếu vẫn cần thì dùng sổ bản cũ `kitviet-previous-texts.ts` qua nút cập nhật Kit Việt |
| Workflow template sai | Sửa manifest `catalog/workflows/flow_<squad>`, tạo bản `rev` mới (quickstart hoặc nút cập nhật), đặt làm workflow đang dùng; bản sai chỉ xóa được khi `open_flow_requests = 0` |
| Gỡ khối `flow` khỏi catalog | Chỉ khi không còn workflow `template_ref` `flow_*` nào được dùng; id manifest không đổi hay tái dùng (`catalog/README.md`) |

## 5. Theo dõi (ngoài đợt này)

| Hạng mục | Vì sao chưa làm | Điều kiện bắt đầu |
| --- | --- | --- |
| Research Lab | Cần loại task `check` (gọi `litscout.State.Gate`, `server/internal/litscout/state.go:375`), giới hạn vòng lấy từ dữ liệu, verdict ba giá trị; lead hiện tự theo bảng `catalog/skills/builtin/research-lab-squad-lead/SKILL.md:54-68` | Sau lô 2, có quyết định D-RL về cách đưa tệp trạng thái tới máy chủ |
| Market Research | Khởi động từ routine, critic, giới hạn 3 proposal; manifest `catalog/squads/market_research/template.json` có 4 vai và 1 routine | Sau lô 2; dùng `flow/start` |
| Content Studio | Cần bước đợi quyết định proposal (10 vai, `catalog/squads/content_studio`) | Sau lô 2 và có hạng mục proposal-wait |
| Sales Loop | Cần `foreach` theo lane, nhiều route (`catalog/squads/sales_loop`, 3 routine) | Sau `foreach` |
| `foreach` (cũng cho Office Ops từng việc) | Chưa cần ở đợt đầu | Khi một squad cần số task động |
| SDLC, BMAD Planning | Đã chạy trên engine mức issue việc (`sdlc_greenfield` → `sdlc_auto`, `bmad_planning` → `planning_auto`); đổi mức request có nguy cơ hồi quy; các hợp đồng catalog ràng buộc tên | Khi có nhu cầu; tham số hóa giới hạn vòng (mặc định 3) |
| Trình thiết kế sửa `tasks`, bản sao do người dùng | Cần luật hợp nhất khi nâng cấp ở mức cả workflow | Sau khi đợt đầu ổn định |
| Mobile | Mobile độc lập (`apps/mobile/AGENTS.md:7`), không có màn hình quản lý squad | Khi có yêu cầu |
| Dọn tự động workflow phiên bản cũ | Admin xóa tay khi `open_flow_requests = 0` | Khi số phiên bản tích lũy gây phiền |
| Chỉ số và cảnh báo cho runner | Chưa xác minh package metrics phù hợp | Sau G2 |
| Máy chủ dựng workflow từ catalog (như `catalogWorkflowSteps` cho SDLC, `server/internal/service/catalog_workflows.go`) thay vì client | Mẫu có sẵn cho `sdlc_auto`; chọn client ở đợt đầu vì quickstart và nâng cấp Kit Việt đã ở TS | Khi cần tạo workflow flow mà không qua web |

## 6. Thứ tự và tóm tắt

28 bước (S0 đến S27), ba cổng chính (G0 trước mọi bước [D]: người dùng xác nhận đã tạo DB test; G1 ở S17; G2 ở S22) và bốn cổng lô (S23 đến S26). Đường găng: S0 → S1 → S2 → S3 → S5 → S6 → S7 → S8 → S9 → S11 → S12 → S16 → S17(G1) → S18 → S19 → S20 → S21 → S22(G2) → lô.

Song song được: S4, S10 với S5 (sau S2 và S1); S11 với S6 trở đi (cần S2); S13, S14, S15 sau S8 và S12; hoàn thiện tài liệu S27 đi cùng.

Phần làm được ngay không cần DB (nhãn [N]): S2, S3, phần TS của S4 và S8, S11, S12, S13, S14, S15, phần tĩnh của S1.

Việc người dùng phải làm hoặc xác nhận: **trước mọi bước [D]** G0 (tự tạo database test kết thúc `_test` theo D19 và xác nhận tên cùng quyền CREATEDB); **trước tầng 2 (G1)** D7 (`order_report`, `fnb_regulation`, `pr_numbers_gate`), D11 (gán dự án và luật "làn"), D15 (Trade), D17 (thứ tự lô), cùng D18 (flow là manifest catalog), D19 (đường test DB) và D20 (sửa 15 câu ở `rewrites.json`), ba mục này đã được chọn và chỉ chờ xác nhận. Phần làm được ngay không cần DB: xem dòng trên.
