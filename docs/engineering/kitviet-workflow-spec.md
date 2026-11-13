# Engine workflow có thực thi cho squad (đợt đầu: 21 squad Kit Việt): đặc tả

Trạng thái: **đặc tả thiết kế đã chốt quyết định, chưa có mã.** Kế hoạch từng bước: [kitviet-workflow-plan.md](kitviet-workflow-plan.md). Hai tài liệu này là nguồn duy nhất cho việc triển khai.

Ngày rà soát: 2026-11-12, đợt ba (đợt hai đọc `ba189d733`). Đọc mã ở **HEAD `4e8fc136c`** (`git show HEAD:<path>`), nhánh `main`, trùng `origin/main`; đợt một đọc `b324e4241`. Commit `4e8fc136c` chỉ thêm hai tài liệu này (`git diff --name-status ba189d733 HEAD`), nên mọi trích dẫn mã không đổi so với đợt hai. Ba commit sau `b324e4241` là `9d3a9e081` (chuyển nguồn template sang `catalog/`, sinh mã lúc build), `90dff73f9` và `ba189d733` (sửa đường dẫn trong tài liệu). Chúng **không** đổi `server/migrations` và không đổi các tệp handler, scheduler, `issueworkflow.go`, `transitions.go` mà tài liệu này trích dẫn (`git diff --name-status b324e4241 HEAD` chỉ cho thấy test handler, `issueworkflow/templates.go`, `service/initiative_build.go` và các tệp `catalog` mới trong `server/`). Chúng đổi phía template, script, Makefile, turbo, CI: xem mục 2 dòng 26 đến 31 và mục 14. Mọi trích dẫn dạng `tệp:dòng` trong hai tài liệu đã được đối chiếu với HEAD bằng script (tệp có thật, dòng nằm trong tệp) và dòng đầu của từng khoảng được đọc lại bằng mắt; chỗ lệch đã sửa và ghi ở mục 14. Mã sinh từ `catalog/` (`packages/core/catalog/generated/`, `server/internal/catalog/generated/*.json`) **không còn được commit**, nên tài liệu chỉ trích nguồn trong `catalog/`. Cây làm việc có thêm thay đổi chưa commit của người khác (`server/internal/migrations/migrations_lint_test.go`, `server/internal/service/knowledge_test.go`); không đụng tới.

Ngôn ngữ: tiếng Việt. Tên tệp, kiểu, lệnh, mã lỗi giữ tiếng Anh. Chú thích trong mã viết tiếng Anh.

Tài liệu thay thế (là tệp chưa commit do chính tôi viết ở đợt một, nội dung đã gộp vào đây): `squad-workflow-engine-plan.md` (đã xóa) và `kitviet-workflow-extend-analysis.md` (đã xóa ở đợt một; lúc rà soát đợt hai tệp này **có mặt lại** trong cây làm việc, chưa commit, giờ sửa 22:35 ngày 2026-11-12, đọc `b324e4241`, và vẫn chưa commit ở đợt ba; đợt hai và ba không tạo lại nó và không được sửa hay xóa tệp ngoài hai tài liệu này, nên người dùng xóa tay hoặc giữ; nếu nó khác tài liệu này thì tài liệu này đúng). Mô hình bảng `squad_flow` của các bản trước **bị bỏ**.

## 1. Vấn đề, mục tiêu, không mục tiêu

**Vấn đề.** Quy trình của squad nằm trong prompt và skill, agent có thể không tuân thủ. Bằng chứng trong repo:

- Diễn tập Kit Việt: gate không có dòng `VERDICT` và bản nháp không đính kèm tệp (`docs/engineering/kitviet-drill-report.md:28-29`, 4 trong 20 lần chạy B hỏng, dòng 46).
- 15 skill dây chuyền sinh ra còn câu bảo lead giao việc lập kế hoạch cho chính vai lead ("lead giao sub-issue cho vai `<squad>_lead`"): accounting, b2b, banking, construction, ecom, edu, factory, health, hospitality, insurance, legal, marketing, realestate, talent, trade. Câu do quy tắc viết lại `call-kv-agent` (`catalog/_sources/kitviet/rewrites.json:188`) sinh ra khi tác nhân upstream `kv:lap-ke-hoach-dieu-phoi` được ánh xạ sang vai lead (`catalog/_sources/kitviet/squads/<id>.json`, mục `agents`). Đếm bằng cách chạy generator ra thư mục tạm (`node scripts/generate-kitviet-pack.mjs --out <thư mục tạm>`, không ghi vào repo) rồi đếm đúng cụm này trong từng tệp `<squad>.ts`. Đợt một ghi 14 vì sót `b2b`; ba squad `academic`, `admin`, `pr` chỉ nêu vai lead ở cột Agent của bảng dây chuyền, không có câu giao việc, nên không tính. Cách sửa đã chọn (D20): thêm quy tắc vào `rewrites.json`, không sửa từng skill (S13).
- Quy tắc chạy lại gate mâu thuẫn: `kv-squad-lead` bước 6 chạy lại mọi gate (`catalog/_sources/kitviet/skills/kv-squad-lead/SKILL.md:24`); ghi chú lead của officeops, education, teaching (`packages/core/squads/kitviet-agent-instructions.ts:327,486,500`), `docs/engineering/kitviet-squads.md:167` và upstream (`day-chuyen-chien-dich-marketing` bước 6) chạy lại chỉ gate bị chặn.
- Research Lab: mã tính quyết định gate (`server/internal/litscout/state.go:375`) nhưng lead phải nhớ gọi lệnh (`catalog/skills/builtin/research-lab-squad-lead/SKILL.md:54-68`, mục 3 "Follow the gate").

**Mục tiêu.** Nền tảng, không phải prompt, quyết định: thứ tự khâu, ai làm, song song và rào chắn, gate độc lập, verdict có cấu trúc, số vòng sửa, điểm dừng người. Một cơ chế dùng chung cho mọi squad có quy trình; đợt đầu áp dụng cho 21 squad Kit Việt, thí điểm Trade.

**Engine không bảo đảm được** (ghi rõ để không hứa quá): nội dung bản nháp đúng hay sai; gate phán đoán đúng không; lead chọn route và công tắc đúng không (chỉ chọn trong thực đơn đã định nghĩa và được ghi lại); gate không đọc comment của người soạn (engine chỉ đưa liên kết tới tệp đính kèm, không chặn đọc).

**Không mục tiêu (đợt đầu).** Research Lab, Market Research, Content Studio, Sales Loop (mục 12.3); trình thiết kế sửa `tasks`; bản sao hoặc workflow flow do người dùng tự viết; giao diện mobile; pipeline initiative của công ty.

**Không đổi.** Workflow SDLC Auto và BMAD Planning Auto (`squad.workflow` khác `null` ở `catalog/squads/sdlc_greenfield/template.json:16` = `sdlc_auto` và `catalog/squads/bmad_planning/template.json:16` = `planning_auto`; `sdlc_maintenance` là `null`; kiểu `SquadTemplate.workflow` ở `packages/core/squads/templates.ts:168`), các hợp đồng catalog `squad.sdlc`, `workflow.sdlc_auto`, `squad.planning`, `workflow.planning_auto` (`server/internal/catalog/contracts.go`), workflow dự án, Default, hằng số `reworkReturnLimit = 3` (`server/internal/handler/issue_workflow_entry.go:157`), lời nhắc workflow của issue trong dự án. Mọi trường mới là tùy chọn; hành vi của workflow không có `flow_config` giữ nguyên, được khóa bằng test vàng (S0).

## 2. Hiện trạng engine (đã đọc ở HEAD)

