# SDLC specialist routing: implementation plan

Kế hoạch triển khai cho [sdlc-specialist-routing.md](sdlc-specialist-routing.md). Đọc spec trước;
các mục "6.x" và "Qx" dưới đây trỏ về spec.

Quy ước thực hiện (theo AGENTS.md):

- Commit nhỏ, mỗi task một commit, kiểu conventional (`feat(squads): ...`). Kết thúc commit bằng dòng
  `Co-Authored-By` theo hướng dẫn của phiên làm việc.
- PR chỉ mở vào `--repo TuPK/multica`, không bao giờ vào `multica-ai/multica`.
- Sau mỗi task chạy kiểm tra hẹp nhất có ích; cuối mỗi giai đoạn chạy bộ kiểm tra rộng hơn.
  Báo cáo đúng những gì đã chạy.
- Skill sửa trong `packages/core/squads/sdlc-skills/`, rồi `pnpm generate:sdlc-skills`. Không sửa tay `sdlc-skills-data.ts`.
- Test mặc định không được chạy agent CLI thật.

## Tổng quan giai đoạn

| Giai đoạn | Nội dung | Phụ thuộc | Đụng server | Rủi ro |
| --- | --- | --- | --- | --- |
| 1 | Skill, workflow template, instruction của agent, test chống lệch | không | không | thấp |
| 2 | Server mirror cho project do initiative tạo, đường nâng cấp squad đã tạo | 1 | có (Go, quickstart) | trung bình |
| 3 | Nâng cấp squad mua_ngon_ecommerce, diễn tập 6 kịch bản | 1, 2 | không | thấp, tốn thời gian chạy agent |
| 4 | (Tuỳ chọn) `blocker_class` có cấu trúc, bộ đếm vòng, thống kê | 3 và số liệu | có | cao, chỉ làm khi có buy-in |

Thứ tự đề xuất: 1 → 2 → 3. Giai đoạn 1 tự nó đã đổi hành vi của Lead khi squad được nâng cấp skill;
nhưng step `Environment` chưa có trong workflow của squad đã tạo cho tới Giai đoạn 2 và 3, nên **không
nâng cấp squad đang chạy thật trước khi xong Giai đoạn 2**.

---

## Giai đoạn 1: skill, template, test

**Trạng thái: xong (2026-11-07).** T1.1 đến T1.9 đã làm; T1.8 không có chỗ nào cần sửa. Test squad, views và core liên quan qua. `pnpm typecheck` và `pnpm lint` còn đỏ ở hai file không thuộc thay đổi này (`squad-quickstart.test.tsx`, `superpowers--brainstorming/scripts/helper.js`).

### T1.1 `sdlc-delivery-core`: báo blocker, quyền quyết định, phán quyết

File: `packages/core/squads/sdlc-skills/sdlc-delivery-core/SKILL.md`.

- Thêm mục "When you cannot proceed" với khối Blocker report (spec 6.3), danh sách class cố định.
- Thay đoạn "Decisions and assumptions" bằng bảng quyền quyết định (spec 6.2), giữ luật "chọn
  phương án không đổi schema/API/hành vi đã phát hành".
- Thêm mẫu Ruling comment vào "Report format" (spec 6.5); thêm `docs/sdlc/rulings.md` và
  `docs/sdlc/environment.md` vào "Rules that apply to all work" (đọc trước khi hỏi).
- Sửa "Stop only for what cannot be undone": thêm `capability` và `irreversible` là hai lớp dành cho người.
- Giữ nguyên toàn bộ luật cây thư mục bẩn và "never end a run in a waiting state".

Kiểm: `pnpm --filter @multica/core test -- sdlc-skills` (sau khi sinh lại ở T1.9).

### T1.2 `sdlc-squad-lead`: Step 2 khởi động, Step 7 định tuyến

File: `.../sdlc-squad-lead/SKILL.md` và `reference.md`.

- Step 2: thêm "Readiness checks" theo bảng spec 6.8; Plan A cũng chạy các kiểm tra này. Ghi quyết
  định và lý do bỏ qua vào comment kế hoạch (mẫu Plan comment thêm dòng `Readiness:`).
- Step 3: thêm đoạn ngoại lệ cho unblock issue (spec 6.4) ngay dưới luật `in_place`, nêu lý do: epic bị
  block đang đỗ nên không có hai run chạm cùng working copy.
