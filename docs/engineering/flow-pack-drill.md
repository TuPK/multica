# Diễn tập bộ skill/agent: từ bộ tới workflow đang chạy (P28)

Báo cáo này ghi bốn bộ được diễn tập từ nhập bộ đến chạy hết một request, bằng test Go chạy trên DB thật và một lần chạy trình duyệt ở 1280px và 390px. Hướng dẫn vận hành nằm ở [flow-authoring-runbook.md](flow-authoring-runbook.md) mục 9. Tiêu chí N1 đến N10 nằm ở spec 12.3.

Không agent thật nào chạy: việc của agent do test làm thay (đính `flow.json`, báo kết quả việc), runtime của trình duyệt chỉ là một hàng trông như đang trực tuyến. Không cờ nào được bật trên workspace thật; các test tự bật cờ cho handler của chúng.

## 1. Bốn bộ

Test ở `server/internal/handler/flow_rehearsal_pack_test.go`, dữ liệu thử ở `server/internal/handler/testdata/pack_rehearsal/` (hai manifest `multica.flow-manifest/v1`; không nằm trong `catalog/`). Mỗi test cài bộ vào workspace test, đi qua API như người quản lý squad và dọn đúng những hàng nó tạo.

| Bộ | Agent | Skill | Bước của flow | Kết cục lần chạy | Test và ảnh |
| --- | --- | --- | --- | --- | --- |
| Bộ gốc `catalog/_sources/kitviet/upstream/ban-hang-online/` (định dạng thư mục agent kiểu Claude Code, không `skills:` ở agent, một skill `day-chuyen-len-san-pham-moi`) | 7 của bộ + 1 lead chung do lần cài tạo = 8 | 18 | 5 (`plan`, `collect`, `draft`, `check`, `handover`) và terminal `done` | Chạy thử (`trial = true`) đến `done`; `plan` 1, `collect` 1, `draft` 1, `check` 1, `handover` 1; `flow-insights?include_trial=true` đếm đúng 1 request, không kèm tham số thì 0 | `TestRehearsalPack_KitVietUpstreamRaw`; ảnh 36 đến 52 |
| Chỉ có skill (`skills_only`: `outline-first`, `clear-brief`, `data-check`) | 3 agent khai lúc nhập + 1 lead chung = 4 | 3 | 4 (`outline`, `write`, `review`, `sign_off`; hai việc làm, một cổng duyệt chéo, một bước người) | Chạy thật trên phiên bản đang dùng (không trial) đến `done`, mỗi bước vào 1 lần | `TestRehearsalPack_SkillsOnly` |
| Bộ gốc của Multica có `flow.json` (`native_bundle` cộng một agent `checker` và một cổng; bản `native_gated_flow.json`) | 3 (`planner` lead, `writer`, `checker`) | 2 (`outline`, `tone-check`) | 3 (`plan`, `write`, `check`) | Cổng từ chối một lần rồi chấp nhận: đến `done`, `write` vào 2 lần, `check` vào 2 lần, vòng `redo` quay 1 lần, `write` mở 2 issue việc | `TestRehearsalPack_NativeBundleWithFlow` |
| Một squad, hai dòng phiên bản (dòng của bộ và dòng `Brief flow`) | 3 | 2 | 3 và 3 (`brief`, `approve` ở dòng hai) | Request không chọn chạy phiên bản đang dùng; request có `flow_workflow_id` chạy phiên bản đã chọn và squad vẫn giữ phiên bản đang dùng; mỗi phiên bản đếm đúng request của mình | `TestRehearsalPack_TwoWorkflowsOneSquad` |

Chi tiết từng bộ:

- **Bộ gốc.** `flow.json` do test viết tay (`ban_hang_flow.json`) thay cho agent `flow_designer`: lead lập kế hoạch, hai việc thu thập, một việc soạn listing cần ba skill (`mo-ta-san-pham-shop`, `tieu-de-seo-san-tmdt`, `tinh-gia-ban-san`), hai cổng, vòng sửa `fix` tối đa 2 lần, ba điểm dừng, rồi bước bàn giao cho người. Agent soạn listing được gán ba skill bằng lựa chọn lúc nhập (`PackAgentChoice.skills`), vì bộ không khai skill cho agent. Đường đi: nhập (định dạng 3) → lead chung → agent đính `flow.json` → `import-proposal` → kiểm không lỗi → mô phỏng → xuất bản không ack bị `422 flow_ack_required` (cần `proposal_reviewed`) → xuất bản có ack → chạy thử.
- **Chỉ có skill.** Bản nháp trắng dựng qua API với hai việc làm, một cổng duyệt chéo, một bước người, dùng `Task.skills`. Kiểm thêm R09: việc nêu skill mà agent được gán không có bị từ chối lúc xuất bản (`flow_draft_invalid`, vấn đề `skill_not_attached`).
- **Bộ gốc của Multica.** Bản nháp sinh từ `flow.json`, vai gắn theo khóa agent mà bộ khai (không theo tên agent trong workspace), kiểm không lỗi, xuất bản nguyên trạng.
- **Hai dòng.** Chọn phiên bản không đổi phiên bản đang dùng của squad.

## 2. Ảnh chụp của lần chạy trình duyệt

Spec `e2e/pack-rehearsal-drill.spec.ts`, bộ gốc `ban-hang-online` đọc từ đĩa và gửi dưới dạng zip, đi từ hộp thoại nhập đến phiên bản đã xuất bản. Ảnh nằm ở `/home/av-server/.cache/claude-delegate/20261113-140138-feature/p21-screens/` (ngoài repo), đánh số tiếp từ 36.

| Ảnh | Nội dung |
| --- | --- |
| `36-upstream-preview-1280` | Xem trước 7 agent và 18 skill, **Install** tắt vì chưa chọn lead |
| `37-upstream-ready-1280` | Đã chọn **Use a general leader**, tick ba skill cho agent soạn listing, đặt tên squad: **Install** bật |
| `38-upstream-preview-390` | Cùng hộp thoại ở 390px, các hàng xếp dọc |
| `39-upstream-install-390` | Nút **Install** ở 390px |
| `40-upstream-result-390` | Kết quả cài: **Installed** và liên kết **Design the workflow** |
| `41-squad-no-workflow-1280` | Trang squad chưa có workflow |
| `42-draft-editor-1280` | Bản nháp nhập từ manifest, 5 bước |
| `43-draft-graph-1280` | Đồ thị của bản nháp (canvas rộng hơn khung 1280px: ba bước đầu hiện, dùng **Fit** để xem hết; không đổi trong đợt này) |
| `44-roles-bound-1280` | Năm vai gắn cho năm agent của bộ |
| `45-check-1280` | **Check** không còn lỗi |
| `46-simulate-1280` | Hộp thoại mô phỏng: kết thúc ở `terminal: done`, đường đi 5 bước; các ô chọn kết quả nằm gọn trong hộp thoại (xem lỗi 1) |
| `47-publish-dialog-1280` | Hộp thoại xuất bản với các ack đã tick |
| `48-published-1280` | Squad có phiên bản `v1` đang dùng |
| `49-published-390` | Trang squad ở 390px |
| `50-draft-editor-390` | Trình soạn ở 390px |
| `51-draft-graph-390` | Đồ thị dạng danh sách ở 390px |
| `52-trial-dialog-390` | Hộp thoại **Start a trial run** ở 390px |

Spec còn kiểm bằng mã: không cuộn ngang ở mọi cỡ, `preview` trả 7 agent và 18 skill, `commit` trả 8 agent và 18 skill mới, `validate` không có lỗi mức `error`, mô phỏng đi qua hơn 3 bước, xuất bản trả 201. Spec dọn squad, agent và skill nó cài (hàm `afterEach`), nên chạy lại được.

## 3. Lỗi tìm thấy và cách sửa

1. **Hộp thoại Simulate cắt các ô chọn kết quả ở mép phải** (`packages/views/squads/flow-designer/simulate-dialog.tsx`). Tên việc dài (tiếng Việt) làm nhãn không co lại; `<fieldset>` mặc định có `min-inline-size: min-content`, nên ô chọn bị đẩy qua mép vùng cuộn và bị cắt. Lỗi có từ P21 (ảnh `08-simulate-1280` của đợt đó cũng bị cắt) và kiểm tra cuộn ngang cũ không thấy vì vùng tràn nằm trong vùng cuộn bên trong hộp thoại.
   - Test đỏ trước: spec drill kiểm mọi ô nhập của hộp thoại nằm trong vùng cuộn của nó. Kết quả đỏ: `simulate dialog at 1280px: SELECT sticks out of its scroll area, Expected: <= 808, Received: 833`.
   - Sửa: `min-w-0` cho hai `<fieldset>` và `shrink-0` cho các `<select>`, để tên việc bị cắt bớt bằng dấu chấm lửng thay vì đẩy ô chọn ra ngoài. Bốn dòng trong một file.
   - Xanh sau sửa: cùng spec đạt, ảnh `46-simulate-1280` có các ô chọn nằm trọn.