| # | Thành phần | Vị trí | Điều cần biết |
| --- | --- | --- | --- |
| 1 | `Handler`, `Step`, `Definition` | `server/internal/issueworkflow/issueworkflow.go:161,169,187` | Một bước = `status_key` + `Handler` + `Instructions` + `Transitions` + `Layout`. Tối đa 50 bước (`:48`); mỗi `status_key` một lần (`:483`); không có trường cho task, tùy chọn, bước chỉ người |
| 2 | `validateHandler` | `issueworkflow.go:564` | Chỉ `none`, `project_lead`, `agent`, `member`; `squad` và `creator` bị từ chối |
| 3 | `Condition`, `Transition`, `Facts`, `Route`, `Evaluate`, `IsOutcome` | `server/internal/issueworkflow/transitions.go:55,65,76,96,183,87` | `Route` trả chuyển tiếp khớp đầu tiên; điều kiện có trường `outcome`, `priority`, `labels`, `project_id`, `rework_count`, `creator_type`, `assignee_type`; tối đa 10 chuyển tiếp (`:48`); outcome gồm `approved, rejected, blocked, done` |
| 4 | `ForIssue`, `routingSquad`, `activeWorkflowOf` | `issueworkflow.go:610,640,662` | `ForIssue` trả `nil` khi issue không có `project_id` (`:611-613`); workflow đang dùng của squad áp cho issue giao cho squad hoặc cho agent có `home_squad_id` là squad (`:640-658`) |
| 5 | `CheckStatus` | `issueworkflow.go:838` | Ghi trạng thái ngoài danh sách bị từ chối (400 `status_not_in_workflow`) |
| 6 | Đường ghi trạng thái | `server/internal/handler/issue.go`: `UpdateIssue` `:3982`; `Outcome` `:3786`; đọc outcome `:4218-4225`; gọi `applyIssueWorkflow` `:4225`; **đường batch** `:5140` (outcome rỗng); `updateIssueWithStatusGuard` `:284`; `runWithIssueStatusGuard` `:262` | Outcome chỉ là tham số tạm của một lần ghi, không lưu |
| 7 | `applyIssueWorkflow` | `server/internal/handler/issue_workflow_entry.go:54-150` | Kiểm trạng thái; kiểm `outcome` chỉ khi agent tự truyền (`:86`); chặn `done` từ cột giữa (`:100-104`, `stepSkippedError`); ép `blocked` ở vòng trả về (`:129`); request của lead không bị giao lại trừ khi chính lượt chạy của lead đổi (`:141`); giao lại người nhận khi vào cột có người nhận (`:148`) |
| 8 | Vòng sửa | `issue_workflow_entry.go:157` (`reworkReturnLimit = 3`), `:171-200` (`reworkLoopFor`), `:263` (`checkOutcomeRoute`), `:280` (`issueRouteFacts`); `server/pkg/db/queries/activity.sql:70-82` (`CountWorkflowReturns`) | Chỉ cho `in_review`/`qa` về `in_progress` khi cột `blocked` có người nhận là agent. Chuyển tiếp chỉ được kiểm, chưa được thực thi |
| 9 | Ghi workflow | `server/internal/handler/issue_workflow.go`: `resolveStepInputs` `:558-592`; `stepRouting` `:156`; tạo trạng thái theo tên, dùng lại nếu có `:594-633`; `status_create_forbidden` `:730,945`; chặn xóa `:1103,1112`; sự kiện `:1188` | `resolveStepInputs` dựng `Step` chỉ từ trường đã biết, nên trường mới bị mất nếu client cũ ghi |
| 10 | Thư viện và workflow đang dùng của squad | `server/internal/handler/squad_workflow.go`: `ListSquadWorkflows` `:42`; `CreateSquadWorkflow` `:74`; `SetSquadActiveWorkflow` `:172`; `adoptSquadWorkflow` `:287`; `squadWorkflowPlan` `:304`; `remapSquadIssues` `:355`; route `server/cmd/server/router.go:2469-2471` (`GET`, `POST /workflows`, `POST /workflow` dưới `/api/squads/{id}`) | Đặt workflow đang dùng dời trạng thái mọi issue của squad trong dự án; migration `973`, `974`. `adoptSquadWorkflow` đưa workflow cấp workspace vào thư viện của squad (`SetIssueWorkflowSquad`) rồi đặt làm đang dùng; `CreateSquadWorkflow` chỉ **sao chép** từ `source_workflow_id` nên không mang cột mới. Client web (`packages/core/api/client.ts`) và kiểu `Squad` (`packages/core/types/squad.ts:11`) **chưa có** hàm hay trường nào cho thư viện hay workflow đang dùng của squad (không có kết quả cho `active_workflow`, `listSquadWorkflows` ở HEAD): đó là việc mới (S4, S12) |
| 11 | Gán dự án | `server/internal/handler/squad_project.go:108` | **Điều kiện thật:** khối `if !activeSquad.ActiveWorkflowID.Valid` mới gọi `EnsureSquadProjectWorkflow` (`:109`) và mới sinh 422 `build_squad_not_sdlc` (`:112`, lỗi `ErrBuildNeedsSDLCSquad` ở `server/internal/service/initiative_build.go:57,140-156`). Squad **đã có** workflow đang dùng thì khối bị bỏ qua, không có 422 |
| 12 | Hàng đợi chạy | `server/internal/handler/issue_queue.go:218` (`PromoteQueuedIssues`), `:239` (`promoteQueuedIssue`); `server/cmd/server/issue_queue_listeners.go:19`; `server/internal/scheduler/jobs_issue_queue.go:18`; đăng ký `server/cmd/server/main.go:733,860`; truy vấn `server/pkg/db/queries/issue.sql:806-860` | Mẫu "chỉ đọc sự thật đã lưu, nghe sự kiện, quét 30 giây" |
| 13 | Sự kiện sub-issue bền | `server/migrations/558_issue_child_event.up.sql` (trigger ghi `issue_child_event` cho **mọi** đường ghi); `server/internal/handler/issue_child_event.go:14-15` (`Handler.processChildEvents`); `server/internal/service/issue_wakeup_system.go:115,183,280`; job `server/internal/scheduler/jobs_child_done.go:19`, đăng ký `main.go:857` | Sự kiện được đánh dấu xong **trước** khi gọi lead (`:214` định nghĩa `finish`, `:256` gọi); lần quét sau chỉ xử lý hàng chưa xử lý. `Handler.processChildEvents` được gọi ngay sau ghi ở `issue.go:4421`, `issue.go:5258`, `issue_queue.go:304`, `pr_auto_complete.go:305`; đường ghi hệ thống (`service/task.go:6489`, `runtime_sweeper.go:726`) chỉ để trigger ghi sự kiện rồi chờ job quét. `service.IssueWakeupService` được dựng theo giá trị `&IssueWakeupService{Tasks: ...}` ở 15 nơi (`main.go:854,857`, `daemon.go:2882,4292`, `issue_child_event.go:15`, `issue_system_wakeup.go:231,312`, `issue_wakeup.go` ×7, `qoder_issue_lifecycle.go:83`), không nhận phụ thuộc nào khác |
| 14 | Lease của scheduler | `server/internal/scheduler/spec.go:102-165` | `JobSpec`: `Cadence`, `RunTimeout`, `StaleTimeout`, `HeartbeatInterval`, `AllowStaleReentry`, `MaxAttempts`, `Scopes` |
| 15 | Chạy lại khi lượt chạy kết thúc mà issue chưa dịch chuyển | `server/internal/handler/issue_unmoved_run.go:26`; gọi ở `server/internal/handler/daemon.go:5060` | Chỉ cho issue `in_progress` có người nhận là handler của cột; bỏ qua request của squad (`isSquadRequest`, `issue_workflow_entry.go:663`) |
| 16 | Ghi nhật ký trạng thái | `server/cmd/server/activity_listeners.go:55-111` | Hàng `activity_log` chỉ sinh khi có sự kiện `issue:updated` mà payload `issue` là `handler.IssueResponse` (`:60-62`). Vì vậy **mã phát sự kiện trạng thái phải nằm ở package `handler`**, không ở `service` |
| 17 | Tạo issue từ máy chủ | `server/internal/service/issue.go:246` (`IssueService.Create`), tham số `:64` (`IssueCreateParams`: `Stage`, `Metadata`, `ParentIssueID`, `AssigneeType/ID`, `AllowDuplicate`), tùy chọn `:109` (`BroadcastPayload`); `createWorkflow` `:880`; `maybeEnqueueOnAssign` `:910`; `server/internal/issueguard/duplicate.go:60`; ví dụ `server/internal/service/proposal.go:1061`, `retro.go:358`, `knowledge_audit.go:306` | `Create` có giao dịch riêng cho từng issue, nên tạo N task là N giao dịch; lỗi được bọc bằng `%w` nên `errors.As(*pgconn.PgError)` dùng được cho vi phạm chỉ mục duy nhất. `Create` khóa và kiểm trùng tiêu đề chuẩn hóa dưới cùng cha và dự án, trả `ErrActiveDuplicate` trừ khi `AllowDuplicate`. `createWorkflow` đưa trạng thái xin về `todo` nếu workflow của dự án (hoặc Default ngoài dự án) không liệt kê nó. `maybeEnqueueOnAssign` xếp lượt chạy khi người nhận là agent, trạng thái không phải backlog hay triage và agent không bị chặn (`agentAssigneeVerdict`: máy offline vẫn xếp, runtime bị chặn thì không) |
| 18 | Ghi trạng thái ngoài `UpdateIssue` | `server/cmd/server/runtime_sweeper.go:726`; `server/internal/service/task.go:6489` (đặt `blocked` khi lượt chạy hỏng, `FailedRunLandingStatus`); `service/initiative.go:498,874`; `service/proposal.go:675,1101,1156`; `proposal_expiry.go:101`; `qoder_issue_lifecycle.go:56` | Các đường này không qua chốt kiểm của handler; engine phải chịu được (mục 6) |
| 19 | Lời nhắc workflow | `issueworkflow.go:902` (`Brief`); `issue_workflow_entry.go:562,569` | Chỉ gắn khi có `def`, tức issue có dự án |
| 20 | Bộ nhớ trạng thái tự do của issue | `server/internal/handler/issue_metadata.go` (khóa `^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}$`, ≤ 50 khóa, giá trị nguyên thủy, ≤ 8 KB); route `router.go` (`PUT /metadata/{key}`) | Agent ghi được; **không dùng cho trạng thái engine** |
| 21 | Cờ tính năng | `server/internal/featureflags/keys.go` (hằng khóa và hàm `XxxEnabled(ctx, flags)`); `server/pkg/featureflag/service.go:58` (`IsEnabled(ctx, key, defaultVal)`), `env_provider.go` | Mặc định do nơi gọi truyền (`LocalSearchIndex` mặc định bật, `TriageV1` mặc định tắt). Biến môi trường là `FF_<KHÓA_VIẾT_HOA>` nhận `true/on/1/yes`, `false/off/0/no`; giá trị rỗng là tắt rõ ràng; chưa đặt thì rơi xuống mặc định. Dùng làm công tắc tắt |
| 22 | Template, quickstart, nâng cấp (sau `9d3a9e081`) | `packages/core/squads/templates.ts:134` (`SquadTemplate`), `:146` (`expertReview`), `:168` (`workflow`); `SQUAD_TEMPLATES` do `assembleSquadTemplates` (`packages/core/catalog/squads.ts:52`) dựng từ `CATALOG_SQUADS` sinh ra, dòng `:56` là `squad.workflow === null ? null : workflows[squad.workflow]`; `quickstart.ts:237` (`workflowSteps`), `:814` (`template_key`), `:878-904` (tạo workflow khi `template.workflow` khác null, trùng thì theo **tên**); `upgrade.ts:69` (`decide`), `:79` (`mergeWorkflowSteps`), `:238` (`upgradeKitvietSquad`), `:284` (**tìm squad theo tên**); `kitviet-previous-texts.ts:7,474` | Cả 21 manifest `catalog/squads/kv_*/template.json` có `squad.workflow = null` và `adapter.type = kitviet` (đã đọc từng tệp); 6 squad có `expertReview: true` ở dòng 8; `routines` khác rỗng ở `kv_legal` (dòng 57) và `kv_accounting` (dòng 62). Trường `squad.workflow` của manifest là workflow **dựng cho dự án build**: `validateSquad` (`scripts/catalog/lib/validate.mjs`) buộc nó trỏ tới workflow có khối `workflow`, và 21 test Kit Việt (`packages/core/catalog/kitviet/*.test.ts`, ví dụ `trade.test.ts` "has no routine and no workflow") khẳng định `template.workflow` là null. **Không dùng trường này cho workflow flow** (mục 8, D18) |
| 23 | Runner migration | `server/cmd/migrate/main.go:142` (`concurrentIndexCleanups`), `:493`; test `server/cmd/migrate/migrate_mul5999_index_retry_test.go:163` (`TestEveryConcurrentUpBuildHasCleanup`) | Mỗi migration `CREATE INDEX CONCURRENTLY` **phải** có một dòng trong bảng này, nếu không test đỏ |
| 24 | Cột đã xác nhận (`server/pkg/db/generated/models.go` ở HEAD) | `Issue` có `ParentIssueID`, `Stage`, `Revision` và **không** có `flow*`, `outcome`; `IssueWorkflow` có `ID, WorkspaceID, Name, Description, InitialStatusKey, Steps, IsDefault, OwnerID, SquadID` và không có `flow_config`, `template_ref`, `flow_auto`; `Squad` có `TemplateKey`, `ActiveWorkflowID`; `Attachment` có `IssueID`, `CommentID`, `UploaderType`, `UploaderID`, `TaskID` | Tên cột mới ở mục 3 không va chạm |
| 25 | Đường chạy test (D19) | `Makefile:5` (`ENV_FILE ?=` ưu tiên `.env`, rồi `.env.worktree`); `Makefile:358-362` (`test` = `catalog-build`, `scripts/ensure-postgres.sh`, `go run ./cmd/migrate up`, `scripts/test-go.sh --race`); `Makefile:367-370` (`migrate-up`); `Makefile:377` (`sqlc`); `scripts/ensure-postgres.sh` (khởi `docker compose up -d postgres` rồi tạo DB nếu thiếu); `server/internal/handler/handler_test.go:46-48`, `server/internal/testutil/db_test.go:29-31` và `server/cmd/migrate/main.go:933-936` (mặc định `postgres://multica:multica@localhost:5432/multica` khi `DATABASE_URL` trống; `TestMain` của `handler` ghi fixture vào DB đó); `server/internal/testutil/isolated_db.go:23` (`IsolatedDB` tạo DB `multica_test_<uuid>` trên máy chủ của DSN truyền vào rồi chạy migrate trên DB mới); `server/internal/testutil/isolated_db_test.go:11-17` (DSN có thể là URL kèm `?database=`/`dbname=` hoặc dạng `key=value`) | Mã Go của `server` không đọc tệp `.env` (không có `godotenv` trong `server/`); chỉ `make` và các script `bash` nạp tệp env. Trên máy rà soát (tệp gitignore) `.env` trỏ DB `multica` dùng chung. `make test` luôn migrate rồi test trên DB của `ENV_FILE`; `go test` trần của gói DB và `go run ./cmd/migrate up` trần đều rơi về DB `multica`. **D19 (chọn bởi trợ lý):** test DB chỉ chạy khi `DATABASE_URL` trỏ tới DB riêng có tên kết thúc `_test` và khác `multica`, đặt tường minh trong từng lệnh, qua guard `scripts/require-test-database.sh` (S0); không dùng `make test`, `make check`, `make migrate-up` hay `ensure-postgres.sh` cho việc này (mục 0.1.1 của kế hoạch) |
| 26 | Catalog: nguồn template, mã sinh không commit | `catalog/README.md`; `scripts/catalog/cli.mjs` (`check` chỉ đọc, `build` ghi); `.gitignore:43-46`; `Makefile:331` (`catalog-build`); `turbo.json` (task `//#catalog:build`); `scripts/test-go.sh:8-9` (tự chạy `node scripts/catalog/cli.mjs build --target go`); `scripts/catalog/lib/build.mjs` (ghi `packages/core/catalog/generated/*.ts` và `server/internal/catalog/generated/*.json`); `server/internal/catalog/catalog.go` | `catalog/workflows/` hiện có hai dạng, cả hai `ui: false`: workflow thực thi (khối `workflow`: `sdlc_auto`, `planning_auto`) và mẫu trình thiết kế (khối `designer`: `designer_*`, không đặt được tên agent). `scripts/catalog/lib/build-workflows.mjs` kiểm hai dạng và báo "workflow block is missing" cho dạng khác: **khối thứ ba cho workflow flow chưa có** (S11). Quy tắc catalog: id không đổi, thêm `version` khi đổi nội dung, mẫu mà mã tìm theo tên khai `contracts`. `pnpm catalog:check` (`cli.mjs check`) chỉ đọc tệp: đã chạy ở HEAD, kết quả `catalog ok: 181 templates` và 3 cảnh báo về skill vendored |
| 27 | Nguồn Kit Việt và generator | `catalog/_sources/kitviet/{upstream,patches,squads,extensions,skills}` và `rewrites.json`; `scripts/generate-kitviet-pack.mjs` (ghi `packages/core/catalog/generated/kitviet/*.ts`; tham số `--out <thư mục>`; `--check` so với thư mục đã sinh nên chỉ có nghĩa sau `catalog build`; thứ tự áp dụng cho mỗi văn bản: bản vá `patches/`, rồi các quy tắc của `rewrites.json` theo thứ tự mảng, rồi giải tham chiếu `kv:<agent>` thành khóa vai, `generate-kitviet-pack.mjs:335-337`; quy tắc không khớp chỗ nào mà không `optional` làm generator lỗi, `:883`); `packages/core/squads/kitviet-squad-roles.ts`, `kitviet-agent-instructions.ts:211` (`KV_LEAD_NOTES`), `kitviet-previous-texts.ts` | Chạy generator ra thư mục tạm cho kết quả giống từng byte với bản đã commit ở `b324e4241`, trừ hai dòng chú thích đầu tệp (đã so `trade`, `b2b`, `legal`). `kitviet-previous-texts.ts` do `scripts/kitviet-update.mjs` ghi khi nâng upstream (`WRITE_PATHS`); thay đổi văn bản lead ở S13 phải thêm mục cũ vào tệp này |
| 28 | Kiểm tra migration | `server/internal/migrations/migrations_lint_test.go`: `TestMigrationNumericPrefixesAreUnique` (tiền tố từ 129 phải duy nhất, ngoại lệ 595–599), `TestMigrationFilesHaveMatchingDirections` (đủ `.up` và `.down`) | Ở HEAD có 16 tiền tố trùng (600–608 và 610–616) ngoài danh sách ngoại lệ, nên `TestMigrationNumericPrefixesAreUnique` **đỏ sẵn** (tính từ `git ls-tree HEAD server/migrations/`, 874 tệp up, số lớn nhất 984). Cây làm việc có bản sửa chưa commit của người khác; không sửa tệp này. 985–987 là số trống nên không thêm trùng |
| 29 | Trình thiết kế dựng lại bước khi lưu | `packages/views/workflow/workflow-data.ts:14-26` (`workflowStepInputs`), gọi ở `packages/views/workflow/designer/workflow-designer-page.tsx:439`; `server/internal/handler/issue_workflow.go:558-592` (`resolveStepInputs`) | Cả hai chỉ chép khóa trạng thái, handler, hướng dẫn, chuyển tiếp, bố cục: trường mới (`tasks`, `human_only`) **bị mất** nếu client cũ lưu lại workflow. Đó là lý do khóa 409 cho workflow có `template_ref` và giữ trường cũ khi yêu cầu không gửi (mục 7.5) |
| 30 | Tệp đính kèm và người tải | `server/pkg/db/queries/attachment.sql:96-102` (`LinkAttachmentsToComment` đòi `issue_id` bằng issue của comment và `comment_id IS NULL`), `:200-214` (`LockAttachmentsForCommentLink`); `server/internal/handler/file.go:455-460` (`resolveActor` ghi `uploader_type`, `uploader_id`) | Tệp gắn vào comment vẫn mang `issue_id` của issue chứa comment; nên R2 kiểm `issue_id` = issue của task là đủ cho cả tệp ở mô tả lẫn ở comment. Agent tải lên thì `uploader_type = 'agent'` và `uploader_id` là agent của lượt chạy |
| 31 | Các đường tạo issue | Qua `IssueService.Create`: `handler/issue.go:3627`, `source_context.go:723`, `squad_sources.go:117`, `service/knowledge_audit.go:306`, `proposal.go:1061`, `retro.go:358`, `review.go:508`, `squad_handoff.go:473`. Không qua `Create` (truy vấn trực tiếp): `service/autopilot.go:784`, `goals_planning.go:167`, `initiative.go:266`, `initiative_build.go:103`, `proposal.go:317` | Một móc duy nhất trong `IssueService.Create` **không đủ** để biết mọi request. Vì vậy request được khởi tạo bằng `EnsureFlowStarted` idempotent gọi từ nhiều nơi (mục 4.4) |

## 3. Mô hình dữ liệu

### 3.1 Migration (đánh số 985, 986, 987)

HEAD `ba189d733` có `984_publish_attempt_sending_status` là migration cuối (874 tệp up; 985, 986, 987 trống); `git diff --stat b324e4241 HEAD -- server/migrations` rỗng; `git status --short -- server/migrations` rỗng; `docs/specs/template-registry/plan.md:12` và `spec.md:334` ghi rõ phiên chuyển `catalog/` không thêm migration. **Chọn 985 đến 987.** S0 kiểm lại và đổi số nếu có va chạm (mỗi tiền tố số phải duy nhất; xem mục 2 dòng 28 về bài kiểm tiền tố đỏ sẵn).

| Tệp | Nội dung (mỗi tệp một ý; tệp chỉ mục chỉ một câu lệnh) |
| --- | --- |
| `985_issue_flow_columns.up.sql` | `ALTER TABLE issue ADD COLUMN IF NOT EXISTS outcome TEXT;` `... flow JSONB;` `... flow_step TEXT;` `... flow_task TEXT;` `... flow_round INTEGER;` `ALTER TABLE issue_workflow ADD COLUMN IF NOT EXISTS flow_config JSONB;` `... template_ref TEXT;` `... flow_auto BOOLEAN NOT NULL DEFAULT false;` |
| `985_issue_flow_columns.down.sql` | `ALTER TABLE ... DROP COLUMN IF EXISTS` cho cả tám cột |
| `986_issue_flow_task_once_idx.up.sql` | `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_flow_task_once ON issue (parent_issue_id, flow_step, flow_task, flow_round) WHERE flow_task IS NOT NULL;` |
| `986_issue_flow_task_once_idx.down.sql` | `DROP INDEX CONCURRENTLY IF EXISTS idx_issue_flow_task_once;` |
| `987_issue_flow_open_idx.up.sql` | `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_flow_open ON issue (workspace_id, id) WHERE flow IS NOT NULL AND NOT (flow ? 'closed_at');` |
| `987_issue_flow_open_idx.down.sql` | `DROP INDEX CONCURRENTLY IF EXISTS idx_issue_flow_open;` |

Quy tắc áp dụng:

- **Không khóa ngoại, không cascade, không CHECK** trên cột mới (kiểm ở ứng dụng; tránh quét bảng `issue` nóng khi thêm ràng buộc). `ADD COLUMN` không mặc định (trừ `flow_auto` mặc định hằng số) là thay đổi siêu dữ liệu, giống migration `909` (`issue.run_queued`...).
- **Chỉ mục:** `CREATE INDEX CONCURRENTLY IF NOT EXISTS`, một câu lệnh mỗi tệp, runner chạy ngoài giao dịch.
- **Đăng ký bắt buộc:** thêm vào `server/cmd/migrate/main.go:142` hai dòng `"986_issue_flow_task_once_idx": "idx_issue_flow_task_once"` và `"987_issue_flow_open_idx": "idx_issue_flow_open"`. Không thêm vào bảng `concurrentDownIndexCleanups` (`:493`, chỉ cho down tạo lại chỉ mục).
- **Migration bị bỏ qua có điều kiện** vẫn được ghi vào `schema_migrations` (AGENTS.md). Mọi DDL về sau chạm các cột/chỉ mục này phải idempotent (`IF EXISTS`/`IF NOT EXISTS`). Bộ migration này không có nhánh bỏ qua có điều kiện.
- Bảng `issue` lớn: README của runner (`server/cmd/migrate/README.md`) khuyên dựng trước chỉ mục ở khung giờ thấp điểm; ghi vào runbook (S27).
- Khôi phục: chạy down theo thứ tự 987, 986, 985; trước đó `detach` mọi request đang chạy (mục 7.1, 9.3), vì down xóa dữ liệu flow.

### 3.2 Ý nghĩa cột

| Cột | Kiểu | Ai ghi | Nội dung |
| --- | --- | --- | --- |
| `issue.outcome` | `TEXT NULL` | Ghi trạng thái đóng task hoặc engine | `approved`, `rejected`, `blocked`, `done` (kiểm bằng `IsOutcome`, `transitions.go:87`); NULL khi chưa báo |
| `issue.flow` | `JSONB NULL` | **Chỉ engine** | Trạng thái của **request**: `{"workflow_id": uuid, "step": "gate", "rework": 1, "switches": {"route": "..."}, "entered_at": RFC3339, "stop": null\|1\|3\|"stalled", "blocked_from": "gate", "plan_done": true, "closed_at": null}` |
| `issue.flow_step`, `flow_task`, `flow_round` | `TEXT`, `TEXT`, `INTEGER` NULL | Chỉ engine | Danh tính của **task**: bước, khóa task, vòng (= `flow.rework` lúc mở). Cùng `parent_issue_id` tạo khóa duy nhất ở chỉ mục 986 |
| `issue_workflow.flow_config` | `JSONB NULL` | Chỉ tạo từ template (admin) | Cấu hình cấp workflow (mục 3.4). Workflow có `flow_config` khác NULL là **workflow flow** |
| `issue_workflow.template_ref` | `TEXT NULL` | Như trên | `kv_trade@<rev>`; khác NULL nghĩa là workflow do template quản lý, chỉ đọc |
| `issue_workflow.flow_auto` | `BOOLEAN NOT NULL DEFAULT false` | Admin, endpoint riêng | Bật runner (tầng 2) cho squad đang dùng workflow này |

Số vòng sửa nằm ở `issue.flow.rework`, **không** suy từ `activity_log`: nhật ký được ghi bất đồng bộ qua sự kiện (mục 2 dòng 16) nên không đủ tin cậy để ra quyết định.


### 3.3 Mở rộng bước (`server/internal/issueworkflow/`)