- Step 5: bỏ cách xử lý "rerun once rồi chặn request" cho issue blocked; trỏ về Step 7.
- Step 7: viết lại thành quy trình 6.4 đến 6.7: phân loại, tra `rulings.md`/`environment.md`/knowledge,
  tạo unblock issue, đăng ký wakeup, kiểm tra phán quyết, cho tiếp tục, giới hạn vòng, lên người dùng
  kèm khuyến nghị. Giữ các luật: không bao giờ đổi AC để qua blocker; không đặt build done khi kiểm tra
  cần thiết chưa chạy; cây thư mục bẩn không phải blocker.
- Step 6: thêm kiểm tra phán quyết (Resume-check chạy được, không sửa AC trái thẩm quyền, không mâu thuẫn `rulings.md`).
- Thêm bước "kiểm tra trước epic" (spec 6.8) và "kiểm toán assumption" khi epic xong.
- Self-check cuối: thêm các dòng tương ứng.

### T1.3 `team-playbook.md` cho Lead

File mới: `.../sdlc-squad-lead/team-playbook.md`, đóng gói như `reference.md`
(test hiện có kiểm danh sách file của skill Lead: cập nhật nó thành `["reference.md", "team-playbook.md"]`).

Nội dung theo spec 6.9: bản đồ năng lực 8 vai trò, bảng định tuyến 6.1, bảng quyền quyết định 6.2,
mẫu unblock issue 6.4 và Ruling 6.5, bảng thay thế khi thiếu vai trò. Đây là bản gốc duy nhất của
bảng định tuyến (T1.9 gắn test vào nó).

### T1.4 `sdlc-devops-release`: hai chế độ

File: `.../sdlc-devops-release/SKILL.md`.

- Tách thành "Mode 1: Environment" (spec 6.10) và "Mode 2: Release" (nội dung hiện tại, giữ nguyên).
- Đặt "You do not", "Preconditions" và luật digest/approval dưới Mode 2 để chúng không chặn Mode 1.
- Mode 1 ghi rõ ranh giới: được sửa file hạ tầng/tooling/tài liệu; không sửa mã ứng dụng; không đụng
  credential thật; không deploy staging/production; không in bí mật; không dừng stack không do mình khởi động.
- Mẫu `docs/sdlc/environment.md` (cấu trúc: probe, dịch vụ và cổng, lệnh start/ensure/stop/reset,
  stack cô lập cho QA, giới hạn đã biết).

### T1.5 Ruling mode cho các chuyên gia

Thêm mục "Ruling mode (unblock issues)" (spec 6.11) vào:
`sdlc-product-analysis` (kèm: đây là kênh chính thức để sửa AC đã chấp nhận, qua `bmad-correct-course`
cho thay đổi cỡ Medium trở lên), `sdlc-architecture` (ADR Accepted-by-squad thay cho Proposed mãi mãi),
`sdlc-ux-design`, `sdlc-security-review` (thêm chế độ threat-model trước khi build), `sdlc-qa-verification`,
`sdlc-bug-fix`, `sdlc-code-review` (ý kiến thứ hai).

Sửa các câu mâu thuẫn: ở `sdlc-architecture` đổi "Take the default and record it; the squad does not
wait" cho các quyết định ràng buộc thành "ra ADR"; ở `sdlc-product-analysis` bỏ "never edit a
requirement an owner has accepted" thành "không sửa không ghi lý do; thay đổi đi qua Ruling mode".

### T1.6 `sdlc-development`, reviewer, QA: phát tín hiệu

- `sdlc-development`: khi nào tự quyết và khi nào chặn (trỏ bảng 6.2); đếm vòng trả lại và
  `rework-loop` từ lần thứ ba (spec 6.8); bước đầu kiểm tra runtime là chạy `env-ensure` nếu
  `docs/sdlc/environment.md` có; sửa "Backend the path needs" và "When a check fails" để phân loại
  `environment` rồi chặn đúng lớp khi ba cách khác nhau không dựng nổi, thay vì thử mãi.
  Cắt đoạn "chỉ phần phụ thuộc ngoài mới là blocked" thành tham chiếu bảng class.
- `sdlc-code-review` và `sdlc-qa-verification`: mỗi finding chặn có `Owner-role`; finding có owner
  khác developer thì set `blocked` với class thay vì REQUEST_CHANGES/NO GO thuần. QA: chạy
  `env-ensure` trước; dùng stack cô lập; khi môi trường thiếu thì Blocker report thay vì mô tả tự do.

### T1.7 Workflow template và briefs

File: `packages/core/squads/templates.ts`.

- `SquadRoleKey`/`SDLC_AUTO_WORKFLOW`: thêm step `{ name: "Environment" }`, handler `devops`,
  `next: stage_done`, `back: blocked`, đặt cạnh `Release prep`.