2. **Chưa có test nào cho ô chọn skill của việc (`Task.skills`) trong trình soạn** (tiêu chí N5). Không phải lỗi hành vi: thêm `packages/views/squads/flow-designer/task-card.test.tsx` (4 test, file mới, không sửa test có sẵn); tất cả đạt ngay khi thêm vì mã đã đúng.

Không thấy lỗi nào ở máy chủ. Bốn test Go đạt ngay lần chạy đầu: chúng được viết đủ rồi mới chạy, nên **không ghi nhận được kết quả đỏ**. Để chứng minh chúng bắt được lỗi, đã thử hai biến đổi tạm rồi hoàn lại:

| Biến đổi tạm | Kết quả |
| --- | --- |
| Bỏ yêu cầu ack `proposal_reviewed` ở `flow_publish.go` | `TestRehearsalPack_KitVietUpstreamRaw` đỏ: kỳ vọng 422, nhận 201 |
| Bỏ qua `flow_workflow_id` ở `issue.go` | `TestRehearsalPack_KitVietUpstreamRaw` đỏ (422 `flow_workflow_invalid`) và `TestRehearsalPack_TwoWorkflowsOneSquad` đỏ (request chạy phiên bản khác) |

Cả hai file đã trả về như cũ. Trong lúc làm, một lần chạy spec bị hỏng ở đăng nhập (`New Issue` không hiện) ngay sau khi sửa mã vì máy chủ dev đang biên dịch lại; chạy lại đạt, không do mã.

## 4. Lệnh đã chạy

DB thử `multica_engine_test`, DSN đặt rõ trước mỗi lệnh DB, mỗi lần một lệnh.

| Lệnh | Kết quả |
| --- | --- |
| `go test -count=1 -p 1 ./internal/handler -run TestRehearsalPack_ -v` | 4 test PASS |
| `pnpm exec playwright test e2e/pack-rehearsal-drill.spec.ts --project chromium` (máy chủ API cổng 18780, web cổng 3010, ba cờ bật) | `1 passed` (lần cuối 50.7s) |
| `go test -count=1 -p 1 ./internal/handler` | ok, 267.803s |
| `go test -count=1 -p 1 ./internal/service` | ok, 137.072s |
| `go test -count=1 -p 1 ./internal/scheduler` | ok, 1.985s |
| `go test -count=1 -p 1 ./internal/issueworkflow ./cmd/migrate ./cmd/multica ./internal/catalog ./internal/migrations ./internal/featureflags ./internal/pack ./cmd/server` | tất cả ok: issueworkflow 5.575s, cmd/migrate 96.789s, cmd/multica 19.511s, catalog 0.001s, migrations 2.134s, featureflags 0.001s, pack 0.021s, cmd/server 26.075s |
| `node scripts/catalog/cli.mjs build --target go` | `205 templates, 6 files written` |
| `pnpm catalog:build` | `205 templates, 48 files written` |
| `pnpm catalog:check` | `catalog ok: 205 templates` (ba cảnh báo về skill vendor chưa ghim, giấy phép) |
| `pnpm typecheck` | 10 tác vụ thành công |
| `pnpm lint` | 0 lỗi, 21 cảnh báo có sẵn (không có ở file đợt này) |
| `pnpm test` (một lần, không song song) | 8 tác vụ thành công; core 5224 đạt (1 bỏ qua), ui 430, docs 30, web 321, desktop 839, views 8632, không lỗi và không quá giờ |
| `git ls-files packages/core/catalog/generated server/internal/catalog/generated` | chỉ `server/internal/catalog/generated/.gitkeep` |
| `git diff --check` | sạch |
| `git status --short -- apps/landing apps/mobile` | không có gì |