```go
// Step gains two optional fields; a step without them behaves exactly as before.
type Step struct {
    // ... existing fields
    Tasks     []Task `json:"tasks,omitempty"`      // sub-issues opened when the request enters the step
    HumanOnly bool   `json:"human_only,omitempty"` // only a member may move the request out of the step
}

// Task is one unit of work. Kind "gate" must report an outcome to close.
type Task struct {
    Key          string     `json:"key"`                    // [a-z][a-z0-9_]{0,31}, unique in the step
    Title        string     `json:"title"`                  // at most 80 runes
    Kind         string     `json:"kind"`                   // "work" | "gate"
    Handler      Handler    `json:"handler"`                // type "agent" with an id; resolved by the quickstart
    Instructions string     `json:"instructions"`           // at most MaxInstructionsRunes (8000)
    When         *Condition `json:"when,omitempty"`         // absent: always opened
    Requires     []string   `json:"requires,omitempty"`     // "outcome" | "attachment"
}
```

Giới hạn: tối đa 12 bước flow, 8 task mỗi bước, 8 công tắc. Hai gate cùng bước phải có `handler` khác nhau, và gate không được trùng agent với task `work` của bước ngay trước nó (kiểm lúc ghi workflow: `invalid_workflow`; kiểm lại lúc mở task).

Tuần tự trong cùng bước **không** hỗ trợ: dùng hai bước (ví dụ Talent: `Screen` rồi `Draft`; Banking: `Analyze` rồi `Draft`).

### 3.4 `flow_config`

```json
{
  "version": 1,
  "task_timeout_minutes": 120,
  "task_hard_cap_minutes": 240,
  "task_max_attempts": 2,
  "switches": [
    {"key": "route", "type": "enum", "values": ["shipment_dossier"], "default": "shipment_dossier"},
    {"key": "customs_lookup", "type": "bool", "default": false},
    {"key": "export", "type": "bool", "default": false}
  ],
  "messages": {"stop1": "...", "stop2": "...", "stop3": "...", "stalled": "..."}
}
```

- `switches[].key` khớp `^[a-z][a-z0-9_]{0,31}$`; kiểu `bool` hoặc `enum`; công tắc `route` là enum bắt buộc nếu squad có nhiều route.
- `messages`: mẫu tiêu đề và thân comment dừng, giữ đúng tiêu đề của `kv-squad-lead` (`BLOCKED · stop 1 · thiếu căn cứ`, `IN REVIEW · stop 2 · trước khi ra khỏi workspace`, `BLOCKED · stop 3 · hết 2 vòng sửa`) và dòng nhắc người yêu cầu `[@name](mention://member/<id>)` với id lấy từ `creator_id` khi `creator_type` là `member`. Văn bản do pack sở hữu, không nằm trong mã Go.
- Khóa hiệu lực: mọi `Condition` dùng trường trong mục 3.5; mọi `switch.<key>` phải có khai báo.

### 3.5 Điều kiện và dữ kiện

Mở rộng `Facts` (`transitions.go:76`) và bảng trường hợp lệ (`transitions.go` quanh `FieldReworkCount`):

| Trường | Kiểu | Toán tử | Ý nghĩa |
| --- | --- | --- | --- |
| `tasks_outcome` | enum | `eq`, `in` | Gom outcome của các task bước vừa đóng: `all_approved`, `any_rejected`, `any_blocked`, `none` (bước không có gate). `any_blocked` thắng `any_rejected`; task đóng không có outcome tính là `rejected` nếu là gate |
| `rework_count` | số | `eq, neq, gt, gte, lt, lte` | Với request flow là `flow.rework` (đã có tên trường này) |
| `switch.<key>` | bool/enum | `eq`, `neq`, `in` | Giá trị công tắc của request |
| `outcome` | enum | như cũ | Outcome do **lead** báo ở bước plan (`blocked`) |

Chuyển tiếp có thêm cờ `rework` (boolean): khi đi theo chuyển tiếp này, `flow.rework` tăng 1 và các task của bước đích mở với `flow_round` mới. Giới hạn là điều kiện `rework_count lt N` trên chính chuyển tiếp đó: Kit Việt dùng `N = 2`, SDLC giữ nguyên hằng số 3 (không dùng cơ chế này). `rework` thứ 2 xong, gate lần thứ ba bị chặn khiến điều kiện `rework_count lt 2` sai và rơi xuống chuyển tiếp mặc định `blocked` (dừng 3), đúng "sau vòng sửa thứ hai" của `kv-squad-lead` bước 6.

### 3.6 Ví dụ đầy đủ: Trade (squad thí điểm)

Trạng thái dùng chung (tạo một lần mỗi workspace, tái dùng theo tên: `issue_workflow.go:611-621`): `Collect`, `Draft`, `Gate`, `Handover` (loại `started`). Có sẵn: `todo`, `in_review`, `blocked`, `done`, `cancelled`.

```text
step todo      (plan, lead)     transitions: [outcome=blocked -> blocked] [default -> collect]
step collect   tasks: docs (work, agent trade_docs_reader, requires [attachment])
                      customs (work, trade_customs_research, when switch.customs_lookup = true, requires [attachment])
               transitions: [tasks_outcome=any_blocked -> blocked] [default -> draft]
step draft     tasks: crosscheck (work, trade_crosscheck_drafter, requires [attachment])
               transitions: [tasks_outcome=any_blocked -> blocked] [default -> gate]
step gate      tasks: docs_gate (gate, trade_docs_gate, requires [outcome])
                      customs_gate (gate, trade_customs_gate, requires [outcome])
               transitions: [any_blocked -> blocked]          # gate nói lỗi nằm ở chứng từ gốc: outcome blocked, dừng 1
                            [all_approved -> handover]
                            [any_rejected AND rework_count < 2 -> draft, rework]
                            [default -> blocked]            # stop 3
step handover  tasks: handover (gate, trade_handover, requires [outcome])
               transitions: [approved -> in_review] [default -> blocked]
step in_review human_only   tasks: export (work, trade_crosscheck_drafter, when switch.export = true)
               leaving: a member moves the request to done
step blocked   (stop 1 / stop 3 / stalled)   leaving: member `resume`
```

Hai quyết định rút ra từ diễn tập thật (docs/engineering/kitviet-flow-web-drill.md, mục 8 và 9):

- **Đầu ra của Collect là tệp (`requires: ["attachment"]`).** Engine giao cho bước sau tệp đính kèm của bước trước (mục 3.4 và `Inputs` trong mô tả task), nên một task Collect đóng mà không có tệp để drafter và gate không có gì để đọc, và vòng sửa không làm lại Collect nên không cứu được. Vai collector của pack ghi "không có công cụ ghi" (đúng cho chế độ cũ, nơi kết quả nằm ở comment); hướng dẫn của task nói rõ rằng ở task này vẫn tạo tệp bằng shell. Server từ chối đóng task thiếu tệp (R2). Áp cho cả 21 manifest, xem mục 12.4.
- **Gate phân biệt `rejected` và `blocked`.** `rejected` gửi bản nháp về cho drafter sửa (dùng một vòng, tối đa 2). Nếu mọi lỗi chặn nằm ở chính chứng từ gốc (số liệu mâu thuẫn giữa các chứng từ, chứng từ thiếu hoặc chưa được xác nhận) và bản nháp đã nêu đúng, drafter không sửa được; gate đóng task bằng `blocked --outcome blocked` và request dừng 1 để người xác nhận rồi `resume`, không tốn vòng sửa. Không có thay đổi engine: chuyển tiếp `any_blocked -> blocked` đã có. Áp cho mọi gate của 21 manifest, kể cả gate bàn giao (mục 12.4).

Vai (agent) khớp `catalog/squads/kv_trade/template.json` (`squad.roles`, 7 vai gồm lead; hồ sơ pipeline ở `catalog/_sources/kitviet/upstream/xuat-nhap-khau/skills/day-chuyen-ho-so-lo-hang/SKILL.md:25-33`): `trade_docs_reader`, `trade_customs_research`, `trade_crosscheck_drafter`, `trade_docs_gate`, `trade_customs_gate`, `trade_handover` (HEAD). Điểm dừng 1 của Trade là thiếu invoice, packing list, B/L hoặc loại hình tờ khai (`KV_LEAD_NOTES.kv_trade`, `kitviet-agent-instructions.ts:445`): `docs` báo outcome `blocked`.

### 3.7 Bất biến kiểm tra khi ghi workflow (Go, `issueworkflow.ValidateFlow`)

1. `flow_config` khác NULL thì có ít nhất một bước có `tasks`; ngược lại `tasks` cấm.
2. Bước `todo` là bước plan: không có `tasks`; có chuyển tiếp mặc định.
3. Mọi `to_status_key` thuộc workflow; có đúng một chuyển tiếp mặc định cuối mỗi bước có chuyển tiếp; chuyển tiếp `rework` chỉ trỏ về bước đứng **trước** bước nguồn; mọi chu trình đi qua ít nhất một chuyển tiếp `rework` có điều kiện `rework_count lt N` (N hữu hạn, ≤ 5).
4. Mọi bước có `tasks` có chuyển tiếp; bước `gate`-only có chuyển tiếp cho `all_approved`, `any_rejected`, `any_blocked` hoặc mặc định.
5. Mỗi `Task.Handler` là `agent` có `id`; hai gate cùng bước khác agent; `Kind: gate` thì `Requires` chứa `outcome`.
6. Mọi `switch.<key>` trong điều kiện có khai báo; mọi `route` trong `when` thuộc `values`.
7. `HumanOnly` không đi với `tasks` giao cho agent trừ task `export` tùy chọn.
8. Tổng số bước, task, công tắc trong giới hạn mục 3.3; chuỗi theo giới hạn có sẵn (tên 64, mô tả 256, hướng dẫn 8000 rune).

## 4. Tầng 1: kiểm (engine thực thi bất biến)

Tầng 1 giao lên trước. Lead vẫn gọi `advance` (mục 7); **mọi thứ còn lại do máy chủ làm và kiểm.**

### 4.1 Sơ đồ trạng thái của request

```text
          plan ok                all closed        all closed
 todo ───────────────► collect ───────────► draft ───────────► gate
  │ outcome=blocked        │ any_blocked        │ any_blocked      │ all_approved ──► handover ── approved ──► in_review (human_only) ── member ──► done
  ▼                        ▼                    ▼                  │ any_rejected & rework<N ──► draft (rework+1, mọi gate mở lại)
 blocked(stop 1) ◄─────────┴────────────────────┘                  │ any_blocked ──► blocked(stop 1)
                                                                    └ default ──► blocked(stop 3)
 handover ── rejected ──► blocked(stop 3)
 blocked ── member resume ──► blocked_from  (stop 3: rework = 0; stop 1: giữ rework)
 mọi bước ── member cancel ──► cancelled (hủy task đang mở)
```

Task: `todo` → `in_progress` → `done` (kèm `outcome`) hoặc `blocked` (không đóng, giữ rào chắn) hoặc `cancelled` (engine/member hủy). Task `blocked` làm bước "treo"; engine đưa request sang `blocked` (dừng 1) khi mọi task còn lại đã đóng hoặc ngay khi có outcome `blocked` (chọn: **ngay**, để người nhận cảnh báo sớm).

### 4.2 Quy tắc thực thi

| # | Quy tắc | Nơi thực thi |
| --- | --- | --- |
| R1 | Gate (hoặc task có `requires: ["outcome"]`) chỉ đóng (`done`) khi có outcome ∈ {`approved`, `rejected`}; thiếu thì 422 `flow_outcome_required` và trạng thái giữ nguyên; `done` kèm outcome `blocked` hoặc `blocked` kèm outcome khác `blocked` thì 422 `flow_outcome_invalid`. Task không thể làm tiếp báo `multica issue status <id> blocked --outcome blocked`. Kết luận trong comment (`VERDICT: QUA\|CHẶN`) chỉ để người đọc; **outcome mới là nguồn sự thật** | `UpdateIssue` (`issue.go:4225`) và batch (`issue.go:5140`, hiện truyền outcome rỗng: phải truyền outcome từng dòng hoặc từ chối) |
| R2 | Task có `requires: ["attachment"]` chỉ đóng khi có ít nhất một dòng `attachment` với `issue_id` = issue task (tệp gắn vào comment của task cũng mang `issue_id` này, mục 2 dòng 30), `uploader_type = 'agent'` và `uploader_id` = người nhận task; thiếu thì 422 `flow_attachment_required` | như R1 |
| R3 | Agent không rời, không vào, không sửa trạng thái của bước `human_only`; 403 `flow_human_only`. Chỉ thành viên có `canManageSquad` (`squad.go:139`) | `applyIssueWorkflow` |
| R4 | Request đang được flow quản lý: agent ghi trạng thái trực tiếp bị 409 `flow_managed_request`; thành viên có `canManageSquad` được phép (engine đồng bộ lại: mục 6 dòng "thành viên sửa tay"); người khác bị 403 | `UpdateIssue`, batch |
| R5 | Agent không tạo sub-issue dưới request flow (409 `flow_children_managed`); thành viên được | `CreateIssue` (`issue.go:3315`) |
| R6 | Sub-issue task chỉ do engine tạo (cột `flow_task` do engine đặt; API công khai không nhận trường này) | `CreateIssue`, `UpdateIssue` bỏ qua/từ chối |
| R7 | Gate không giao cho agent đã làm task `work` cùng vòng (422 `flow_gate_same_agent` khi mở task); handler lấy từ định nghĩa, lead không chọn | `FlowEngine.openTasks` |
| R8 | Chuyển bước chỉ qua `advance`/`plan`/`resume` (engine); không có lối nhảy bước ngoài `resume` | engine |
| R9 | **Rerun mọi gate (D3):** mỗi lần vào bước `gate`, engine mở **toàn bộ** task gate của bước (không chỉ task bị chặn), `flow_round` mới | `FlowEngine.openTasks` |
| R10 | Đóng request: `done` chỉ do thành viên; request flow không có "done do agent" | R3, R4 |
| R11 | Công tắc chỉ đặt được trong bước plan (hoặc bởi thành viên `canManageSquad` ở mọi bước, kèm ghi nhật ký); giá trị phải thuộc khai báo; 422 `flow_switch_invalid`, 409 `flow_switch_locked` | `PUT /flow/switches/{key}` |

### 4.3 Mã lỗi

Định dạng giữ như `writeIssueWorkflowError` (`issue_workflow_entry.go:309`): JSON `{"error": <văn bản agent đọc được>, "code": <mã>, ...}`.

| Mã | HTTP | Khi nào | Trường thêm |
| --- | --- | --- | --- |
| `flow_outcome_required` | 422 | Đóng gate/task `requires outcome` mà thiếu outcome | `task`, `allowed_outcomes` |
| `flow_outcome_invalid` | 422 | Cặp trạng thái và outcome không hợp lệ (xem R1) | `task`, `allowed_outcomes` |
| `flow_attachment_required` | 422 | Đóng task `requires attachment` mà thiếu tệp | `task` |
| `flow_human_only` | 403 | Agent chạm bước `human_only` | `step` |
| `flow_managed_request` | 409 | Ghi trạng thái trực tiếp lên request flow không đúng quyền | `step`, `hint: "multica issue flow advance"` |
| `flow_children_managed` | 409 | Agent tạo sub-issue dưới request flow | `step` |
| `flow_gate_same_agent` | 422 | Gate trùng agent với task `work` cùng vòng | `task` |
| `flow_switch_invalid` | 422 | Công tắc hoặc giá trị không khai báo | `switch`, `allowed` |
| `flow_switch_locked` | 409 | Đặt công tắc ngoài bước plan | `step` |
| `flow_wrong_step` | 409 | `plan` khi không ở bước plan | `step` |
| `flow_not_managed` | 404 | Issue không do flow quản lý | |
| `flow_forbidden` | 403 | Người gọi không phải lead của squad, không có `canManageSquad` | |
| `flow_agent_unavailable` | 409 | (trả trong `advance`, không phải lỗi chặn) agent của task đã lưu trữ hoặc không nhận được việc; request chuyển `blocked` | `task`, `agent_id` |
| `workflow_managed_by_template` | 409 | `PATCH`/`PUT handoff` lên workflow có `template_ref` | |
| `workflow_has_flow_requests` | 409 | Xóa workflow còn request flow đang mở ghim vào nó (thêm vào danh sách chặn xóa `issue_workflow.go:1103`) | `count`, `ids` |
| `flow_workflow_forbidden` | 403 | Không phải admin mà tạo workflow có `flow_config` | |

### 4.4 Khởi tạo request flow

Một móc trong `IssueService.Create` không phủ được mọi đường tạo (mục 2 dòng 31: 8 đường qua `Create`, 5 đường dùng truy vấn trực tiếp). Engine dùng `EnsureFlowStarted(issue)`, idempotent, đặt `issue.flow` cho request đủ điều kiện: không có `parent_issue_id`, `assignee_type = 'squad'`, workflow đang dùng của squad đó có `flow_config`, và chưa có `flow`. Ghi `flow.workflow_id` lúc khởi tạo. Hàm được gọi ở: (1) `GET /api/issues/{id}/flow` và `POST .../flow/plan` (văn bản lead hai chế độ bắt đầu bằng `flow show`); (2) ngay sau `CreateIssue` HTTP (`issue.go:3627`) và `squad_handoff.go:473`; (3) job quét tầng 2 cho request đủ điều kiện mà chưa có `flow`, chỉ khi request do **thành viên** tạo (`creator_type = 'member'`: agent giao việc cho squad qua đường tự khởi tạo, ví dụ bàn giao giữa squad, còn job lập kế hoạch công ty tạo issue dưới tên lead nên không phải request) và chưa bị `detach` (mục `flow_detached` trong `activity_log`, ghi cùng giao dịch với `detach`; thiếu nó thì quét đưa request vừa gỡ trở lại flow trong 30 giây). Hệ quả: một request bị agent chạm trước lần gọi đầu của lead thì chưa được bảo vệ; sau lần gọi đầu, mọi chốt R1 đến R11 áp dụng.