- `planningBrief`: đổi câu mở đầu để bao gồm unblock ruling (mô tả issue nói loại nào); thêm
  "follow your skill's Ruling mode for a ruling".
- `SDLC_BLOCKED_INSTRUCTIONS`: rút còn: "Load sdlc-delivery-core and sdlc-squad-lead and follow Blocked work. Read the
  Blocker report, route to the specialist who owns the class, verify the ruling, and resume. Do not leave it here and
  do not wait for a person." Giữ ý "không đổi AC" và "không đặt done khi kiểm tra chưa chạy".
- Brief của `in_review` và `qa`: thay "comment the exact command, its error, and the fix you need" bằng "write the Blocker report".
- `sdlc-agent-instructions.ts`: Lead (bỏ đoạn nhắc lại quy trình trùng skill, thêm "blocked work is routed to the specialist who owns the class"),
  DevOps (luật digest/approval thuộc Mode Release), Developer ("write a Blocker report").
  Cập nhật `sdlc-agent-instructions.test.ts` tương ứng.

### T1.8 Cập nhật tài liệu liên quan

`docs/engineering/digital-company.md` và chỗ nào khác mô tả luồng blocked của SDLC:
`grep -rn "blocked" docs/engineering | grep -i sdlc`. Chỉ sửa chỗ nào nay sai.

### T1.9 Sinh bundle và test chống lệch

1. `pnpm generate:sdlc-skills`.
2. Thêm test (trong `sdlc-skills.test.ts` hoặc file mới `sdlc-routing.test.ts`, đầu file `// @vitest-environment node`):
   - mọi skill chuyên gia có tiêu đề "Ruling mode";
   - parse bảng định tuyến trong `team-playbook.md`: mọi `start status` có trong `SDLC_AUTO_WORKFLOW`, mọi vai trò đích có
     trong `roles` của cả template Greenfield và Maintenance;
   - tập class trong `sdlc-delivery-core`, `team-playbook.md` bằng một hằng số `BLOCKER_CLASSES` trong test;
   - cập nhật phép kiểm danh sách file của Lead.
3. `templates.test.ts`: step `environment` có handler devops, `back: blocked`; thứ tự và thứ còn lại không đổi.
4. Chạy: `pnpm --filter @multica/core typecheck` và `pnpm --filter @multica/core test`.

**Điểm dừng Giai đoạn 1.** Chạy `pnpm typecheck && pnpm lint && pnpm test` ở gốc. Chưa nâng cấp squad đang chạy.

---

## Giai đoạn 2: server mirror và đường nâng cấp

**Trạng thái: đang làm.**

### T2.1 Server mirror cho project do initiative tạo

File: `server/internal/service/initiative_build.go` (+ `initiative_build_steps_test.go`).

Hiện `sdlcSteps` chỉ có backlog, todo, in_progress, in_review, qa, blocked; chưa có status lập kế hoạch
và `environment`. Lead của project do initiative tạo sẽ không có `start_status` hợp lệ cho chuyên gia
(`validateStartStatus` từ chối). Việc làm:

- Thêm các step lập kế hoạch và `environment` vào `sdlcSteps` với handler theo vai trò. Chỉ thêm step
  khi agent của vai trò đó có trong squad (`sdlcBuildSetup`); thiếu thì bỏ step đó, Lead dùng bảng thay thế trong playbook.
- Tạo status còn thiếu như `ensureQAStatus` đang làm (khái quát hoá thành `ensureStatuses`).
- Rút brief blocked và các brief build về cùng nội dung ngắn như T1.7. Comment đầu file nhắc "giữ hai
  bản đồng bộ" được giữ; thêm test so khớp (xem dưới).
- Test: `sdlcSteps` có `environment` khi có devops; blocked vẫn handler lead, `back: in_progress`; brief blocked trỏ tới skill;
  thiếu devops thì không có step `environment`; test hiện có vẫn qua.
- Kiểm: `cd server && go test ./internal/service -run SDLC -count=1`, rồi `make test` (cần DB, dùng môi trường của checkout: `make status`).

### T2.2 Đường nâng cấp cho squad đã tạo

File: `packages/core/squads/quickstart.ts` (+ `quickstart.test.ts`), và nơi UI gọi quickstart nếu cần một nút.

Hiện quickstart "never edits something that already exists" (workflow `existing`, agent `reused`,
`squad.instructions` giữ). Thêm một bước `upgrade` có chủ đích, không đổi hành vi mặc định:

- **Workflow.** Dùng `dryRunIssueWorkflowUpdate` rồi `updateIssueWorkflow`. Chỉ *thêm* step thiếu (ví dụ `environment`), giữ handler
  đã gán; thay `instructions` của một step chỉ khi chuỗi hiện tại đúng bằng một bản cũ đã biết (danh sách hằng số `KNOWN_PREVIOUS_BRIEFS`,
  gồm bản hiện tại của `SDLC_BLOCKED_INSTRUCTIONS`, `planningBrief`, brief `in_review`/`qa`); khác thì giữ và báo trong kết quả.
- **Agent và squad instructions.** Ghi đè `instructions` của agent SDLC và của squad chỉ khi bằng một bản cũ đã biết (băm/đối chiếu chuỗi); khác thì giữ và báo.
- **Status.** Tạo `Environment` nếu thiếu; lỗi quyền admin trả thông báo rõ ("workspace admin must allow: Environment").
- **Skill.** Đã được `attachBuiltinSkills` lo; chỉ cần đảm bảo bước này chạy trong upgrade.
- Kết quả trả về liệt kê từng thứ: đã nâng, đã giữ vì tuỳ biến (kèm lý do), thất bại.
- Test với client giả: workflow cũ → thêm step, giữ handler tuỳ biến; brief tuỳ biến không bị ghi đè; chạy hai lần không đổi gì (idempotent);
  agent có instruction lạ không bị ghi đè.

Cần chốt trước khi làm: tập `KNOWN_PREVIOUS_BRIEFS` lấy từ lịch sử git của `templates.ts`
(`git log -p -- packages/core/squads/templates.ts`) và bản đang nằm trong DB của squad mua_ngon (đã khớp bản repo hiện tại cho skill;
cần so thêm cho brief và instruction).

### T2.3 Kiểm chứng giai đoạn 2

`pnpm typecheck && pnpm lint && pnpm test`, và `make test` cho Go. Báo cáo test nào đã chạy, test nào bỏ qua vì thiếu môi trường.

**Điểm dừng Giai đoạn 2.** Dừng, báo cáo, hỏi người dùng trước khi động vào squad thật.

---

## Giai đoạn 3: áp dụng và diễn tập

Giai đoạn này chạy trên dữ liệu và agent thật, nên mọi bước thay đổi dữ liệu hoặc chạy agent
cần người dùng đồng ý trước.

### T3.1 Chốt các câu hỏi mở còn lại bằng thử nghiệm nhỏ

Trên một project thử (không phải mua_ngon), mỗi câu một thử nghiệm ngắn:

- Q2: giao một issue nhỏ ở status `architecture` cho Architect đang `offline` (runtime `online`); xem task có được dispatch không.
- Q3: chỉ khi muốn truy vết bằng parent; mặc định bỏ qua vì thiết kế đã chọn không parent.
- Q4: tạo hai issue, gán issue A cho Lead, đăng ký `wakeup create A --until-issue B --until-issue-state ended`; đóng B; xem Lead có được đánh thức trên A.
- Q5: chạy `docker version`, `docker ps`, và một lệnh mạng ra ngoài *từ trong một run DevOps* (issue thử ở status `environment`) để biết sandbox có cho phép không.

Nếu Q5 cho thấy sandbox chặn: ghi vào `docs/sdlc/environment.md` của project, và nói với người dùng cấu hình runtime nào cần đổi.
Đây là lớp `capability`, không phải lỗi thiết kế.

### T3.2 Nâng cấp squad mua_ngon_ecommerce

Chỉ sau khi người dùng đồng ý và không có run nào đang chạy trên project (kiểm: không issue nào ở
`in_progress`/`in_review`/`qa` có task `running`). Epic 7 đang ở `qa`, nên đợi nó xong hoặc dừng ở điểm an toàn.

1. Sao lưu: xuất `issue_workflow`, `skill`, `agent.instructions`, `squad.instructions` của squad ra file (`pg_dump` các bảng hoặc truy vấn `COPY`).
2. Chạy đường nâng cấp ở chế độ dry-run, đọc báo cáo.
3. Áp dụng. Xác nhận trong DB: skill khớp repo, workflow có step `environment`, brief `blocked` mới, Lead instruction mới.
4. Hoàn tác (nếu cần): khôi phục từ file sao lưu bước 1.

### T3.3 Diễn tập sáu kịch bản

Mỗi kịch bản tạo một issue nhỏ trong một project thử có repo thử (hoặc một nhánh riêng), chạy bằng agent thật,
và có kỳ vọng kiểm được trong DB và trong comment.