Sau khi chạy: máy chủ dừng, `/tmp` dọn sạch, `apps/web/AGENTS.md` trả về bản ở HEAD, test Go không để lại hàng nào. Mỗi lần chạy spec trình duyệt để lại trong `E2E Workspace 0` của DB thử một squad đã lưu trữ (`DeleteSquad` chỉ lưu trữ) và một hàng `squad_pack_install` của nó, như các spec đợt G; agent đã lưu trữ và skill đã xóa.

## 5. Không chạy

- Agent thật và smoke test agent thật (`agentintegration`): không được phép.
- Bật `FF_FLOW_PACKS` hay cờ nào trên workspace thật: không làm; cờ chỉ bật trong tiến trình thử.
- Chạy một request trong trình duyệt đến cuối: runtime chỉ trông như trực tuyến, nên trình duyệt dừng ở hộp thoại chạy thử. Chạy hết thì do bốn test Go làm.
- `pnpm exec playwright test` toàn bộ: chỉ chạy spec của đợt này.
- Không có trang tài liệu người dùng nào mô tả workflow của squad (`workflows.*` nói về workflow trạng thái của dự án, `squads.*` chỉ nhắc workflow ở phần Quickstart và SDLC Auto), nên `apps/docs` không đổi.

## 6. Bản đồ N1 đến N10 (spec 12.3)

| # | Test đỏ-xanh nêu tên |
| --- | --- |
| N1 | `TestPackPreview_FourFormats`, `TestPackCommit_CreatesSquadAgentsSkillsAtomically` (handler); `TestNormalize_ClaudeAgents`, `TestNormalize_SkillsOnlyHasNoAgents`, `TestNormalize_MulticaPack`, `TestNormalize_NativeBundleKeepsLeaderAndFlow` (pack); với bộ gốc: `TestRehearsalPack_KitVietUpstreamRaw` |
| N2 | `TestPackPreview_WritesNothing`, `TestPackCommit_CreatesSquadAgentsSkillsAtomically` |
| N3 | `TestValidate_BrokenPacks` (pack), `TestPackCommit_BrokenPacksRejected`, `TestPackCommit_ScanBlockRejected_WarnNeedsAck` |
| N4 | `TestPackCommit_SameDigestIsNoop_OtherDigestConflicts`, `TestPackCommit_NamesThatAreTakenNeedAChoice` |
| N5 | `TestPackCommit_SkillsOnly_NoAgentsNoSquad`, `TestRehearsalPack_SkillsOnly`, `TestValidate_R09SkillNotAttached` (issueworkflow); giao diện: `task-card.test.tsx` ("offers the skills of the squad and puts the one that is clicked in Task.skills"), `pack-import-dialog.test.tsx` ("shows Add agent and One agent per skill for a pack that has skills and no agents") |
| N6 | `TestPackCommit_NativeFlowBecomesUnboundDraft`, `TestRehearsalPack_NativeBundleWithFlow`; vai gắn tay sau `import-proposal`: `TestRehearsalPack_KitVietUpstreamRaw` |
| N7 | `TestProposal_AgentCanValidateButNotWriteDrafts`, `TestProposal_ImportReadsLatestAgentAttachment`, `TestPublish_ProposalNeedsReviewedAck`, `TestRehearsalPack_KitVietUpstreamRaw` |
| N8 | `TestTrial_PinsVersionAndForbidsUnattendedRelease`, `TestCreateIssue_FlowTrialStartsARehearsalOnTheChosenVersion`, `TestRehearsalPack_KitVietUpstreamRaw` |
| N9 | `TestStart_OtherSquadOrNonFlowRejected`, `TestCreateIssue_FlowWorkflowIDIgnoredForAgentActor`, `TestCreateIssue_FlowWorkflowIDChoosesTheVersion`, `TestRehearsalPack_TwoWorkflowsOneSquad` |
| N10 | `TestFlowInsights_OnlyThisVersion_TrialExcludedByDefault` (handler), `TestInsights_EndsEdgesLoopsDurations` (issueworkflow), `TestRehearsalPack_TwoWorkflowsOneSquad` và `TestRehearsalPack_NativeBundleWithFlow` (số request và vòng của từng phiên bản) |

Tất cả nằm trong các lần chạy ở mục 4 và đạt.