Định nghĩa luôn lấy theo `flow.workflow_id`, **không** qua `ForIssue` (trả `nil` khi issue không có dự án, `issueworkflow.go:611-613`), nên request flow không cần dự án.

## 5. Tầng 2: runner (job nền ở máy chủ)

Chỉ bật sau cổng G1 (S17 trong kế hoạch; điều kiện ở mục 11 và D14). Runner **không có logic riêng**: nó gọi đúng `FlowEngine.Advance` của tầng 1.

### 5.1 Vị trí

- Quyết định thuần (không DB): `server/internal/issueworkflow/flow.go` (`Decide(def, state, tasks) Decision`).
- Điều phối có DB, tạo issue, phát sự kiện, comment dừng: `server/internal/handler/flow_engine.go` (**đặt trong `handler`** vì sự kiện `issue:updated` cần `handler.IssueResponse`, mục 2 dòng 16; đúng chỗ của `PromoteQueuedIssues`, dòng 12).
- Job: `server/internal/scheduler/jobs_flow.go` nhận interface `FlowAdvancer` do `*handler.Handler` cài (mẫu `IssueQueuePromoter`, `jobs_issue_queue.go:11-13`); đăng ký ở `server/cmd/server/main.go` cạnh dòng 860.

### 5.2 Kích hoạt

1. **Sự kiện sub-issue (nhanh):** trong `Handler.processChildEvents` (`handler/issue_child_event.go:14-15`), sau `ProcessChildEvents`, gọi `h.advanceFlowRequests(ctx, parentIDs...)` cho cha là request flow `auto`. Đặt ở `handler` (cùng package với `FlowEngine`) thay vì trong `service.IssueWakeupService`, vì `IssueWakeupService` được dựng theo giá trị ở 15 nơi mà không có phụ thuộc nào (mục 2 dòng 13), và `finish()` đã đánh dấu sự kiện xong trước khi gọi tiếp (`issue_wakeup_system.go:214,256`) nên hàng đợi sự kiện không thử lại giúp. Móc này chỉ chạy ở các đường ghi gọi `Handler.processChildEvents` (`issue.go:4421,5258`, `issue_queue.go:304`, `pr_auto_complete.go:305`); các đường ghi hệ thống (`service/task.go:6489`, `runtime_sweeper.go:726`) chờ job quét ở điểm 2, tối đa một chu kỳ 30 giây.
2. **Quét định kỳ (an toàn):** job `flow_sweep` (30 giây, `CatchUpLatestOnly`, `AllowStaleReentry: true`, `RunTimeout: 45s`, `StaleTimeout: 1m`, `HeartbeatInterval: 10s`, `MaxAttempts: 1`, `ScopeGlobal`), đọc chỉ mục `idx_issue_flow_open`, tối đa 200 request mỗi lượt, `Advance` từng request.
3. **Hoàn tất lượt chạy** (`daemon.go:5060`): gọi `Advance` cho request của task vừa xong, bổ sung cho `reconcileUnmovedWorkflowRun` (chỉ phát hiện task treo, không đẩy bước).
4. Rule `child_done` (`issue_wakeup_system.go:115`): **không bắn** cho request flow `auto` (nếu không lead bị đánh thức dư ở mỗi bước). Chỗ chặn là `dispatchSystem` (`issue_wakeup_system.go:411`), ngay sau `LockWakeupIssue`: bỏ qua khi cha là request flow `auto`. `auto` được tính tại chỗ, không lưu: `FF_FLOW_RUNNER` bật và `issue_workflow.flow_auto` của `flow.workflow_id` là true (truy vấn mới `IsFlowRequestAuto`). Nhờ vậy tắt cờ hoặc tắt `flow_auto` thì `child_done` chạy lại ngay mà không cần job quét. Request flow không `auto` (tầng 1) vẫn nhận `child_done` để lead gọi `advance`.

### 5.3 `Advance(request)` – bất biến và khóa

Ba pha, không phụ thuộc bộ nhớ tiến trình:

1. **Quyết định (một giao dịch, khóa dòng):** `SELECT ... FOR UPDATE` request (truy vấn mới `LockIssueForFlow`, mẫu `LockSquadForWorkflowChange`, `squad.sql:32`). Đọc `flow`, danh sách task theo `parent_issue_id`. Nếu bước hiện tại có task chưa đóng: không chuyển. Nếu mọi task đã đóng: `Decide` → `Route` → ghi `flow` (bước mới, `rework`, `stop`), `UpdateIssueStatus`, comment dừng nếu có, `SetIssueFlow` có điều kiện `WHERE flow->>'step' = $bước_cũ AND (flow->>'rework')::int = $vòng_cũ` để hai quyết định đồng thời không cùng thắng. Trạng thái của request ghi bằng `UpdateIssueStatus` (`issue.sql:320`): câu này tăng `revision` khi đổi trạng thái và **không** có tham số `expected_revision` (tham số đó chỉ có ở `UpdateIssue`, `issue.sql:259,317`), nên chống thắng đôi bằng khóa dòng `FOR UPDATE` cộng điều kiện trên `flow`, không bằng `revision`. Commit.
2. **Mở task thiếu (ngoài giao dịch):** tập task kỳ vọng cho `(bước, rework)` = các task có `when` đúng. So với task đã có theo `(parent, flow_step, flow_task, flow_round)`; tạo task còn thiếu bằng `IssueService.Create` (mỗi lần một giao dịch riêng, mục 2 dòng 17). Mỗi lần tạo dùng `AllowDuplicate = true` (khóa trùng tiêu đề vẫn được lấy nhưng không từ chối) và tiêu đề `<bước> · <task> · v<vòng>`; vi phạm chỉ mục duy nhất 986 (`*pgconn.PgError`, mã `23505`, ràng buộc `idx_issue_flow_task_once`, nhận bằng `errors.As` vì `Create` bọc lỗi bằng `%w`) được coi là task đã có. Task tạo ở `todo`, giao agent của định nghĩa, `parent_issue_id` = request, `stage` = vị trí bước, cột `flow_*` đặt trong cùng câu `INSERT` (qua tham số mới của `IssueCreateParams`), mô tả sinh từ mẫu (mục 5.5). Tạo `todo` có người nhận là agent sẽ tự xếp lượt chạy qua `maybeEnqueueOnAssign` (đã đọc, mục 2 dòng 17); nếu agent bị chặn (`agentAssigneeVerdict`) thì issue vẫn được tạo mà không có lượt chạy, và đường phát hiện treo ở mục 5.6 xếp lượt chạy bằng `TaskService.EnqueueTaskForIssue`.
3. **Phát sự kiện:** `issue:updated` (`status_changed: true`, `prev_status`, nguồn `flow`), `issue:created` cho task, `issue_flow:updated` (mục 7.3).

**Điều khiến chạy lại là no-op:** (a) bước hiện tại đã có đủ task kỳ vọng và còn task mở; hoặc (b) request đang ở điểm dừng; hoặc (c) request đã `closed_at`. Chạy lại không tạo issue, comment hay thay đổi trạng thái nào (tiêu chí B1).

**Crash giữa pha 1 và 2:** lần `Advance` sau thấy bước mới thiếu task và tạo bù. **Crash trong pha 2:** thiếu một phần task; lần sau tạo bù.

### 5.4 Rào chắn, verdict, vòng, dừng