| # | Kịch bản | Cách gây ra | Kỳ vọng |
| --- | --- | --- | --- |
| S1 | Môi trường chết | Dừng stack dịch vụ của project thử trước khi QA chạy | QA báo `Blocker-class: environment`; Lead tạo issue cho DevOps ở `environment`; DevOps ra `env-ensure` + `environment.md`; Lead cho tiếp tục; không có proposal xin hạ tầng |
| S2 | AC mâu thuẫn | Hai AC của một story đối nghịch | Developer báo `requirement`; Lead giao Analyst; Analyst sửa spec qua Ruling; Developer tiếp tục với AC mới |
| S3 | Quyết định kiến trúc | Story cần hợp đồng API mà tài liệu không nói | Developer báo `design`; Architect ra ADR; Developer theo ADR |
| S4 | Vòng lặp gốc thiết kế | Cố ý để Reviewer trả lại ba lần cùng một finding thiết kế | Từ lần ba Developer báo `rework-loop`; Lead giao Architect; vòng dừng |
| S5 | Finding bảo mật | Một endpoint thiếu kiểm quyền | Security báo/được giao; phán quyết nêu kiểm soát cần có và owner fix; không có waiver |
| S6 | Thiếu vai trò | Project initiative không có Architect trong roster | Lead dùng bảng thay thế hoặc lên người dùng với khuyến nghị, không im lặng |

Mỗi kịch bản ghi: số lượt chạy theo agent, thời gian `blocked` → tiếp tục, số proposal gửi người dùng.
Lỗi phát hiện được sửa trong skill (Giai đoạn 1) rồi diễn tập lại kịch bản đó.

### T3.4 Báo cáo

Một báo cáo ngắn trong `docs/engineering/` hoặc comment issue: kết quả sáu kịch bản, chỉ số mục 8 của spec
trước/sau (số liệu "trước" lấy từ mục 2 của spec), những gì chưa kiểm.

---

## Giai đoạn 4 (tuỳ chọn): tự động hoá phía server

AGENTS.md yêu cầu giải thích chi phí và khả năng xảy ra trước khi thêm phức tạp cho trường hợp hiếm. Giai đoạn này **chỉ
làm khi số liệu Giai đoạn 3 cho thấy cần**, và cần buy-in của người dùng. Ứng viên và điều kiện để xem là đáng:

| Ứng viên | Lợi ích | Chi phí | Chỉ làm khi |
| --- | --- | --- | --- |
| `blocker_class` lưu ở `issue.properties` (hoặc cột riêng) | Thống kê theo lớp, hiển thị trên bảng | Đổi API, schema zod, client, migration | Muốn bảng thống kê và Lead phân loại sai thường xuyên |
| Bộ đếm vòng trả lại phía server, tự chuyển issue sang `blocked` ở lần thứ ba | Không phụ thuộc agent tự đếm | Logic mới ở handler workflow, test race | Developer hay quên đếm trong diễn tập S4 |
| Giới hạn vòng chuyên gia phía server | Chặn chi phí mất kiểm soát | Trạng thái mới | Có trường hợp vượt giới hạn trong thực tế |
| Bảng điều khiển "Chuyên gia nào được gọi bao nhiêu lần" | Nhìn thấy ngay vấn đề kiểu mục 2 | UI mới | Người dùng muốn |

---

## Rủi ro của kế hoạch

- **Chi phí chạy agent** tăng ở Giai đoạn 3 (diễn tập). Làm từng kịch bản một, dừng khi đã có kết quả.
- **Đường nâng cấp ghi đè nhầm tuỳ biến của người dùng.** Chỉ ghi đè khi khớp bản cũ đã biết; luôn dry-run; có sao lưu.
- **Q2, Q4, Q5 có thể lật thiết kế.** Chúng được thử trước (T3.1) và trước khi nâng cấp squad thật. Nếu Q4 sai thì thay wakeup bằng
  `--kind every` ngắn của Lead trên epic bị block (cơ chế đã dùng ở Step 4 của Lead); nếu Q2 sai thì cần cách đánh thức agent
  offline trước khi giao việc.
- **Skill dài ra.** Lead skill hiện 198 dòng; giữ phần mới ngắn, đưa bảng và mẫu vào `team-playbook.md`.

## Danh sách việc ngắn (để theo dõi)

Giai đoạn 1: T1.1 core, T1.2 lead, T1.3 playbook, T1.4 devops, T1.5 ruling mode, T1.6 dev/review/QA, T1.7 template,
T1.8 docs, T1.9 bundle và test. Giai đoạn 2: T2.1 server mirror, T2.2 đường nâng cấp, T2.3 kiểm chứng.
Giai đoạn 3: T3.1 thử Q2/Q4/Q5, T3.2 nâng cấp mua_ngon, T3.3 sáu kịch bản, T3.4 báo cáo.
Giai đoạn 4: tuỳ chọn.