- **Rào chắn:** bước xong khi mọi task có trạng thái thuộc nhóm đóng (`done`/`cancelled`, hoặc trạng thái tùy biến loại `done`/`closed`, cùng định nghĩa "đã xong" của `issue.sql:806-840`) **và** không còn lượt chạy `queued/dispatched/running/waiting_local_directory` của task đó (tránh đóng trạng thái trước khi lượt chạy kết thúc). Task `blocked` giữ rào chắn.
- **Gom verdict:** đọc `issue.outcome` của từng task, tính `tasks_outcome` (mục 3.5). Gate đóng không outcome không thể xảy ra (R1); nếu dữ liệu cũ/đường ghi hệ thống để lọt, tính `rejected`.
- **Đếm vòng:** `flow.rework` tăng đúng khi đi theo chuyển tiếp `rework`; không đếm lại từ nhật ký.
- **Comment dừng: server đăng, trừ khi lead đã đăng cho đúng dừng đó.** Luật squad và skill `kv--squad-lead` (bước 7) bảo lead tự đăng comment dừng, còn trong chế độ flow server cũng đăng, nên không được để hai bên cùng đăng. Server không dựa vào lời nhắc để tránh trùng: trước khi đăng, nó tìm comment của agent trên request có tiêu đề đúng dừng (`BLOCKED · stop 1`, `IN REVIEW · stop 2`, `BLOCKED · stop 3`) được viết sau `entered_at` của bước vừa dừng (`stopCommentByLead`). Comment của dừng trước có `entered_at` cũ hơn nên không tính; tiêu đề của dừng khác cũng không tính. Nếu có, server bỏ qua phần comment. **Trạng thái request luôn do server đặt**, kể cả khi lead đã đăng. Lời nhắc của lượt chạy (`FlowLeadBrief`) nói đúng hành vi này: server đăng comment khi lead chưa đăng, lead có thể tự đăng, và request luôn có đúng một comment cho mỗi dừng.
- **Điểm dừng người:** dừng 1 (`stop: 1`), dừng 3 (`stop: 3`), treo (`stop: "stalled"`) đặt request sang `blocked` kèm comment hệ thống có tiêu đề cố định; dừng 2 là vào bước `in_review` (`human_only`) kèm comment dừng 2 (sản phẩm đính kèm của task cuối, liên kết kết luận bàn giao, số vòng đã dùng, danh sách task và outcome). Mọi comment kèm nhắc người yêu cầu.
- **Task `blocked` và dừng 1:** ngay khi có task `blocked`, request sang `blocked` (`stop: 1`) nhưng engine **không hủy** task còn lại đang chạy. Comment dừng 1 điền `{missing}` bằng dòng đầu của comment mới nhất mà agent viết trên mỗi task `blocked` của vòng hiện tại (dòng `VERDICT: CHẶN - ...` của gate, mục thiếu của collector; cắt 300 ký tự) và `{reason}` bằng một câu cố định nêu task nào không làm tiếp được; dừng 1 do lead (`plan --blocked`) vẫn dùng `--missing` và `--reason` của lead. **`resume` của thành viên:** quay lại `blocked_from`. Dừng 1: giữ `rework`; chỉ đưa các task `blocked` của vòng hiện tại về `todo` (task đã đóng giữ nguyên). Với task `blocked` là gate, gate chạy lại trên chính bản nháp cũ, đọc được tệp mới của request nhưng không thấy bản nháp đổi; nếu nguồn mới phải vào bản nháp thì `resume` với `to_step` là bước soạn (API `{"to_step": "draft"}`, CLI `--to-step draft`; trên UI, nút Resume có một mũi tên mở lựa chọn **Quay về bước soạn**, gửi `to_step` bằng `rework_step` của state, chỉ hiện khi bước dừng có chuyển tiếp rework (mục 7.1). Dừng 3: đặt `rework = 0`, mở lại **toàn bộ** task của bước với `flow_round` mới. Dừng `stalled`: đưa task treo về `todo` và đặt lại bộ đếm thử.

### 5.5 Mô tả task do engine sinh

Mẫu cố định, không do lead soạn: mục tiêu (từ `Task.Instructions`), đầu vào (mã request và mã task bước trước, lệnh `multica attachment download`, **không** dán lập luận của người soạn), lệnh kết thúc (`multica issue comment add ... --attachment ...` rồi `multica issue status <id> done --outcome <approved|rejected|blocked>`), điều máy chủ sẽ từ chối (R1, R2), và với vòng sửa danh sách mã task gate bị `rejected` của vòng trước. Gate chỉ nhận liên kết tới **tệp đính kèm** của bản nháp.

### 5.6 Thời hạn, thử lại, phát hiện treo

Cấu hình trong `flow_config` (mặc định ở mục 3.4; các số là mặc định cần hiệu chỉnh bằng diễn tập, chưa đo):

| Tình huống treo | Phát hiện | Xử lý |
| --- | --- | --- |
| Task mở, không còn lượt chạy `queued/dispatched/running`, quá `task_timeout_minutes` | quét | Xếp lượt chạy mới (`attempt + 1`) với lời nhắc đổi cách làm |
| Task có lượt chạy chạy quá `task_hard_cap_minutes` | quét | Hủy lượt chạy, tính một lần thử |
| Hết `task_max_attempts` (đếm số hàng `agent_task_queue` của task) | quét | Task sang `blocked`, request sang `blocked` với `stop: "stalled"`, comment hệ thống |
| Request ở bước plan, lead không gọi `plan` quá `2 × task_timeout_minutes` | quét | Đánh thức lead lại (tối đa `task_max_attempts`) với lời nhắc đòi kế hoạch (`flowRetryLeadNote`, khác lời nhắc "đóng task" của task), rồi `stalled`. Lượt chạy đang hoạt động được đếm bất kể tạo trước hay sau `entered_at` (comment của người dùng đánh thức lead trước khi `resume` đặt mốc mới) |
| Request ở bước có task nhưng không có task nào mở và không ở điểm dừng | quét (hai lượt liên tiếp) | `Advance` lại; nếu vẫn không đổi thì `stalled` |
| Lượt chạy hỏng vĩnh viễn, `FailedRunLandingStatus` đặt task `blocked` (`service/task.go:6489`) | sự kiện `issue_child_event` | Request sang `blocked` (dừng 1 hoặc `stalled` tùy outcome) |

### 5.7 Khởi động lại máy chủ, giới hạn chi phí

- Không giữ trạng thái trong bộ nhớ. Khởi động lại: lease của scheduler hết hạn (`StaleTimeout`), quét kế tiếp tiếp quản (`AllowStaleReentry: true`); `Advance` bất biến nên an toàn.
- Giới hạn: 200 request mỗi lượt quét, 8 task mỗi bước, 12 bước, mỗi request tối đa ~10 ghi cho một lần chuyển bước; tổng số vòng ≤ N+1 lượt qua bước gate; nếu một lượt quét không kịp trong `RunTimeout`, lượt sau tiếp tục vì không có trạng thái dở dang ngoài các hàng đã commit.
- Công tắc: cờ `FF_FLOW_ENGINE` (tầng 1: guard và API, mặc định bật nhưng chỉ có tác dụng với workflow flow), `FF_FLOW_RUNNER` (tầng 2, mặc định **tắt**), và `issue_workflow.flow_auto` theo từng squad. Runner chạy cho request khi cả `FF_FLOW_RUNNER` bật và `flow_auto = true`.

## 6. Các tình huống lỗi

| Tình huống | Phát hiện | Khôi phục |
| --- | --- | --- |
| Tạo trùng task | Chỉ mục duy nhất `idx_issue_flow_task_once` (986) | `Advance` bỏ qua vi phạm; số task theo `(bước, task, vòng)` không bao giờ > 1 (tiêu chí B2) |
| Mất verdict (agent đóng task không outcome) | R1 từ chối; đường ghi hệ thống để lọt thì `Decide` tính `rejected` | Vòng sửa hoặc dừng 3; không bao giờ tự duyệt |
| Agent sập giữa task | Không còn lượt chạy mà task mở (mục 5.6) | Chạy lại tối đa `task_max_attempts`, rồi `stalled` |
| Hai runner/hai tiến trình cùng `Advance` | Khóa dòng + `revision` + chỉ mục duy nhất | Bên thua không ghi gì; bên thắng chạy xong |
| Squad có hai người điều phối (lead cũ tự tạo sub-issue) | R5 từ chối agent tạo sub-issue; lời nhắc lead có chế độ flow (S13) | Request flow không nhận con do agent tạo |
| Thành viên sửa trạng thái tay giữa chừng | R4 chỉ cho `canManageSquad`; engine nhận trạng thái mới nếu là bước hợp lệ | Hủy task mở của bước cũ (`cancelled`), ghi nhật ký `flow_manual_move`, đồng bộ `flow.step`; chuyển sang bước không thuộc workflow thì 400 `status_not_in_workflow`; chuyển sang `done/cancelled` thì hủy mọi task và đặt `closed_at` |
| Request không thuộc dự án | Định nghĩa lấy theo `flow.workflow_id`, không qua `ForIssue` (mục 4.4); thành viên ghi tay trạng thái bước flow trên request ngoài dự án bị kiểm với workflow Default (`CheckStatusOutsideProject`, `issue_workflow_entry.go:70`, `issue.go:3438`) | Engine ghi bằng `UpdateIssueStatus` nên không bị kiểm; thao tác kéo thả tay có thể nhận 400 `status_not_in_workflow`: giao diện hiện thông báo, thành viên dùng `resume` hoặc `detach` |
| Đổi tên squad | Workflow tra theo `id`, ghim vào `flow.workflow_id` | Không ảnh hưởng. Nâng cấp tìm squad theo `template_key` trước, tên sau (S14) |
| Template cập nhật giữa chừng | Request ghim `flow.workflow_id`; bản mới là hàng workflow mới (mục 8) | Request cũ chạy hết trên bản cũ; chỉ request mới dùng bản mới |
| Client cũ thiếu trường mới | Trường mới là tùy chọn trong mọi phản hồi (`.loose()`); ghi workflow thiếu trường giữ bản đã lưu (S4) | Client cũ ghi `PATCH` lên workflow template: 409 `workflow_managed_by_template`; kéo thả trạng thái request flow: 409 `flow_managed_request` với thông báo đọc được |
| Agent của task bị lưu trữ/đổi | `advance` kiểm agent trước khi mở task | `flow_agent_unavailable`: request `blocked`, comment nêu vai |
| Task bị thành viên hủy | Task `cancelled` không có outcome | Gate bị hủy tính `rejected`; task `work` bị hủy: bước coi như xong nhưng thiếu đầu ra, `requires attachment` không được kiểm lại, nên engine mở lại task bị hủy khi thành viên không chọn "bỏ qua" (xem `resume`) |
| Request bị hủy/xóa | `closed_at`/không còn hàng | Hủy mọi task mở; quét bỏ qua hàng có `closed_at` |
| Ghi trạng thái hệ thống ngoài `UpdateIssue` (mục 2 dòng 18) | `issue_child_event` + quét | `Advance` đọc trạng thái thật; nếu lệch bước thì đồng bộ như "thành viên sửa tay" |
| Runtime offline | Lượt chạy hỏng, task `blocked` (`task.go:6489`) | Dừng 1/`stalled` với nội dung nêu runtime |
| Lỗi giữa pha 1 | Giao dịch rollback | Không đổi gì; quét sau chạy lại |
| Cờ tắt giữa chừng | `FF_FLOW_RUNNER=off` | Runner dừng; request ở lại bước hiện tại; `child_done` nhận lại (mục 9.3) |

## 7. API

Tất cả cần `X-Workspace-ID`; quyền: đọc = thành viên workspace; ghi nêu từng dòng. Tác nhân agent xác thực bằng token tác vụ như mọi lệnh `multica` (`resolveActor`).

### 7.1 Endpoint

| Method và đường dẫn | Quyền | Body | Phản hồi |
| --- | --- | --- | --- |
| `GET /api/issues/{id}/flow` | thành viên | | 200 `IssueFlowState` (`rework_step`: bước mà chuyển tiếp rework của bước dừng đưa về, tức bước soạn; chỉ có khi request đang dừng, rỗng khi không có); 404 `flow_not_managed` |
| `POST /api/issues/{id}/flow/plan` | agent là lead của squad, hoặc `canManageSquad` | `{"switches": {"route": "...", ...}, "blocked": null \| {"missing": ["..."], "reason": "..."}}` | 200 `IssueFlowState` (đã chuyển bước hoặc dừng 1); 409 `flow_wrong_step`; 422 `flow_switch_invalid` |
| `POST /api/issues/{id}/flow/advance` | như `plan`, hoặc hệ thống | `{}` | 200 `{"state": IssueFlowState, "actions": [{"kind": "opened_tasks"\|"routed"\|"stopped"\|"noop", "step": "...", "tasks": ["..."]}], "pending": [{"task": "...", "issue_id": "...", "status": "..."}]}` |
| `PUT /api/issues/{id}/flow/switches/{key}` | như `plan` | `{"value": true\|"..."}` | 200 `IssueFlowState`; 409 `flow_switch_locked`; 422 `flow_switch_invalid` |
| `POST /api/issues/{id}/flow/resume` | chỉ thành viên `canManageSquad` | `{"to_step": "draft"?, "reset_rework": true?}` | 200 `IssueFlowState` |
| `POST /api/issues/{id}/flow/detach` | chỉ thành viên `canManageSquad` | `{"cancel_tasks": false}` | 200 `{"detached": true}`; xóa `issue.flow` và cột `flow_*` của task (task thành sub-issue thường), ghi `flow_detached` để quét không đưa request trở lại. Với `cancel_tasks: true` mọi task mở và lượt chạy của chúng bị hủy và lead không bị đánh thức vì việc hủy đó; với `false` các task chạy tiếp như sub-issue thường và khi chúng đóng, lead nhận `child_done` của chế độ cũ |
| `POST /api/issues/{id}/flow/start` | như `plan` | `{}` | 200 `IssueFlowState`; 409 nếu issue có cha, đã có con, hoặc squad không có workflow flow đang dùng |
| `PUT /api/issue-workflows/{id}/flow-auto` | admin/owner workspace | `{"auto": true}` | 200 `IssueWorkflowResponse` |
| `POST /api/issue-workflows` (mở rộng) | admin nếu có `flow_config` | thêm `flow_config`, `template_ref`, `steps[].tasks`, `steps[].human_only` | 201; 403 `flow_workflow_forbidden` |
| `GET /api/issue-workflows`, `GET .../{id}`, `GET /api/squads/{id}/workflows` (mở rộng) | thành viên | | thêm `flow_config`, `template_ref`, `flow_auto`, `squad_id`, `open_flow_requests` |
| `GET /api/squads`, `GET /api/squads/{id}` (mở rộng `SquadResponse`, `squad.go:26`) | thành viên | | thêm `active_workflow_id`, `active_workflow_name`, `workflow_count`, `flow_enforced` (bool: workflow đang dùng có `flow_config`) |

`IssueFlowState`:

```json
{
  "managed": true, "role": "request",
  "workflow_id": "uuid", "workflow_name": "Import & Export Desk · a1b2c3",
  "step": "gate", "rework": 1, "stop": null,
  "switches": {"route": "shipment_dossier", "customs_lookup": true},
  "auto": false, "entered_at": "RFC3339",
  "tasks": [{"issue_id": "uuid", "identifier": "MUL-12", "step": "gate", "task": "docs_gate",
             "round": 1, "status": "done", "outcome": "rejected", "assignee_id": "uuid", "closed_at": "RFC3339"}],
  "next": [{"id": "...", "label": "...", "to": "draft", "rework": true}]
}
```

`IssueResponse` (`issue.go`) thêm `outcome: string|null` và `flow: {"role": "request"|"task", "step", "task"?, "round"?, "stop"?}|null`.

### 7.2 CLI (`server/cmd/multica/`, tệp mới `cmd_issue_flow.go`)

`multica issue flow show|plan|advance|switch|resume|detach|start`; `multica issue status <id> done --outcome ...` đã có (`cmd_issue.go:728`), sửa văn bản trợ giúp nêu "máy chủ từ chối nếu thiếu". `scripts/agent-cli-command-names.txt` liệt kê tên **tệp thực thi của CLI agent** (`claude`, `codex`, `kiro-cli`, ...) cho bộ chặn test (`scripts/go-test-with-agent-cli-guard.sh`), không liệt kê lệnh con của `multica`: lệnh `multica issue flow` **không** thêm vào đó (đã đọc ở đợt hai).

### 7.3 Sự kiện thời gian thực

Thêm `EventIssueFlowUpdated = "issue_flow:updated"` ở `server/pkg/protocol/events.go` (cạnh dòng 148) và `packages/core/types/events.ts`; payload `{"issue_id": "..."}`; client chỉ vô hiệu hóa khóa truy vấn `issueFlowKeys.detail(wsId, issueId)` (AGENTS.md: sự kiện vá hoặc vô hiệu hóa Query, không lưu payload máy chủ vào Zustand).

### 7.4 Client (`packages/core`)

- Kiểu: `packages/core/types/issue-workflow.ts` (thêm `IssueWorkflowTask`, `human_only`, `flow_config`, `template_ref`, `flow_auto`, `squad_id`), `packages/core/types/issue-flow.ts` (mới, `IssueFlowState`), `packages/core/types/squad.ts` (thêm bốn trường).
- Zod: trong `packages/core/api/schemas.ts` (không phải `schema.ts`; `parseWithFallback` nằm ở `packages/core/api/schema.ts:38`): `IssueFlowStateSchema`, `IssueFlowAdvanceResponseSchema`, mở rộng `IssueWorkflowStepSchema` (hiện `.loose()` ở `schemas.ts` quanh dòng 671) và `IssueWorkflowSchema` (`:672`, `.loose()`), `SquadSchema`, `IssueSchema`. Mọi trường mới có mặc định (`flow: null`, `outcome: null`, `workflow_count: 0`, `flow_enforced: false`, `tasks: []`, `human_only: false`); `stop` lạ → `"stalled"`; `kind` lạ → `"work"`; outcome lạ giữ nguyên chuỗi; `tasks` null → `[]`. Gọi qua `parseWithFallback` trong `packages/core/api/client.ts`, có `endpoint` để log; không dùng `as T`.
- Client API **mới** (chưa có ở HEAD, mục 2 dòng 10): `listSquadWorkflows(squadId)` (`GET /api/squads/{id}/workflows`, trả `active_workflow_id`, `workflows`, `total`), `setSquadActiveWorkflow(squadId, {workflow_id})` (`POST /api/squads/{id}/workflow`), cùng trường `active_workflow_id` ở kiểu `Squad` (`packages/core/types/squad.ts:11`); mỗi phản hồi qua zod và `parseWithFallback`.
- Truy vấn: `packages/core/issue-workflows/` (hiện 8 tệp) thêm `squadWorkflowsOptions(wsId, squadId)`, `issueFlowOptions(wsId, issueId)`; khóa có `wsId`; mutation `plan/advance/resume/detach` chờ máy chủ rồi vô hiệu hóa (không cập nhật lạc quan, AGENTS.md).
- Test phản hồi sai bắt buộc (`packages/core/api/schemas.test.ts`): thiếu `flow`, `tasks` null, `stop` lạ, `kind` lạ, outcome lạ, `workflow_count` chuỗi, `squad` không có bốn trường mới.

### 7.5 Tương thích ngược (AGENTS.md "API Compatibility")

- Phản hồi chỉ thêm trường; `.loose()` giữ nguyên trường lạ.
- Ghi workflow: `resolveStepInputs` (`issue_workflow.go:558`) giữ `Tasks`, `HumanOnly` và `flow_config` đã lưu khi yêu cầu **không gửi** (con trỏ/`omitted`), theo mẫu `stepRouting(..., prev)` (`:588`). Test: PATCH của client cũ không làm mất trường mới (tiêu chí A13).
- Trình thiết kế hiện tại dựng lại bước chỉ từ trường đã biết khi lưu (đã kiểm, mục 2 dòng 29): vì vậy workflow có `template_ref` bị khóa ghi (409) và giao diện ẩn nút sửa/nhân bản cho workflow flow.
- Client cũ thấy request ở trạng thái tùy biến (`Collect`...) như SDLC đang có; thao tác kéo thả bị 409 với thông báo.

## 8. Nâng cấp và cập nhật Kit Việt

- **Nguồn (D18).** Mỗi squad có một manifest catalog `catalog/workflows/flow_<id squad>/template.json` (ví dụ `flow_kv_trade`), `kind: workflow`, `ui: false` (nội dung là tiếng Việt như phần còn lại của Kit Việt nên không cần bản dịch; các workflow `sdlc_auto` và `designer_*` cũng `ui: false`), với **khối mới `flow`** bên cạnh `workflow` và `designer`: `flow.squad` (id template squad, ví dụ `kv_trade`), `flow.config` (mục 3.4), `flow.steps` (bước; `status` dạng `{key}` hoặc `{name, category}` như khối `workflow` của `catalog/workflows/sdlc_auto`; `tasks[].role` là **khóa vai** của squad; `human_only`; chuyển tiếp). Viết tay. Công cụ catalog phải học khối này (S11): `scripts/catalog/lib/build-workflows.mjs` (`validateWorkflow` hiện báo `workflow block is missing` cho dạng lạ), `build.mjs` (sinh `packages/core/catalog/generated/flows.ts` và `server/internal/catalog/generated/flows.json`), `packages/core/catalog/types.ts`, `scripts/catalog/catalog.test.mjs`. Kiểm khi `catalog:check`: phong bì chung (id bằng tên thư mục, `^[a-z][a-z0-9_]{1,47}$`, semver), `flow.squad` là squad có thật, mỗi `role` thuộc `squad.roles` và không phải lead, mọi vai (trừ lead) được dùng ít nhất một lần, mỗi squad tối đa một manifest flow, và các bất biến mục 3.7 (bản `.mjs`). Mã Go kiểm lại bằng `ValidateFlow` trên `flows.json` (tiêu chí A19), nên hai bản kiểm không lệch. `rev` = 8 ký tự đầu của sha256 của JSON chuẩn hóa của khối `flow`, tính lúc build. **Mã sinh không commit.**
- **Vì sao không dùng chỗ khác (đã đọc mã).** Trường `squad.workflow` của manifest squad: `validateSquad` buộc nó trỏ tới workflow có khối `workflow`, `quickstart.ts:878` coi nó là workflow dựng cho dự án build, và 21 test Kit Việt khẳng định `template.workflow` là null (mục 2 dòng 22). Mẫu `designer_*`: không đặt được tên agent (`issueworkflow/templates.go`: "a template never names an agent") và squad không chạy được chúng. Thư mục `catalog/_sources/kitviet/flows/`: **đã loại** (D18), vì đó là đầu vào của generator Kit Việt, "không phải template" theo `catalog/README.md`, chỉ phục vụ Kit Việt và vẫn cần công cụ kiểm riêng. Đánh đổi của D18: phải sửa công cụ catalog (S11); đổi lại flow đi cùng cơ chế catalog có sẵn (kiểm tra, sinh mã lúc build, `contracts`), dùng lại được cho Research Lab, Content Studio, Sales Loop về sau và không đụng trường `workflow` của workflow dự án.
- **Hợp đồng.** Không khai `contracts` cho manifest flow: không có mã nào tìm một flow theo tên; quickstart tra theo `flow.squad`. Các hợp đồng `squad.sdlc`, `workflow.sdlc_auto`, `squad.planning`, `workflow.planning_auto`, `company.executive_office` (`server/internal/catalog/contracts.go`, `packages/core/catalog/contracts.ts`) không đổi; bài kiểm của chúng phải vẫn xanh.
- **Phiên bản bất biến.** Mỗi `rev` là **một hàng `issue_workflow` mới** (tên `<Tên squad> · <rev>` ≤ 64 ký tự, `template_ref = flow_<id squad>@<rev>`, `flow_auto` kế thừa bản đang dùng). Không sửa hàng cũ. Hàng workflow có `template_ref` bị khóa ghi (409 `workflow_managed_by_template`).
- **Quickstart.** Với template có manifest flow, quickstart: (1) đổi `role` thành `handler.id` bằng cách tra agent theo vai như `workflowSteps` (`quickstart.ts:237`); (2) `POST /api/issue-workflows` với `flow_config`, `template_ref` (quản trị workspace; tạo trạng thái dùng chung theo tên); (3) `POST /api/squads/{id}/workflow` (`adoptSquadWorkflow` đưa hàng vào thư viện của squad). Không dùng `POST /api/squads/{id}/workflows` vì nó chỉ sao chép từ `source_workflow_id` và không mang cột mới (mục 2 dòng 10). Trùng workflow nhận biết theo `template_ref`, không theo tên (hiện `quickstart.ts:881` theo tên).
- **Chuyển bản.** `upgradeKitvietSquad` (`upgrade.ts:238`) sau khi cập nhật agent/skill: (1) tìm squad theo `template_key` rồi theo tên (`upgrade.ts:284` hiện chỉ theo tên; kiểu `Squad` có `template_key`, `squad.ts:11-13`; `quickstart.ts:814` ghi nó); (2) nếu workflow đang dùng có `template_ref` cùng `rev` thì `current`; (3) khác `rev` mà `template_ref` thuộc cùng template thì tạo hàng mới rồi đặt làm đang dùng (không remap, mục 9.1), kết quả `upgraded`; (4) workflow đang dùng không do template quản lý thì `kept` và báo lý do. Chạy hai lần không đổi gì.
- **Request đang chạy.** Ghim `flow.workflow_id`; chạy hết trên bản cũ. Hàng cũ chỉ xóa được khi `open_flow_requests = 0` (chặn xóa `workflow_has_flow_requests`).
- **Bản sao của người dùng.** **Không có** trong đợt đầu. Không nhân bản, không sửa workflow flow; vì vậy không có chuyện hợp nhất bản sửa với bản mới. (Theo nguyên tắc có sẵn của `upgrade.ts`: "chỉ thay khi còn bằng một bản cũ của ta"; nếu sau này có bản sao thì áp ở mức cả workflow bằng `template_ref`.)
- **Văn bản agent.** Nút cập nhật Kit Việt (`useUpgradeKitvietSquads`, `packages/core/squads/mutations.ts:48`) vẫn là cách đưa instructions mới tới agent; bản `rev` chỉ đổi workflow. Sổ bản cũ `kitviet-previous-texts.ts` được thêm văn bản lead/agent bản trước khi đổi sang chế độ flow (S13); tệp này do `scripts/kitviet-update.mjs` ghi khi nâng upstream (`WRITE_PATHS`) nên thêm mục tay phải theo mẫu hiện có và bài kiểm `kitviet-previous-texts.test.ts`.
- **Lệch giữa workflow và instructions agent.** Có thể có khoảng thời gian workflow mới hơn instructions. Văn bản lead chế độ flow chỉ gọi lệnh (`flow show/plan/advance`) nên không phụ thuộc nội dung bước.

## 9. Giao diện, D11, hoàn tác

### 9.1 D11 (squad có flow được gán dự án)

Điều kiện thật nằm ở `squad_project.go:108`: squad **có** `active_workflow_id` thì không vào khối `EnsureSquadProjectWorkflow`, nên **không có 422**. Việc cần làm là bảo đảm phần còn lại đúng:

- `activeWorkflowOf` (`issueworkflow.go:662`) bỏ qua workflow có `flow_config` khi định tuyến issue thường: issue thường của squad trong dự án giữ workflow dự án (không hồi quy).
- `SetSquadActiveWorkflow` với workflow flow: `squadWorkflowPlan` trả kế hoạch rỗng và **không** gọi `remapSquadIssues` (workflow flow không quản issue thường).
- Request flow nằm trong dự án vẫn do flow quản (trạng thái theo bước); task có `flow_task` không bị `CheckStatus` của workflow dự án (vòng đời mặc định).
- Squad chưa có workflow đang dùng và không có vai SDLC vẫn nhận 422 như cũ.

### 9.2 Giao diện (`packages/views`, web và desktop chung; không route mới)

| Màn hình | Nội dung | Tệp (mới/đổi) |
| --- | --- | --- |
| Danh sách squad (tổng quan) | Cột **Workflows** (`SquadColumnKey` thêm `workflows` ở `packages/core/squads/stores/view-store.ts:36`, hiện mặc định): tên workflow đang dùng + số workflow; huy hiệu "Có thực thi" khi `flow_enforced`; "-" khi không có. Dữ liệu từ `SquadResponse`, không gọi thêm API | `packages/views/squads/components/squads-page.tsx` (đổi, cột ở khoảng `:527-535`), `squad-workflows-cell.tsx` (mới) |
| Chi tiết squad | Mục **Workflows** giữa Instructions và Recent work (`squad-detail-page.tsx`, quanh `:708-714`): danh sách thư viện (`GET /api/squads/{id}/workflows`): tên, huy hiệu đang dùng, `template_ref`/phiên bản, số bước/task, số request đang mở. Bấm một dòng mở **sheet** với đồ thị: mỗi bước là một thẻ (chip trạng thái, khóa chỉ người, danh sách task kèm loại gate/work, avatar agent, điều kiện `when` bằng lời, `requires`, chuyển tiếp kèm "quay lại tối đa N lần") | `squad-workflows-section.tsx`, `workflow-graph-sheet.tsx` (mới), `squad-detail-page.tsx` (đổi) |
| Hành động quản trị (chỉ `canManage`, cùng điều kiện ở `squad-detail-page.tsx:270-281`) | Đặt làm workflow đang dùng (dùng lại hộp thoại ánh xạ `workflow-mapping-dialog.tsx` khi workflow không phải flow); bật/tắt `auto` (tầng 2); xóa phiên bản cũ (có chặn); không có sửa/nhân bản | như trên |
| Chi tiết request | Mở rộng `IssueFlowCard` (`packages/views/workflow/issue-flow-card.tsx`, gắn ở `issues/components/issue-detail.tsx:4213`, hiện trả `null` khi issue không có dự án) thành bảng **Flow**: bước hiện tại, vòng, bảng task và outcome, mã dừng; nút **Resume** (mặc định không gửi bước; mũi tên mở **Quay về bước soạn** khi `rework_step` có giá trị)/**Detach** (chỉ `canManage`) | `issue-flow-state-panel.tsx` (mới), `issue-flow-card.tsx`/`issue-detail.tsx` (đổi) |
| Trạng thái rỗng/lỗi | Squad không có workflow: dòng "Chưa có workflow" (không mô tả thừa theo `conventions.mdx`); lỗi tải: dòng lỗi có nút thử lại; workflow giải mã lỗi: hiện tên và huy hiệu "Không đọc được" thay vì ẩn | các tệp trên |
| Tab Workflow | Ẩn nút sửa, nhân bản, xóa trên workflow có `template_ref`; chip "Do Kit Việt quản lý" | `packages/views/workflow/manage/*`, `designer/*` |

**i18n (7 locale: en, zh-Hans, zh-Hant, fr, es, ko, ja):** khóa mới ở `packages/views/locales/<locale>/squads.json` (`page.table.workflows`, `workflows.section_title`, `.empty`, `.error`, `.retry`, `.badge_active`, `.badge_enforced`, `.badge_template`, `.steps_count`, `.tasks_count`, `.open_requests`, `.set_active`, `.auto_on`, `.auto_off`, `.sheet.human_only`, `.sheet.optional_when`, `.sheet.requires_outcome`, `.sheet.requires_attachment`, `.sheet.kind_gate`, `.sheet.kind_work`, `.sheet.rework_limit`), `issues.json` (`flow.panel_title`, `flow.step`, `flow.round`, `flow.stop.1`, `.3`, `.stalled`, `flow.outcome.approved`, `.rejected`, `.blocked`, `.done`, `.none`, `flow.resume`, `flow.detach`, `flow.detach_confirm`, `flow.errors.<mã lỗi mục 4.3>`) và `workflow.json` (`manage.managed_by_template`). Quy tắc số nhiều: ngôn ngữ CJK không có khóa `_one` (ghi nhận ở `docs/engineering/workflow-tab.md` mục 8, kiểm bằng `packages/views/locales/parity.test.ts`). Nội dung workflow Kit Việt là tiếng Việt và không dịch.

**Mobile (quyết định).** Không đổi trong đợt này. Lý do: `apps/mobile/AGENTS.md:7` cho mobile sở hữu UI/API riêng và không có màn hình quản lý squad (`git grep -li squad -- apps/mobile` chỉ thấy squad ở mention picker, chat, my-issues, hàng issue, bộ chọn người nhận và `data/api.ts`, không có màn quản lý squad). Request flow hiện trên mobile như issue thường ở trạng thái tùy biến (đang có với SDLC). Việc phải **kiểm** (không sửa nếu đạt): mobile chịu được trạng thái lạ và trường mới (chưa kiểm).

### 9.3 Hoàn tác

- **Tầng 2:** `FF_FLOW_RUNNER=off` hoặc `flow_auto=false` → runner dừng; `child_done` quay lại cho request không `auto`; lead gọi `advance` thủ công. Request đang chạy giữ nguyên bước.
- **Tầng 1:** `FF_FLOW_ENGINE=off` → guard và API flow trả 404/bỏ qua, workflow flow trở thành workflow thường không định tuyến issue; request flow có `flow` nhưng không được quản.
- **Xóa trạng thái đang chạy:** `POST /api/issues/{id}/flow/detach` từng request (hoặc kịch bản lặp của admin); sau đó lead dùng chế độ cũ nhờ văn bản lead hai chế độ (lead chạy `flow show`; 404 `flow_not_managed` thì theo quy trình cũ) – nên không cần hoàn tác văn bản.
- **Migration:** down 987, 986, 985 (mục 3.1) sau khi `detach` hết.

## 10. Quyết định D1 đến D20

"Trợ lý chọn" = người dùng ủy quyền; **phải được xác nhận trước khi bật tầng 2.** D18, D19 và D20 là lựa chọn cuối cùng của đợt ba (không còn mục nào để ngỏ); riêng việc tạo DB test (D19) là việc của người dùng, chặn bằng cổng G0 của kế hoạch. "Mặc định theo khuyến nghị" = người dùng chưa trả lời riêng, giữ khuyến nghị trước.

| # | Câu hỏi | Quyết định | Lý do | Nguồn | Xác nhận trước tầng 2? |
| --- | --- | --- | --- | --- | --- |
| D1 | Tên hiển thị | "Workflow" (en) ở giao diện; `flow` chỉ là tên mã (`flow_config`) | Định nghĩa chính là hàng `issue_workflow`, không còn khái niệm thứ hai | mặc định theo khuyến nghị | Không |
| D2 | Flow của người dùng có điều khiển lead không | Không có flow do người dùng tự viết đợt này; flow template được engine thực thi | Giữ phạm vi; tránh trình sửa `tasks` chưa kiểm | mặc định | Không |
| D3 | Sau CHẶN chạy lại | **Mọi gate của vòng** (quy tắc R9) | Một lần sửa có thể phá điều gate khác đã kiểm; khớp `kv-squad-lead` bước 6 | người dùng | Không |
| D4 | "Flow nhẹ" cho việc nhỏ | Không | Gate là lý do tồn tại của squad | mặc định | Không |
| D5 | Phạm vi | Chỉ Kit Việt (mục 12.3 là theo dõi) | Giảm rủi ro | mặc định | Không |
| D6 | Ẩn flow của pack theo squad | Không | Không có nhu cầu | mặc định | Không |
| D7 | Vai chạy cho E-commerce `order_report`, `fnb_regulation`, `pr_numbers_gate` | `order_report`: `ecom_intake` (đọc tệp xuất đơn; đã gắn skill `bao-cao-doanh-so-sku`, `phan-tich-huy-hoan-don-san`, `catalog/_sources/kitviet/squads/ecom.json`) tạo báo cáo đính kèm, `ecom_numbers_gate` tính lại số, `ecom_handover` bàn giao. `fnb_regulation`: task `?(regulation_check)` trong Collect, chạy song song với `fnb_reader`. `pr_numbers_gate`: gate `?(has_figures)` trong cả hai route PR | Mỗi vai đã có mô tả khớp (`ecom_numbers_gate`: kiểm số; `fnb_regulation`: tra quy định bằng web; `pr_numbers_gate`: tính lại số bằng script); không thêm agent mới; lead đặt công tắc ở plan | **trợ lý chọn** | **Có** (đặc biệt `order_report` vì giao việc viết báo cáo cho một vai collector) |
| D8 | Thứ tự với phiên `catalog/` | Bước engine (S1–S10) không phụ thuộc layout. Điều kiện "nguồn Kit Việt ổn định ở HEAD" đã đạt từ `9d3a9e081`: nguồn ở `catalog/_sources/kitviet/`, manifest squad ở `catalog/squads/kv_*`; tác giả hóa flow từ S11 theo D18 | Tránh viết hai lần | mặc định (đợt hai: điều kiện đã đạt) | Không |
| D9 | Sửa câu "lead giao sub-issue cho vai `<squad>_lead`" ở 15 squad (mục 1) | Có, trong S13, theo cách của D20 | Câu này mâu thuẫn với `kv-squad-lead`; số đếm đợt hai là 15 | mặc định | Không |
| D10 | Hiện workflow đang dùng của squad | Có (cột và mục Workflows, S15) | Yêu cầu ban đầu về tổng quan và chi tiết | mặc định | Không |
| D11 | Squad có flow được gán dự án | **Có.** Không cần đổi điều kiện 422; bảo đảm mục 9.1 bằng test (S10) | Điều kiện thật chỉ chạy khi squad chưa có workflow đang dùng (`squad_project.go:108`) | **trợ lý chọn** | **Có** (kiểm luật "làn" của hàng đợi khi chạy song song trong dự án, `issue.sql:806-860`) |
| D12 | Chạy Kit Việt trên engine | Có | Theo D13, D14 | người dùng | Không |
| D13 | Job nền chạy flow ở máy chủ | Có | | người dùng | Không |
| D14 | Hai tầng | Có; tầng 1 giao và diễn tập trước, tầng 2 sau cổng G1 | | người dùng | G1 |
| D15 | Squad thí điểm | **Trade** | Dữ liệu `catalog/squads/kv_trade/template.json` (7 vai gồm lead, `routines: []` dòng 52, không `expertReview`), `catalog/_sources/kitviet/squads/trade.json`, `KV_LEAD_NOTES.kv_trade` (`kitviet-agent-instructions.ts:445`; không sổ, không routine), pipeline `catalog/_sources/kitviet/upstream/xuat-nhap-khau/skills/day-chuyen-ho-so-lo-hang/SKILL.md:25-33` (1 route, 1 tùy chọn `1b`, 2 gate luôn chạy `3a`/`3b`, 1 drafter, 1 bàn giao). Legal (khuyến nghị cũ) có 2 route và routine; Education có gate điều kiện và chặn hồ sơ học sinh; Admin có sổ công văn | **trợ lý chọn** | **Có** |
| D16 | Bỏ `squad_flow` | Có; 32 flow cũ chỉ là dữ liệu đầu vào để viết manifest flow (D18; đợt một gọi là generator) (mục 12.2, nay 33 route) | | người dùng | Không |
| D17 | Thứ tự sau thí điểm | Mục 11.1 | Rủi ro thấp trước | **trợ lý chọn** | **Có** |
| D18 | Flow nằm ở đâu trong catalog | Mỗi squad một manifest `catalog/workflows/flow_<id squad>/template.json` (`kind: workflow`, `ui: false`) với khối `flow` mới, tra theo `flow.squad` (mục 8). Phương án `catalog/_sources/kitviet/flows/` bị loại | Đi cùng cơ chế catalog có sẵn (kiểm tra, sinh mã lúc build, `contracts`); dùng lại được cho Research Lab, Content Studio, Sales Loop; không đụng trường `squad.workflow` (bị `validateSquad`, quickstart và 21 test Kit Việt dùng cho workflow dự án build); `designer_*` không đặt được agent (mục 8) | **chọn bởi trợ lý, xác nhận trước tầng 2** | **Có** (S11 làm theo lựa chọn này; nếu bị bác thì làm lại S11, S12, S16, S23–S26) |
| D19 | Đường chạy test cần DB | Test DB chỉ chạy khi `DATABASE_URL` trỏ tới một database test riêng, tên **kết thúc `_test`** và **khác `multica`**; mọi lệnh cần DB đặt `DATABASE_URL` tường minh trong lệnh (không đọc `.env`) và chạy sau guard `scripts/require-test-database.sh` (S0); không dùng `make test`, `make check`, `make migrate-up`, `scripts/ensure-postgres.sh`. Tạo database test là việc người dùng tự làm (cổng G0), agent không chạy lệnh tạo DB | `Makefile` không có target test an toàn (mục 2 dòng 25); mọi gói DB rơi về `multica` khi `DATABASE_URL` trống; Go không đọc `.env` nên chỉ cần đặt biến tường minh | **chọn bởi trợ lý, xác nhận trước tầng 2** | **Có** (G0: người dùng xác nhận tên DB test và đã tạo; áp dụng cho S0, S1, S4 đến S10, S16, S18 đến S21, S27) |
| D20 | Cách sửa 15 câu tự giao việc | Thêm quy tắc vào `catalog/_sources/kitviet/rewrites.json` đặt **trước** `call-agent` (`:182`), khớp hai dạng upstream "gọi/giao `kv:lap-ke-hoach-dieu-phoi`" và "gọi agent `kv:lap-ke-hoach-dieu-phoi`"; không sửa `patches/*.json` hay từng skill (S13) | `rewrites.json` là nguồn duy nhất, sinh lại được; câu sinh ra từ quy tắc `call-agent` (`:182`) và `call-kv-agent` (`:188`) áp lên văn bản upstream trước bước giải tham chiếu; đếm lại bằng generator: 15 squad (đợt một ghi 14 vì sót `b2b`) | **chọn bởi trợ lý, xác nhận trước tầng 2** | **Có** (kết quả sinh lại được kiểm bằng test đếm bằng 0, S13) |

## 11. Tiêu chí chấp nhận

**Tầng 1** (mọi tiêu chí kiểm bằng test; "kịch bản" = agent giả đóng task qua API, không gọi agent thật):

| # | Tiêu chí |
| --- | --- |
| A1 | `PATCH` workflow có `template_ref` → 409 `workflow_managed_by_template` |
| A2 | Đóng gate không outcome → 422 `flow_outcome_required`, trạng thái giữ nguyên, `issue.outcome` NULL; có `approved` → 200 và `outcome = 'approved'` |
| A3 | Đóng task `requires attachment` thiếu tệp → 422 `flow_attachment_required` |
| A4 | Agent rời bước `human_only` → 403 `flow_human_only`; thành viên có quyền → 200 |
| A5 | Agent ghi trạng thái request flow qua `UpdateIssue` **và** qua batch → 409 `flow_managed_request` |
| A6 | Agent tạo sub-issue dưới request flow → 409 `flow_children_managed` |
| A7 | `advance` lần hai liên tiếp: `actions = [noop]`, số sub-issue và số comment của request không đổi |
| A8 | Bước 2 task: một lần `advance` mở đúng 2 sub-issue; đóng 1 thì bước còn treo; đóng cả 2 thì chuyển |
| A9 | Task có `when` sai không được mở; đặt công tắc đúng thì mở |
| A10 | Gate bị `rejected` hai lần: `draft` mở ở `flow_round` 1 và 2, mỗi vòng mở lại đủ gate (2 gate × 3 vòng = 6 sub-issue gate); lần `rejected` thứ ba → request `blocked`, `flow.stop = 3`, `flow.rework = 2` |
| A11 | Outcome `blocked` của task → request `blocked`, `flow.stop = 1`, comment có tiêu đề dừng 1 và nhắc người yêu cầu |
| A12 | Test vàng S0 (SDLC/Planning, workflow không flow) không đổi một ký tự |
| A13 | Client cũ (PATCH không gửi `tasks`, `human_only`, `flow_config`) không làm mất các trường đó |
| A14 | Gán squad có flow vào dự án → 200 không 422; issue thường của squad trong dự án giữ trạng thái và workflow dự án |
| A15 | Test phản hồi sai của zod (mục 7.4) xanh |
| A16 | Kịch bản Trade: request → `in_review` với đúng số sub-issue (1 hoặc 2 Collect, 1 Draft, 2 Gate, 1 Handover) và 0 sub-issue do agent tạo; sau `done` bởi thành viên `closed_at` được đặt |
| A17 | Mọi migration có down; `TestEveryConcurrentUpBuildHasCleanup` xanh với hai dòng mới |
| A18 | `pnpm catalog:check` (hay `node scripts/catalog/cli.mjs check`) xanh với manifest flow của Trade, và bài kiểm catalog từ chối: `flow.squad` lạ, vai không thuộc squad, vai lead trong task, vai (trừ lead) không được dùng, hai manifest cho một squad, chu trình không `rework`, gate thiếu `requires outcome` |
| A19 | Test Go `TestCatalogFlowsPassValidateFlow` chạy `issueworkflow.ValidateFlow` trên mọi flow trong `flows.json` và xanh; `rev` ổn định khi đổi thứ tự khóa JSON |
| A20 | Không tệp sinh nào được commit: `git ls-files packages/core/catalog/generated server/internal/catalog/generated` chỉ trả `server/internal/catalog/generated/.gitkeep` |
| A21 | `bash scripts/require-test-database.test.sh` xanh: guard từ chối `DATABASE_URL` trống, tên `multica`, tên không kết thúc `_test`, DSN dạng `key=value`, tham số `dbname=`/`database=` trong URL; chấp nhận `<tên>_test`; guard không mở kết nối và không đọc tệp |

**Tầng 2** (thêm):

| # | Tiêu chí |
| --- | --- |
| B1 | `Advance` chạy lại không tạo issue, comment, hay đổi trạng thái (đếm trước/sau = 0) |
| B2 | Hai `Advance` đồng thời trên cùng request: mỗi `(bước, task, vòng)` đúng một issue, không lỗi lộ ra người dùng |
| B3 | Dựng lại `Handler` giữa chừng (mô phỏng khởi động lại): sau một lượt quét trạng thái hội tụ, không trùng |
| B4 | Task treo: quá `task_timeout_minutes` xếp lượt chạy mới; quá `task_max_attempts` thì task và request `blocked`, `stop = "stalled"` trong một lượt quét |
| B5 | `FF_FLOW_RUNNER=off`: quét không làm gì; `advance` thủ công vẫn chạy |
| B6 | Request `auto`: lead nhận 0 wakeup `child_done` giữa các bước (chỉ nhận đánh thức ở bước plan) |
| B7 | Quét 200 request giả trong < 45 giây (`RunTimeout`) |
| B8 | Thành viên sửa trạng thái tay: task bước cũ bị hủy, `flow.step` đồng bộ, nhật ký `flow_manual_move` |

## 12. Ánh xạ squad

### 12.1 Thứ tự triển khai (D17, do trợ lý chọn, thấp rủi ro trước)

Tiêu chí xếp hạng, tính từ dữ liệu `catalog/_sources/kitviet/squads/<id>.json`, `catalog/squads/kv_<id>/template.json` (cờ `expertReview`, `routines`) và ghi chú `KV_LEAD_NOTES`: (1) số route; (2) số task tùy chọn; (3) bước đặc biệt (routine, bước duyệt thông điệp chỉ người, sổ cố định); (4) dữ liệu cá nhân là lõi; (5) cờ `expertReview` (runbook mục 8d yêu cầu người trong ngành đọc trước khi bật). Rủi ro cao hơn thì xếp sau.

| Lô | Squad | Lý do xếp lô |
| --- | --- | --- |
| 0 (thí điểm) | **Trade** | 1 route, 1 task tùy chọn, 2 gate luôn chạy, không routine, không sổ cố định, không dữ liệu cá nhân, không cờ `expertReview` (xem D15) |
| 1 | B2B, Admin | 1 route, ≤ 2 tùy chọn, không routine, không cờ chuyên gia; Admin có sổ công văn nhưng nằm ngoài engine |
| 2 | Marketing, PR, E-commerce, Academic, Legal, Accounting, Real estate | 2 đến 3 route, có bước duyệt thông điệp chỉ người (Marketing, PR) hoặc routine (Legal, Accounting); không cờ chuyên gia; Accounting đòi Bash/Python |
| 3 | Talent, Education, Teaching, Hospitality, Office Ops | Dữ liệu cá nhân (ứng viên, học sinh, khách) hoặc mô hình hóa phải chọn (Office Ops, mục 12.2) |
| 4 | Health, Construction, F&B, Insurance, Banking, Factory | Có cờ `expertReview` (`"expertReview": true` ở dòng 8 của `catalog/squads/kv_{banking,construction,factory,fnb,health,insurance}/template.json`): người trong ngành phải đọc đầu ra trước khi bật |

Mỗi lô chỉ bắt đầu sau khi lô trước qua diễn tập kịch bản (tiêu chí mục 11) và người dùng xác nhận.

### 12.2 Dữ liệu flow cho 21 squad (đầu vào của 21 manifest `flow_<squad>`)

Tên vai viết tắt bỏ tiền tố squad (tiền tố là phần trước `_lead` của vai lead trong manifest, ví dụ `mkt_`, `acct_`, `acad_`, `tm_`). `?(công tắc)` là task/gate tùy chọn, `when` đọc công tắc đó. Bước trung gian `Approval` là bước `human_only` không có task (thành viên duyệt rồi chuyển bước); chuyển tiếp ở `Collect` rẽ qua `Approval` trừ khi công tắc `message_approved = true`. Hai bước tuần tự dùng thêm trạng thái `Screen` (Talent) và `Analyze` (Banking).

| Squad | Route (chuỗi sau Plan) | Công tắc |
| --- | --- | --- |
| Trade | `shipment_dossier`: Collect[docs_reader, customs_research?(customs_lookup)] → Draft[crosscheck_drafter] → Gate[docs_gate, customs_gate] → Handover | `customs_lookup`, `export` |
| B2B | `quote_pack`: Collect[history, company_lookup?(company_lookup)] → Draft[quote_drafter] → Gate[commit_gate, data_gate, tone_gate, numbers_gate] → Handover | `company_lookup`, `export` |
| Admin | `official_document`: Collect[files, format_lookup?(format_lookup)] → Draft[drafter] → Gate[format_gate, numbers_gate?(has_figures)] → Handover | `format_lookup`, `has_figures`, `export` |
| Marketing | `campaign`: Collect[scout, insight] → Approval → Draft[content, ads] → Gate[claim_gate, voice_gate, numbers_gate] → Handover | `message_approved`, `export` |
| PR | `article_check`: Collect[source, transcript] → Approval → Draft[writer] → Gate[fact_gate, ethics_gate, numbers_gate?(has_figures)] → Handover; `release_qa`: Collect[source] → Approval → Draft[release] → cùng Gate | `route`, `message_approved`, `has_figures`, `export` |
| E-commerce | `listing`: Collect[intake, rival_scout] → Draft[listing_drafter] → Gate[policy_gate, margin_gate] → Handover; `aftersale`: Collect[intake] → Draft[cskh_drafter] → Gate[policy_gate, numbers_gate?(has_figures)] → Handover; **`order_report` (D7)**: Collect[intake, kèm tệp báo cáo] → Gate[numbers_gate] → Handover (bỏ Draft) | `route`, `has_figures` |
| Academic | `chapter_draft`: Collect[reader, finder?(web)] → Draft[drafter, matrix] → Gate[citation_gate, integrity_gate] → Handover; `chapter_review`: Collect[reader, finder?(web)] → Gate[citation_gate, integrity_gate] → Handover | `route`, `web` |
| Legal | `contract_review`: Collect[reader, sources?(legal_sources)] → Draft[drafter] → Gate[reading_gate, citation_gate?(has_citations)] → Handover; `validity_review` (routine): Collect[validity] → `in_review` | `route`, `legal_sources`, `has_citations`, `export` |
| Accounting | `month_close`: Collect[collector, tax_research?(tax_lookup)] → Draft[reconciler] → Gate[voucher_gate, numbers_gate] → Handover; `tax_letter`: Collect[collector, tax_research?(tax_lookup)] → Draft[tax_drafter] → Gate[tax_gate, numbers_gate] → Handover; `tax_calendar` (routine): Collect[tax_research] → `in_review` | `route`, `tax_lookup`, `export` |
| Real estate | `listing_pack`: Collect[reader, area_research?(area_research)] → Draft[listing_drafter] → Gate[listing_gate, planning_gate?(mentions_planning)] → Handover; `loan_plan`: Collect[reader] → Draft[loan_drafter] → Gate[loan_gate] → Handover | `route`, `area_research`, `mentions_planning` |
| Talent | `screening`: Collect[reader, market] → Screen[screener] → Draft[drafter] → Gate[bias_gate, data_gate] → Handover | `export` |
| Education | `periodic_test`: Collect[curriculum] → Draft[test_drafter] → Gate[answer_gate, student_gate] → Handover | `export` |
| Teaching | `teaching_material`: Collect[sources] → Draft[drafter] → Gate[source_gate, student_gate?(student_facing)] → Handover; `class_records`: Collect[records] → Draft[drafter] → Gate[student_gate, numbers_gate] → Handover (plan bị chặn nếu `records_anonymised` ≠ true) | `route`, `student_facing`, `records_anonymised` |
| Hospitality | `tour_quote`: Collect[intake, destination?(web)] → Draft[tour_drafter] → Gate[quote_gate] → Handover; `guest_text`: Collect[intake] → Draft[hotel_drafter] → Gate[promise_gate] → Handover | `route`, `web` |
| Office Ops | `office_job`: Collect[files?(has_source_files)] → Draft[drafter] → Gate[numbers_gate?(has_costs)] → Handover (**một** task `drafter` xử lý mọi việc trong 9 việc; trước đây mỗi việc một sub-issue, thay đổi này được ghi nhận, `foreach` là hạng mục theo dõi) | `has_source_files`, `has_costs`, `export` |
| Health | `health_document`: Collect[reader, regulation?(web_regulation), research?(web_research)] → Draft[drafter, table?(needs_table)] → Gate[scope_gate, privacy_gate] → Handover | `web_regulation`, `web_research`, `needs_table` |
| Construction | `site_dossier`: Collect[reader, standards] → Draft[site_drafter, quantity?(has_money_table)] → Gate[quantity_gate?(has_money_table), safety_gate] → Handover | `has_money_table`, `export` |
| F&B | `new_dish`: Collect[reader, regulation?(regulation_check) **(D7)**] → Draft[drafter] → Gate[allergen_gate, margin_gate] → Handover | `regulation_check`, `export` |
| Insurance | `proposal`: Collect[gather] → Draft[proposal] → Gate[advice_gate] → Handover; `claim_guidance`: Collect[claim_reader] → Draft[claim_drafter] → Gate[advice_gate] → Handover | `route` |
| Banking | `credit_memo`: Collect[reader, industry] → Analyze[analyst] → Draft[memo_drafter] → Gate[credit_gate, advice_gate?(customer_facing)] → Handover | `customer_facing`, `export` |
| Factory | `kpi_report`: Collect[reader, standards] → Draft[kpi_drafter, doc_drafter] → Gate[numbers_gate, safety_gate] → Handover; `quality_record`: Collect[reader] → Draft[doc_drafter] → Gate[safety_gate] → Handover | `route`, `export` |

Tổng: 21 workflow (một workflow đang dùng mỗi squad), 33 route (kể cả `order_report`), 2 route routine (`validity_review`, `tax_calendar`). Vai mọi squad (trừ lead) phải xuất hiện trong ít nhất một route; test catalog chặn vai mồ côi (A18). Đợt hai đã kiểm bảng này bằng script đối chiếu với `squad.roles` của 21 manifest `catalog/squads/kv_*/template.json`: mọi khóa vai trong bảng thuộc đúng squad của nó, và vai duy nhất không thấy trong dấu ngoặc là `*_handover` (bước Handover cuối của mọi route). Chuỗi trên là thiết kế để người viết đối chiếu lại với skill `day-chuyen-*` và `KV_LEAD_NOTES` (các số dòng ở `kitviet-agent-instructions.ts:212-493` HEAD) trước khi chốt; riêng `guest_text`, `order_report`, `quality_record` và `aftersale` suy ra từ mô tả vai và ghi chú lead, không có bảng khâu của upstream (cần người viết xác nhận).

### 12.4 O2 và O5 cho 21 manifest (sau diễn tập 3)

Quy tắc, kiểm bằng `TestFlowManifests_RulesHoldForEverySquad` (bảng 21 squad, `server/internal/handler/flow_manifest_rules_test.go`):

- Task `work` ghi kết quả thành tệp (chỉ dẫn có "thành tệp") khai báo `requires: ["attachment"]`. Trước đây chỉ các task Draft, Analyze và Screen đã khai báo `attachment`. Bước Collect là đầu vào của bước soạn, nên một Collect đóng không có tệp làm bản nháp và gate không có gì để đọc.
- Mọi gate, kể cả gate bàn giao, có chỉ dẫn phân biệt `blocked --outcome blocked` (dừng 1) với `rejected` (vòng sửa). Bước có gate phải có chuyển tiếp tới `blocked`.
- Mọi `requires` thuộc {`outcome`, `attachment`}; gate luôn có `outcome`; task có `attachment` có chỉ dẫn đính kèm.
- Mỗi manifest được máy chủ tạo thành workflow (`TestFlowManifests_EveryManifestCompiles`, cùng đường với rehearsal).
- Hành vi qua engine: subtest `gate blocked on the sources stops at one` trong `rehearseSquad` (`flow_rehearsal_squads_test.go`) chạy cho mọi route có gate của 20 squad: gate `blocked` đưa request về dừng 1, không mở vòng sửa. Hai route không có gate (Legal `validity_review`, Accounting `tax_calendar`) bị bỏ qua.

Không đổi: task Draft/Analyze/Screen (đã có `attachment`); task `export` ở `in_review` (tệp xuất là sản phẩm cuối, nhưng không thuộc phạm vi Collect; để người quyết định). Chỉ dẫn của cả 20 manifest viết bằng tiếng Việt như Trade. Không có bản dịch: manifest workflow không có thư mục `i18n/` (`catalog/README.md`). Phiên bản tăng 1.0.0 → 1.0.1 cho mọi manifest đổi, theo quy tắc catalog.

| Squad (manifest) | Task Collect thêm `requires: ["attachment"]` | Gate có chỉ dẫn blocked/rejected | Ghi chú |
| --- | --- | --- | --- |
| Academic (`flow_kv_academic`) | reader, finder (`web`) | citation_gate, integrity_gate, handover | Chương bản thảo |
| Accounting (`flow_kv_accounting`) | collector, tax_research (`tax_lookup`) | voucher_gate, tax_gate, numbers_gate, handover | Month close và thư thuế dùng chung bước thu thập |
| Admin (`flow_kv_admin`) | files, format_lookup (`format_lookup`) | format_gate, numbers_gate, handover | Văn bản hành chính |
| B2B (`flow_kv_b2b`) | history, company_lookup (`company_lookup`) | commit_gate, data_gate, tone_gate, numbers_gate, handover | Bản chào giá |
| Banking (`flow_kv_banking`) | reader, industry | credit_gate, advice_gate, handover | Tờ trình tín dụng |
| Construction (`flow_kv_construction`) | reader, standards | quantity_gate, safety_gate, handover | Hồ sơ công trường |
| E-commerce (`flow_kv_ecom`) | intake, rival_scout (`web`) | policy_gate, margin_gate, numbers_gate, handover | `order_report` (D7) dùng cùng task intake: task đó phải đính kèm tệp báo cáo |
| Education (`flow_kv_education`) | curriculum | answer_gate, student_gate, handover | Đề kiểm tra định kỳ |
| Factory (`flow_kv_factory`) | reader, standards | numbers_gate, safety_gate, handover | Báo cáo KPI và hồ sơ chất lượng |
| F&B (`flow_kv_fnb`) | reader, regulation (`regulation_check`) | allergen_gate, margin_gate, handover | Món mới |
| Health (`flow_kv_health`) | reader, regulation (`web_regulation`), research (`web_research`) | scope_gate, privacy_gate, handover | Tài liệu y tế |
| Hospitality (`flow_kv_hospitality`) | intake, destination (`web`) | quote_gate, promise_gate, handover | Báo giá tour và nội dung gửi khách |
| Insurance (`flow_kv_insurance`) | gather, claim_reader | advice_gate, handover | Đề xuất và hướng dẫn bồi thường |
| Legal (`flow_kv_legal`) | reader, sources (`legal_sources`), validity (routine) | reading_gate, citation_gate, handover | Rà soát hợp đồng và hiệu lực |
| Marketing (`flow_kv_marketing`) | scout, insight | claim_gate, voice_gate, numbers_gate, handover | Chiến dịch |
| Office Ops (`flow_kv_officeops`) | files (`has_source_files`) | numbers_gate, handover | Một task drafter cho mọi việc; task thu thập chỉ mở khi có tệp nguồn |
| PR (`flow_kv_pr`) | source, transcript | fact_gate, ethics_gate, numbers_gate, handover | Bài báo và `release_qa` |
| Real estate (`flow_kv_realestate`) | reader, area_research (`area_research`) | listing_gate, planning_gate, loan_gate, handover | Tin bất động sản và kế hoạch vay |
| Talent (`flow_kv_talent`) | reader, market | bias_gate, data_gate, handover | Sàng lọc; task screener đã có `attachment` từ trước |
| Teaching (`flow_kv_teaching`) | sources, records | source_gate, student_gate, numbers_gate, handover | `class_records` chỉ chạy sau khi plan xác nhận ẩn danh |
| Trade (`flow_kv_trade`) | (không đổi: docs, customs đã có từ 0af9a08e8) | docs_gate, customs_gate (đã có), handover (thêm) | Phiên bản 1.0.0 → 1.0.1 |

Chỉ dẫn gate thêm vào (cùng một đoạn cho mọi gate): nếu mọi lỗi chặn nằm ở nguồn (tệp đầu vào của bước thu thập thiếu, mâu thuẫn hoặc chưa xác nhận) và bản nháp đã nêu đúng từng lỗi, bản nháp không sửa được; gate đóng bằng `blocked --outcome blocked` (dừng 1). Chỉ dùng `rejected` khi lỗi nằm ở bản nháp và người soạn sửa được. Các task có công tắc (`when`) chỉ cần tệp khi được mở.

### 12.3 Ngoài đợt đầu (theo dõi, kèm lý do)

| Squad | Lý do chưa làm | Cần thêm ở engine |
| --- | --- | --- |
| Research Lab | Vòng tìm và sàng lọc tối đa 5 vòng với giới hạn lấy từ dữ liệu; quyết định gate do công cụ Go tính trên tệp trạng thái đính kèm; verdict kiểm toán ba giá trị | Loại task `check` (hàm máy chủ có tên), giới hạn vòng từ thuộc tính request, ánh xạ `PASS WITH CONDITIONS` |
| Market Research | Request mở từ routine, 2 việc song song rồi critic, ≤ 3 proposal; không có Go tool đặc thù | Khởi động flow từ routine (có sẵn qua `start`), giới hạn proposal |
| Content Studio | Duyệt cuối bằng proposal của boss | Bước đợi quyết định proposal |
| Sales Loop | 7 route, fan-out theo lane, công cụ `multica sales collect` | `foreach` theo danh sách lane |
| SDLC, BMAD Planning | Đã chạy trên engine ở mức issue việc; đổi mức request có nguy cơ hồi quy | Tham số hóa giới hạn vòng (mặc định 3) nếu cần |

## 13. Chưa kiểm

### 13.1 Đã kiểm ở đợt hai (chuyển khỏi "chưa kiểm", kèm bằng chứng)

1. **Trình thiết kế dựng lại bước khi lưu:** có, và làm mất trường lạ (`workflow-data.ts:14-26`, `workflow-designer-page.tsx:439`, `issue_workflow.go:558-592`; mục 2 dòng 29). Phương án khóa ghi workflow template và giữ trường khi vắng là cần thiết.
2. **Cờ tính năng:** mặc định do nơi gọi truyền; `FF_<KHÓA>` nhận `on/off/true/false/1/0/yes/no`, rỗng là tắt (mục 2 dòng 21).
3. **`IssueService.Create` với người nhận là agent ở `todo`:** xếp lượt chạy qua `maybeEnqueueOnAssign`, trừ khi trạng thái là backlog hay triage hoặc agent bị chặn; có khóa trùng tiêu đề (mục 2 dòng 17).
4. **Tệp đính kèm vào comment:** vẫn mang `issue_id` của issue chứa comment (`attachment.sql:96-102,200-214`); agent tải lên mang `uploader_type = 'agent'` (mục 2 dòng 30).
5. **`TestMigrationNumericPrefixesAreUnique`:** đỏ sẵn ở HEAD vì 16 tiền tố trùng (600–608, 610–616) ngoài ngoại lệ; bản sửa chưa commit của người khác nằm trong cây làm việc (mục 2 dòng 28).
6. **Đường tạo issue:** 8 qua `IssueService.Create`, 5 qua truy vấn trực tiếp; một móc duy nhất không đủ (mục 2 dòng 31), dẫn tới `EnsureFlowStarted` (mục 4.4).
7. **Đường dẫn nguồn Kit Việt sau `catalog/`:** `catalog/_sources/kitviet/` và `catalog/squads/kv_*`; nội dung sinh ra giống từng byte bản trước khi chuyển, trừ chú thích đầu tệp (mục 2 dòng 27). Số route/vai ở mục 12.2 khớp `squad.roles` của 21 manifest.
8. **Nhánh `Advance` ba pha, đối chiếu với mã hiện có (chưa chạy):** khóa dòng kiểu `FOR UPDATE` đã có tiền lệ (`LockSquadForWorkflowChange`, `squad.sql:32`; `LockIssuesForDuplicateMark`); `UpdateIssueStatus` là câu ghi trạng thái hệ thống và không có `expected_revision` (sửa mục 5.3); tạo task qua `IssueService.Create` có giao dịch riêng, bọc lỗi bằng `%w` nên nhận được vi phạm chỉ mục duy nhất (mục 5.3); `finish()` của hàng đợi sự kiện chạy trước khi dispatch nên móc `Advance` đặt ở `Handler.processChildEvents` và dựa vào job quét để thử lại (mục 5.2).
9. **Thứ tự áp dụng khi sinh Kit Việt:** bản vá, rồi quy tắc `rewrites.json` theo thứ tự mảng, rồi giải tham chiếu (`generate-kitviet-pack.mjs:335-337`); Go không đọc `.env` (không có `godotenv` trong `server/`); `cmd/migrate` rơi về `multica` khi `DATABASE_URL` trống (`main.go:933-936`).
10. **`pnpm catalog:check` ở HEAD:** chỉ đọc tệp (chỉ `build` và `new` ghi), kết quả `catalog ok: 181 templates`, 3 cảnh báo về skill vendored.
11. **Đếm câu tự giao việc:** 15, không phải 14 (mục 1); cơ chế sửa ở D20.

### 13.2 Còn chưa kiểm

1. Chưa chạy mã Go hay TS của engine mới (chưa có), chưa chạy test nào của repo, chưa chạy agent. Đợt hai chỉ chạy hai lệnh không ghi vào repo: `node scripts/catalog/cli.mjs check` và `node scripts/generate-kitviet-pack.mjs --out <thư mục tạm>`. Mọi khẳng định về hành vi đến từ đọc mã ở HEAD.
2. Hành vi khi chạy của `Advance`: khóa dòng, bắt vi phạm chỉ mục, thử lại sau sập. Chỉ mới đối chiếu với mã (mục 13.1 điểm 8); các tiêu chí B1 đến B3 là bằng chứng cần có.
3. Mobile có chịu được trạng thái lạ và trường mới (chưa đọc `apps/mobile` đủ để kết luận).
4. Các số mặc định (`task_timeout_minutes` 120, `task_hard_cap_minutes` 240, `task_max_attempts` 2, 200 request mỗi lượt quét) chưa đo; diễn tập phải hiệu chỉnh.
5. Số route/vai ở mục 12.2 suy từ dữ liệu HEAD. Đã đọc lại bảng dây chuyền của Trade (`SKILL.md:25-33`); chưa đọc lại từng skill `day-chuyen-*` cho 18 squad còn lại. Bốn route `guest_text`, `order_report`, `quality_record`, `aftersale` suy ra từ mô tả vai và ghi chú lead, cần người viết xác nhận.
6. Khả năng chịu tải của chỉ mục trên bảng `issue` lớn.
7. Công cụ catalog chưa học khối `flow` (S11): chưa viết, chưa đọc hết `scripts/catalog/catalog.test.mjs` để biết cách thêm bài kiểm mới; `server/internal/catalog/catalog.go` cần thêm `FlowsJSON()` tương tự `WorkflowsJSON()`.
8. Tác động của `EnsureFlowStarted` lười (mục 4.4): khoảng trống bảo vệ trước lần gọi đầu của lead chưa đo.
9. Vị trí chặn `child_done` trong `dispatchSystem` và truy vấn `IsFlowRequestAuto` chưa viết; B6 là bằng chứng cần có.
10. `e2e/workflow.spec.ts` (tab Workflow, trình thiết kế) có thể bị S15 chạm; chưa đọc hết, chưa chạy (cần máy chủ và DB; chỉ chạy nếu người dùng cho phép, trên DB test của D19).
11. Quy tắc `rewrites.json` của D20: cách sửa đã chọn nhưng chưa viết; lời thay thế phải đọc được với các đuôi đã quan sát trong 15 squad (" lập kế hoạch", " lập kế hoạch rồi người dùng duyệt", " lập kế hoạch thay vì tự viết", " lập comment Kế hoạch của lead trên issue gốc", "."); chưa chạy generator với bản sửa. Hai dạng upstream được đếm bằng grep ở `catalog/_sources/kitviet/upstream` (dạng "gọi/giao `kv:…`" ở 10 tệp, dạng "gọi agent `kv:…`" ở 8 tệp, các tệp có thể trùng nhau); số squad bị ảnh hưởng (15) là số đếm trên đầu ra sinh ra.
12. Guard `scripts/require-test-database.sh` (D19) chưa viết, `scripts/test-go.sh` với `DATABASE_URL` tường minh chưa chạy thử, và `testutil.IsolatedDB` cần quyền CREATEDB của vai trong DSN test: người dùng chấp nhận quyền này khi xác nhận G0.

## 14. Sửa các trích dẫn và khẳng định cũ

| Cũ | Đúng |
| --- | --- |
| `handler/issueworkflow.go`, `server/internal/handler/issueworkflow.go`, `server/internal/issueworkflow/issue_workflow_entry.go`, `server/internal/service/issue_workflow_entry.go` (không tồn tại) | `server/internal/issueworkflow/issueworkflow.go` (engine thuần) và `server/internal/handler/issue_workflow_entry.go` (đường ghi trạng thái) |
| Runner đặt ở `server/internal/service/flow_runner.go` | Điều phối đặt ở `server/internal/handler/flow_engine.go` vì sự kiện `issue:updated` cần `handler.IssueResponse` (`activity_listeners.go:60-62`) |
| Đếm vòng từ `activity_log` | Đếm lưu ở `issue.flow.rework` (nhật ký ghi bất đồng bộ) |
| `scheduler/jobs_issue_queue.go:16-28` | `:18-31` (hàm `IssueQueueSweepJob` ở dòng 18) |
| Hai cột mới và hai migration 985-986 | Tám cột (`issue` ×5, `issue_workflow` ×3) và ba migration 985-987; mỗi chỉ mục phải đăng ký ở `server/cmd/migrate/main.go:142` |
| "Gán dự án đòi workflow đang dùng hoặc SDLC" | Chỉ khi squad **chưa** có workflow đang dùng (`squad_project.go:108`) |
| "Squad Kit Việt không thể vào dự án" | Đúng chỉ khi chưa có workflow đang dùng; quickstart chưa đặt workflow nào |
| Zod trong `packages/core/api/schema.ts` | Schema ở `schemas.ts`; `parseWithFallback` ở `schema.ts:38` |
| `make test` mặc định an toàn (đợt một) | Mặc định `ENV_FILE=.env` trỏ DB `multica` dùng chung (`Makefile:5`) và `make test` chạy migrate trên DB đó (`Makefile:358-362`); đợt một đề nghị dùng tệp env riêng của worktree nhưng đó là tệp riêng của từng máy, nên đợt hai bỏ đề nghị này và đợt ba thay bằng D19 (dòng cuối) |
| Batch update không có guard | Đường batch `issue.go:5140` gọi `applyIssueWorkflow` với outcome rỗng: phải được phủ (R1, R4) |
| `kitviet-flows` 32 flow | 33 route (thêm `order_report`) trên 21 workflow |
| Khuyến nghị pilot Legal | Trade (D15) |
| `packages/core/squads/kitviet-skills/kv-squad-lead/SKILL.md:24` | `catalog/_sources/kitviet/skills/kv-squad-lead/SKILL.md:24` |
| `packages/core/squads/kitviet-skills-data/*.ts` (14 skill tự giao việc) | Không còn commit; sinh ra `packages/core/catalog/generated/kitviet/<squad>.ts`. Câu do `catalog/_sources/kitviet/rewrites.json:188` (`call-kv-agent`); đếm lại: 15 squad (thêm `b2b`) |
| `packages/core/squads/kitviet-squads/<id>.json`, `kitviet-templates/<id>.ts` | `catalog/_sources/kitviet/squads/<id>.json` và `catalog/squads/kv_<id>/template.json` (`expertReview` dòng 8; `routines` ở `kv_legal:57`, `kv_accounting:62`) |
| `banking.ts:110`, `construction.ts:114`, `factory.ts:101`, `fnb.ts:106`, `health.ts:124`, `insurance.ts:107` | `"expertReview": true` ở dòng 8 của `catalog/squads/kv_{banking,construction,factory,fnb,health,insurance}/template.json` |
| `legal.ts:83`, `accounting.ts:122` (`startPaused`) | `catalog/squads/kv_legal/template.json:62`, `kv_accounting/template.json:68` |
| `templates.ts:607,619,641` (`SquadTemplate`, `expertReview`, `workflow`) | `packages/core/squads/templates.ts:134,146,168`; dựng bởi `packages/core/catalog/squads.ts:52` |
| `templates.ts:851,1014` (squad có `workflow`) | `catalog/squads/sdlc_greenfield/template.json:16`, `catalog/squads/bmad_planning/template.json:16` |
| `quickstart.ts:812,876`; `upgrade.ts:73,83,242,285` | `quickstart.ts:814,878`; `upgrade.ts:69,79,238,284` |
| `research-lab-squad-lead/SKILL.md` mục 2 (gate) | `catalog/skills/builtin/research-lab-squad-lead/SKILL.md:54-68`, mục 3 "Follow the gate" |
| Nguồn flow là `kitviet-flows/<id>.json` do generator Kit Việt đọc; `template_ref = kv_<id>@<rev>` | Manifest `catalog/workflows/flow_<id squad>/template.json` với khối `flow` (D18); `template_ref = flow_<id squad>@<rev>` |
| Móc `Advance` trong `service.IssueWakeupService.processChildEvents` | `Handler.processChildEvents` (`handler/issue_child_event.go:14-15`) cộng job quét (mục 5.2) |
| Chống thắng đôi bằng `UPDATE ... WHERE revision` | Khóa dòng `FOR UPDATE` và điều kiện trên `flow`; `UpdateIssueStatus` không có `expected_revision` (mục 5.3) |
| `StartFlow` trong `IssueService.Create` "cho mọi đường tạo" | `EnsureFlowStarted` gọi từ nhiều nơi (mục 4.4): 5 đường tạo issue không qua `Create` |
| Thêm tên lệnh `multica issue flow` vào `scripts/agent-cli-command-names.txt` (nếu liệt kê lệnh con) | Danh sách đó là tên tệp thực thi CLI agent; không thêm |
| Đoạn chặn tên DB đọc từ tệp env của worktree (đợt một) | Makefile không có target test an toàn mặc định; đợt ba chốt D19 (mục 10) |
| Client web đã có hàm đặt workflow đang dùng của squad | Không có `listSquadWorkflows`, `setSquadActiveWorkflow`, `Squad.active_workflow_id`; thêm mới (mục 7.4) |
| `POST /api/squads/{id}/workflows` có thể tạo workflow flow | Chỉ sao chép từ `source_workflow_id`; dùng `POST /api/issue-workflows` rồi `POST /api/squads/{id}/workflow` (mục 8) |
| "Không có spec e2e cho squad hay workflow" | Có `e2e/workflow.spec.ts` (tab Workflow, trình thiết kế); không có spec cho squad |
| "`TestMigrationNumericPrefixesAreUnique` ghi nhận đỏ sẵn ở `workflow-tab.md`" | Đã tính lại từ `git ls-tree`: đỏ vì 16 tiền tố trùng 600–608, 610–616; bản sửa chưa commit của người khác (mục 2 dòng 28) |
| `pnpm generate:kitviet --check` làm bước kiểm trong kế hoạch | `--check` so với thư mục đã sinh nên chỉ có nghĩa sau `catalog build`; dùng `pnpm catalog:check` và `node --test scripts/generate-kitviet-pack.test.mjs scripts/catalog/*.test.mjs` (như `.github/workflows/ci.yml:44`) |
| Test Go gói `issueworkflow` chạy được không cần dựng catalog | `issueworkflow.Templates()` panic nếu chưa dựng (`templates.go`); phải chạy `node scripts/catalog/cli.mjs build --target go` trước `go test` |
| *Đợt ba:* D18, D19 và cách sửa 15 câu còn "chờ chọn" | D18 chốt manifest `catalog/workflows/flow_<id squad>`, bỏ `catalog/_sources/kitviet/flows/` (mục 8, S11, S16, S23–S26); D19 chốt DB test `_test` cộng guard S0 và `DATABASE_URL` tường minh, bỏ mọi đường `make` (mục 2 dòng 25, G0); D20 chốt sửa ở `rewrites.json` (S13) |
| *Đợt ba:* G0 là chỗ người dùng chọn cách test | G0 chỉ còn là chỗ người dùng xác nhận đã tự tạo DB test; cách test đã chốt ở D19 |
| *Đợt ba:* S13 chọn giữa `rewrites.json` và bản vá từng skill | Chỉ `rewrites.json`; quy tắc đặt trước `call-agent` (`:182`); thêm test đếm cụm bằng 0 cho 21 squad |
| *Đợt ba:* HEAD `ba189d733` | HEAD `4e8fc136c`, chỉ thêm hai tài liệu này; trích dẫn mã không đổi. Đã kiểm lại tồn tại ở HEAD: `catalog/_sources/kitviet/rewrites.json`, `catalog/squads/kv_trade/template.json`, `scripts/catalog/` (`cli.mjs`, `catalog.test.mjs`, `lib/`), `server/internal/handler/issue_child_event.go`, `server/cmd/migrate/main.go` (`concurrentIndexCleanups` ở `:142`) |
