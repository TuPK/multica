# BMAD Planning squad và auto handoff giữa các phòng

Spec (nghiên cứu phương án, chưa triển khai). Đi sau [sdlc-autonomy.md](sdlc-autonomy.md) và
[sdlc-specialist-routing.md](sdlc-specialist-routing.md). Squad mới là "phía trước" của SDLC squad: nó viết và bổ sung
tài liệu kế hoạch theo BMAD để SDLC squad có thứ để build.

Ngôn ngữ: tiếng Việt để người dùng đọc; tên lệnh, status, file, skill và đoạn văn bản dành cho agent giữ nguyên tiếng
Anh, vì skill và instruction của agent viết bằng tiếng Anh.

## 0. Trạng thái triển khai

**Giai đoạn 1** (template và skill): template `bmad_planning`, 7 agent tên `Blueprint …`, workflow "BMAD Planning Auto"
(Research, Brief, PRD, Design, Architecture, Epics, Plan review, Stage done, Plan ready), 7 skill `planning-*`, 5 skill vendor
(`bmad-toolbox:bmad-deep-recon`, `bmad-forge-idea`, `bmad-review`, `bmad-advanced-elicitation`, `frontend-design`), thẻ chọn
template ở 7 ngôn ngữ, `SDLC_SKILL_NAMES` loại các skill chỉ của planning.

**Giai đoạn 2** (dự án cho squad planning): `AssignSquadProject` chọn workflow theo roster. Squad có `Blueprint Lead` nhận
workflow "BMAD Planning Auto" do quickstart tạo (tra theo tên, kiểm bước `blocked` giao cho lead của squad), không nhân bản định
nghĩa sang Go. Thiếu workflow thì trả `422 planning_workflow_missing`. Các squad khác vẫn nhận SDLC Auto như cũ.

**Giai đoạn 3** (handoff): bảng `squad_handoff_rule` và `squad_handoff` (migration 917 đến 921), `SquadHandoffService`,
API `GET/PUT /api/squads/{id}/handoff-rule` và `GET/POST /api/issues/{id}/handoff` (+ `/decline`), lệnh `multica squad handoff
<request>` và `multica squad handoff-rule get|set`, khối "Handoff" trong panel squad của phòng và mục "Handoff" trong sidebar
request, governance event `squad_handoff`. Lead của planning gọi `multica squad handoff` ở cuối; SDLC Lead có mục "A request from
a planning squad" trong skill.

**Lệch so với thiết kế ở mục 4** (có chủ ý):

- Một rule cho mỗi squad nguồn (unique theo `workspace_id, source_squad_id`), không phải theo cặp: UI chỉ có một đích mỗi squad.
- Chế độ `ask` dùng nút Approve / Decline trên request, **không dùng proposal**: proposal bị giới hạn theo tuần (đã là nguyên
  nhân một block dài trong `sdlc-autonomy.md`), nên một handoff đang chờ có thể bị từ chối chỉ vì hết hạn mức.
- Request đích **không có parent**: nó mang `origin_type = squad_request` với `origin_id` là request nguồn, và bảng
  `squad_handoff` nối hai đầu. Parent sẽ gắn trạng thái của hai bên vào nhau.
- Handoff tự động chỉ chạy khi `company_state` không paused. Khi paused, nó ở `pending` và boss duyệt bằng tay.
- Handoff tự động thất bại (đích không có dự án mở, tạo issue lỗi) thì xoá dòng `pending` vừa tạo, để lần hỏi sau của lead thử lại.
- Server kiểm gói bàn giao theo chuỗi cố định (`## Planning handoff` và `Readiness verdict: PASS`) cho lần lead hỏi; nút bấm tay của
  boss không cần gói.

Chưa làm: đóng vòng (4.7, comment ngược khi request đích xong), diễn tập với agent thật (mục 7).

## 1. Tóm tắt

Hai yêu cầu, hai kết luận:

1. **Squad "BMAD Planning" làm được bằng cơ chế sẵn có, không cần đổi server.** Template, quickstart, bundle skill,
   workflow, charter và đội ngũ agent đều đi theo đúng khuôn của SDLC. Phần mới là một đội 7 agent, một bộ skill
   `planning-*` bọc quy trình BMAD (PRD, UX, architecture, epics, correct-course), và một workflow "BMAD Planning Auto".
   Toàn bộ skill BMAD method đã nằm trong bundle SDLC. Cần thêm 5 skill của BMAD toolbox (nổi bật là `bmad-deep-recon`
   cho nghiên cứu thị trường, đối thủ, kỹ thuật) và `frontend-design` cho UX.
2. **Auto handoff cần một tính năng nền tảng nhỏ.** Không có cơ chế sẵn để "xong ở phòng này thì tự chuyển cho lead của
   phòng khác khi bạn bật công tắc". Đề xuất: một *handoff rule* theo cặp squad (`off | ask | auto`), một lệnh
   `multica squad handoff` mà lead của squad nguồn luôn gọi ở cuối, và một service phía server tạo request cho squad đích.
   Server quyết định dựa trên công tắc, nên hành vi không phụ thuộc agent nhớ làm gì.

Có ba điều chặn phải xử lý, nêu ở mục 2.3: `AssignSquadProject` đang cố định vào workflow SDLC, tên agent trùng với
starter company, và `SDLC_SKILL_NAMES` đang là "mọi skill trong thư mục", nên thêm skill planning vào thư mục sẽ làm
mọi SDLC squad nhận luôn các skill đó.

## 2. Hiện trạng đã kiểm chứng

### 2.1 Tái dùng được

| Cái đã có | Ở đâu | Dùng cho |
| --- | --- | --- |
| Template squad: roles, charter, instructions, workflow, `builtinSkills` | `packages/core/squads/templates.ts` | Khai báo `bmad_planning` |
| Quickstart idempotent: tạo agent (tái dùng theo tên), squad, seat, charter, member, workflow, routine, attach skill | `packages/core/squads/quickstart.ts` | Chạy lại an toàn; không đổi |
| Bundle skill sinh từ thư mục, có test chống bản cũ | `scripts/generate-sdlc-skills.mjs`, `sdlc-skills/` | Thêm skill planning |
| Toàn bộ BMAD method đã vào bundle: product-brief, prfaq, prd, ux, architecture, create-epics-and-stories, sprint-planning, correct-course, project-context, retrospective, walkthrough, spec, agent-analyst/pm/architect/ux-designer | `sdlc-skills/bmad-method--*` | Skill chính của planning |
| BMAD có chế độ headless cho brief, PRD, UX, architecture (`references/headless.md`) | `bmad-method--bmad-prd/SKILL.md`, `bmad-architecture/references/` | Chạy không cần người duyệt |
| Workflow step có handler loại `squad` | `server/internal/issueworkflow/issueworkflow.go` (`HandlerSquad`) | Phương án B ở 4.2 |
| Squad request: `multica issue create --assignee-squad <id> --parent <issue>`, gắn `origin_type = squad_request` | `server/internal/handler/issue.go`, spec 3.5 | Dạng issue mà squad đích nhận |
| Autonomy L0..L3, `company_state` paused/quiet, budget, governance log | `server/internal/service/autonomy.go`, spec phần 4 | Giới hạn cho handoff tự động |
| Proposal có kind, standing approval | spec 3.2, 4.4 | Chế độ `ask` |
| Initiative pipeline: Plan (squad Planning) → Build (squad SDLC), exit `auto/attest/gate` | spec 3.1, `server/internal/service/initiative*.go` | Hướng mở rộng, xem 4.6 |

### 2.2 Chưa có

- Không có khái niệm "khi squad A xong thì giao cho squad B".
- Không có công tắc bật/tắt chuyển giao nào cho một squad hay một phòng.
- `AssignSquadProject` luôn gọi `EnsureSDLCWorkflow` (`server/internal/handler/squad_project.go`) và trả
  `build_squad_not_sdlc` nếu squad không có developer, reviewer và QA của SDLC. Một squad planning không nhận được dự án.
- Workflow SDLC được viết hai lần: TypeScript (`SDLC_AUTO_WORKFLOW`) và Go (`initiative_build.go`), kèm chú thích "keep the
  two in step". Không nên nhân bản thêm lần nữa cho planning.

### 2.3 Ba điều chặn

1. **Dự án của squad.** Phòng planning phải có một dự án (repo, `local_directory`, `execution_mode`) để agent viết tài liệu
   vào đúng chỗ, và workflow của dự án đó phải là workflow của planning, không phải SDLC Auto.
2. **Tên agent trùng.** Quickstart tái dùng agent theo tên. Starter company đã có "Planning Lead", "Research Lead",
   "Competitor Analyst" và `SQUAD_ROLES` cũng dùng hai tên sau. Đặt tên trùng sẽ gắn nhầm agent của squad khác vào đội
   này. Dùng tiền tố `Blueprint` cho mọi agent của squad (không dùng `BMAD`, theo yêu cầu của chủ dự án); squad vẫn tên `BMAD Planning`.
3. **`SDLC_SKILL_NAMES` lấy mọi thư mục.** `templates.ts` định nghĩa `SDLC_SKILL_NAMES = SDLC_BUILTIN_SKILLS.map(name)`.
   Thêm `planning-*`, `bmad-deep-recon` vào cùng thư mục sẽ khiến SDLC Greenfield và Maintenance import cả chúng. Phải
   đổi `SDLC_SKILL_NAMES` thành danh sách tường minh (hoặc loại tiền tố `planning-`) trước khi thêm skill.

## 3. Squad BMAD Planning

### 3.1 Template và đội ngũ

`id: "bmad_planning"`, tên squad `BMAD Planning`, `floorName: null` (bạn tự đặt vào phòng planning sau, như đã nêu),
`routine: null` (squad làm theo yêu cầu, không có lịch), `leader: "plan_lead"`.

| Role key | Tên agent | Việc | Skill chính |
| --- | --- | --- | --- |
| `plan_lead` | Blueprint Lead | Đọc yêu cầu, chọn route, chia việc, kiểm chứng, báo cáo, handoff | `planning-squad-lead` |
| `plan_researcher` | Blueprint Researcher | Nghiên cứu thị trường, miền, đối thủ, kỹ thuật, người dùng, có nguồn và ngày | `planning-research`, `bmad-deep-recon` |
| `plan_analyst` | Blueprint Analyst | Làm rõ yêu cầu, product brief, PRFAQ, kiểm tra ý tưởng | `planning-product`, `bmad-product-brief`, `bmad-prfaq`, `bmad-forge-idea` |
| `plan_pm` | Blueprint PM | PRD, epics và stories, readiness sprint, correct-course khi đổi scope | `planning-product`, `bmad-prd`, `bmad-create-epics-and-stories`, `bmad-sprint-planning`, `bmad-correct-course` |
| `plan_ux` | Blueprint UX Designer | Thiết kế UX, màn hình, trạng thái, design system | `planning-ux`, `bmad-ux`, `frontend-design` |
| `plan_architect` | Blueprint Architect | Architecture, ADR, impact analysis, project context | `planning-architecture`, `bmad-architecture`, `bmad-project-context` |
| `plan_reviewer` | Blueprint Reviewer | Review độc lập tài liệu, kiểm tra sẵn sàng cho implement, trả verdict | `planning-readiness`, `bmad-review`, `bmad-advanced-elicitation` |

Lý do tách `Blueprint Researcher` khỏi `Blueprint Analyst`: nghiên cứu là việc song song và nhiều nguồn, còn brief và PRFAQ là việc
nối tiếp, cần phán đoán sản phẩm. Hai agent chạy đồng thời không đụng nhau. Số agent nhiều hơn SDLC planning một người, đổi
lại thời gian chờ ngắn hơn khi yêu cầu cần cả nghiên cứu lẫn UX.

Mỗi agent có `agentTemplate` riêng trong `agents/instruction-templates.ts` hoặc dùng bản ngắn kiểu `frame()` của
`sdlc-agent-instructions.ts` (giới hạn luôn đúng + dòng "load skill"). Chọn bản ngắn: thủ tục nằm trong skill, như SDLC.

### 3.2 Bộ skill

Phân ba nhóm. Nhóm nào đã có trong bundle thì không phải sao chép thêm.

**Nhóm 1, đã có trong bundle, dùng nguyên:** `bmad-product-brief`, `bmad-prfaq`, `bmad-prd`, `bmad-ux`,
`bmad-architecture`, `bmad-create-epics-and-stories`, `bmad-sprint-planning`, `bmad-correct-course`,
`bmad-project-context`, `bmad-retrospective`, `bmad-walkthrough`, `bmad-agent-analyst`, `bmad-agent-pm`,
`bmad-agent-architect`, `bmad-agent-ux-designer`, `superpowers:brainstorming`, `superpowers:writing-plans`,
`superpowers:dispatching-parallel-agents`, `superpowers:verification-before-completion`.

**Nhóm 2, cần thêm vào bundle** (đã thêm 5 skill; bỏ `bmad-brainstorming` vì assets nặng 240 KB và `superpowers:brainstorming` đã có; lấy từ `plugins/bmad/plugins/toolbox/skills/` và plugin chính thức; kiểm tra giấy phép
trước khi vendor, như đã làm với accessibility, superpowers, supabase):

| Skill | Lý do |
| --- | --- |
| `bmad-deep-recon` | Nghiên cứu có cấu trúc: market, domain, technical, competitive, user; chạy song song và ra bản tóm tắt có trích nguồn mà skill khác dùng thẳng được |
| `bmad-forge-idea` | Thử độ chắc của một ý tưởng, ra brief ngắn |
| `bmad-review` | Review nhiều lăng kính (đối kháng, edge case, thiếu kiểm chứng, cấu trúc, văn phong) cho tài liệu |
| `bmad-advanced-elicitation` | Pre-mortem, red team, first principles để soi lại bản nháp |
| `frontend-design` | Chất lượng thiết kế giao diện cho UX Designer |

Chưa chắc: `bmad-party-mode` (đối thoại nhiều agent) hợp với chạy có người, ít hợp với chạy không người giám sát; để ngoài
bundle. `dataviz` và `deep-research` của anthropic-skills cũng đáng cân nhắc cho sizing thị trường nhưng chồng với
`bmad-deep-recon`; để sau, khi thấy thiếu.

**Nhóm 3, skill riêng của Multica** (viết mới, thư mục `sdlc-skills/planning-*`), giống vai trò `sdlc-*`:

| Skill | Nội dung |
| --- | --- |
| `planning-delivery-core` | Quy tắc chung: unattended, bảng sở hữu tài liệu, thang bằng chứng cho nghiên cứu (mỗi khẳng định một nguồn có ngày), cách ghi giả định vào `docs/planning/assumptions.md`, định dạng báo cáo, ID ổn định |
| `planning-squad-lead` | Thủ tục của lead: route, chia issue, hàng đợi, kiểm chứng, handoff (mục 3.4, 4) |
| `planning-research`, `planning-product`, `planning-ux`, `planning-architecture`, `planning-readiness` | Mỗi role: skill BMAD nào chạy cho việc nào, chạy headless ra sao, ghi vào file nào, tự kiểm trước khi báo xong |

Các skill BMAD gốc hỏi người dùng nhiều lần. Skill `planning-*` bọc chúng bằng quy tắc của SDLC: bật headless khi skill có,
không thì lấy mặc định được khuyến nghị và ghi thành giả định. Đây là cùng cách `RESEARCH_COMMON_INSTRUCTIONS` đã làm
("This overrides ... the approval steps of the bmad skills").

### 3.3 Cách squad làm việc

**Workflow "BMAD Planning Auto"** (do quickstart tạo, handler là agent của squad, status dùng lại theo tên nếu đã có):

| Status | Handler | Xong thì |
| --- | --- | --- |
| Research | Blueprint Researcher | Stage done |
| Brief | Blueprint Analyst | Stage done |
| PRD | Blueprint PM | Stage done |
| Design | Blueprint UX Designer | Stage done |
| Architecture | Blueprint Architect | Stage done |
| Epics | Blueprint PM | Stage done |
| Plan review | Blueprint Reviewer | Stage done khi PASS; Blocked khi CONCERNS hoặc FAIL |
| Blocked | Blueprint Lead | lead định tuyến như Step 7 của SDLC |
| Stage done, Plan ready | không có | Plan ready là điểm cuối, không ai nhận |

Cùng cơ chế với SDLC: lead tạo issue trong dự án, giao cho thành viên bằng id, xếp hàng bằng
`multica issue queue ... --after ... --start-status <status>`. Nếu thư mục dự án là `in_place`, mọi issue xếp thành một
chuỗi, giống Step 3 của `sdlc-squad-lead`. Tài liệu của mỗi role nằm ở file riêng nên đụng nhau ít, nhưng git commit và
working copy thì dùng chung, vì vậy giữ chuỗi.

**Hai chế độ của lead**, chọn bằng việc repo đã có tài liệu BMAD hay chưa:

- **Mode N (mới):** không có tài liệu. Research (nếu yêu cầu cần) → Brief → PRD → Design (nếu có UI) → Architecture →
  Epics → Plan review.
- **Mode X (bổ sung, "thêm tính năng X"):** đã có `prd`, `architecture`, `ux-design`, `epics`. Đây là chế độ bạn dùng nhiều
  nhất, nên được thiết kế kỹ hơn:
  1. Lead đọc tài liệu hiện có và phần đã delivered (epics đã xong, code, git log), rồi viết **impact map**: yêu cầu mới
     chạm tài liệu nào, phần nào, và có đổi gì đã chấp nhận hoặc đã build không.
  2. Chỉ tạo issue cho tài liệu bị chạm. Yêu cầu chỉ là một tính năng nhỏ có UI sẵn thì không có issue Design; không đổi
     kiến trúc thì không có issue Architecture, trừ khi impact map nói khác. Mỗi lần bỏ qua một bước phải ghi lý do.
  3. Mỗi role **sửa tại chỗ**, không viết lại: FR, NFR, ADR, màn hình giữ ID cũ; phần mới lấy ID tiếp theo; mỗi tài liệu có
     mục "Change log" ghi thay đổi, nguồn yêu cầu và vì sao. Không xoá epic hay story đã delivered; yêu cầu mới thành epic
     mới hoặc story mới ở cuối.
  4. Nếu yêu cầu đảo ngược một quyết định đã chấp nhận hoặc đổi tiêu chí chấp nhận của phần đã build, PM chạy
     `bmad-correct-course` ra sprint change proposal, và Reviewer kiểm. Đây là điều kiện duy nhất khiến squad dừng chờ
     boss, vì nó có thể làm hỏng cái đã build.
  5. Plan review kiểm sự nhất quán chéo (PRD ↔ UX ↔ architecture ↔ epics), mọi acceptance criterion kiểm chứng được, mọi
     story có đường tới một FR, và không có thay đổi nào bị bỏ sót so với impact map.

**Bảng sở hữu tài liệu** (nằm trong `planning-delivery-core`, mỗi role chỉ sửa tài liệu của mình):

| Tài liệu | Chủ | Vị trí mặc định của BMAD |
| --- | --- | --- |
| Research report | Researcher | `{planning_artifacts}/research/` |
| Brief, PRFAQ | Analyst | `{planning_artifacts}/briefs/` |
| PRD | PM | `{planning_artifacts}/prds/` |
| UX design | UX Designer | `{planning_artifacts}/ux-designs/` |
| Architecture, ADR, project-context | Architect | `{planning_artifacts}/architecture/` |
| Epics và stories | PM | `{planning_artifacts}/epics.md` |
| Sprint change proposal | PM | `{planning_artifacts}/sprint-change-proposal-{date}.md` |
| Assumptions, rulings | Lead | `docs/planning/assumptions.md` |

`{planning_artifacts}` lấy theo cấu hình BMAD của repo (`_bmad/`), mặc định là `_bmad-output/planning-artifacts`.
SDLC Lead đã đọc `docs/`, `_bmad-output/` và planning artifacts khi triage (Step 2), nên Plan A của SDLC ("BMAD epics exist")
nhận ra output của squad này mà không cần đổi triage.

### 3.4 Gói bàn giao (handoff packet)

Khi Plan review PASS, lead viết một comment cuối trên request, theo mẫu cố định, để cả người lẫn SDLC Lead đọc:

```
## Planning handoff
Request: <identifier> <title>
Outcome: <one line>
Mode: N | X
Documents changed: <path> (<what changed, ids touched>) ...
New epics: <epic id, title, stories count> ...   (Mode X: only new or changed)
Delivered and untouched: <epic ids>
Readiness verdict: PASS (review issue <id>)
Assumptions: <ids in docs/planning/assumptions.md>
Open items for implementation: <list or "none">
Build hint: epics to build, in order; known risks; checks that need an environment
```

Gói này là đầu vào của handoff (mục 4). Server kiểm gói có đủ các trường bắt buộc và có link tới verdict PASS trước khi
cho handoff tự động chạy.

### 3.5 Charter và instructions

- Charter: mission "Turn a request into reviewed planning documents the build squad can follow"; responsibilities
  research, requirements, design, architecture, readiness; outputs `planning_report` (kind `report`), `planning_docs`
  (kind `kb_document`); boundaries "Writes planning documents only; no product code; no production changes; spending money
  needs the boss"; escalation "A change that reverses an accepted decision or a built acceptance criterion goes to the
  requester"; intake "Requests arrive as issues assigned to this squad."
- Instructions của lead: ngắn như `SDLC_SQUAD_INSTRUCTIONS`, chỉ nói "load `planning-delivery-core` và `planning-squad-lead`,
  thủ tục nằm ở đó, skill thắng khi khác", cộng khối Unattended của `RESEARCH_COMMON_INSTRUCTIONS`.

### 3.6 Thay đổi code cho template

| File | Thay đổi |
| --- | --- |
| `packages/core/squads/templates.ts` | Thêm `bmad_planning` vào `SquadTemplateId`, 7 role vào `SquadRoleKey` và `SQUAD_ROLES`, `BMAD_PLANNING_WORKFLOW`, charter, instructions, `PLANNING_SKILL_NAMES`; **đổi `SDLC_SKILL_NAMES` thành danh sách tường minh** (điều chặn 3) |
| `packages/core/squads/sdlc-agent-instructions.ts` hoặc file mới `planning-agent-instructions.ts` | Instruction ngắn cho 7 agent |
| `packages/core/squads/sdlc-skills/` | Thêm skill nhóm 2 và nhóm 3; chạy `pnpm generate:sdlc-skills` |
| `packages/core/squads/quickstart.ts` | `instructionsFor` đọc instruction planning; thêm role vào `QuickstartAgentOutcome` (kiểu đã dùng `SquadRoleKey` nên đủ); `attachBuiltinSkills` giữ nguyên |
| `packages/core/squads/upgrade.ts`, `scripts/upgrade-sdlc-squad.mjs` | Cho phép nâng cấp squad planning đã tạo (skill, instruction) giống SDLC |
| `packages/views/modals/squad-quickstart.tsx` và `packages/views/locales/*/modals.json` | Thẻ chọn template: tên và mô tả, đủ 7 ngôn ngữ (en, zh-Hans, zh-Hant, fr, es, ko, ja); theo "UI Copy" trong `AGENTS.md`, mô tả ngắn, không lặp tiêu đề |
| Tests | `templates.test.ts`, `quickstart.test.ts`, `sdlc-skills.test.ts`, `upgrade.test.ts`, `squad-quickstart.test.tsx`: roster, tên không trùng starter company, SDLC không nhận skill planning, workflow hợp lệ, quickstart chạy lại không tạo trùng |

## 4. Auto handoff giữa các phòng

### 4.1 Yêu cầu

- Công tắc bật/tắt do boss điều khiển, theo cặp (squad nguồn → squad đích).
- Bật: khi planning xong theo quy trình, lead của squad đích (SDLC Lead) nhận một request đầy đủ ngữ cảnh và tự chạy
  quy trình có sẵn (đọc tài liệu, tạo issue Build, queue), không chờ boss.
- Tắt: squad nguồn dừng ở Plan ready; boss thấy gói bàn giao và có thể bấm chuyển bằng tay.
- Không phá các giới hạn đã có: `company_state`, autonomy, budget, và các việc luôn cần boss (production, tiền, dữ liệu thật).

### 4.2 Ba phương án

| | A. Agent tự làm | B. Handler `squad` của workflow | C. Rule + service phía server (chọn) |
| --- | --- | --- | --- |
| Cách chạy | Skill bảo lead đọc cờ rồi `multica issue create --assignee-squad` | Bước cuối của workflow planning có handler là squad SDLC; khi lead chuyển status, issue tự đổi assignee sang squad đích | Lead gọi `multica squad handoff`; server đọc rule và tạo request cho squad đích |
| Công tắc | Cờ ở đâu đó, agent phải nhớ đọc | Sửa handler của bước (bật = đặt squad, tắt = `none`) | `mode` của rule |
| Đảm bảo | Phụ thuộc agent | Server làm, nhưng cùng một issue, cùng một dự án | Server làm, có audit |
| Vấn đề | Chưa kiểm chứng agent có quyền gán cho squad khác (`validateAssigneePair`, nhánh `squad`); không có audit; có thể quên | Issue ở lại dự án planning và workflow planning, trong khi SDLC Lead chờ request nằm trong dự án Build với workflow SDLC Auto; workflow gắn với dự án nên không đổi được theo từng yêu cầu | Phải viết bảng, service, API, CLI, UI |
| Giữ audit và idempotent | Không | Một phần | Có |

B nghe gọn nhưng sai ở chỗ SDLC Lead cần request nằm trong dự án Build của nó. A rẻ nhưng không đáng tin cho một thao
tác mà bạn muốn tin tưởng khi bật. **Chọn C**, và C vẫn dùng lại `squad_request` và squad leader trigger sẵn có.

### 4.3 Thiết kế

**Dữ liệu.** Một bảng, không khoá ngoại (theo `AGENTS.md`), index tạo `CONCURRENTLY` trong migration riêng:

```sql
CREATE TABLE squad_handoff_rule (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    source_squad_id UUID NOT NULL,
    target_squad_id UUID NOT NULL,
    mode TEXT NOT NULL DEFAULT 'off' CHECK (mode IN ('off', 'ask', 'auto')),
    created_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);   -- unique (workspace_id, source_squad_id, target_squad_id) where live
```

Bản ghi mỗi lần chuyển giao không cần bảng riêng: request tạo ra mang `origin_type = squad_request`, `origin_id` là request
nguồn, nên nối ngược được; mỗi lần còn ghi một sự kiện governance (`squad_handoff`) có chế độ, nguồn, đích, và ai bật.

**Ba chế độ:**

| Mode | Khi lead nguồn gọi `multica squad handoff` |
| --- | --- |
| `off` | Server trả "handoff is off". Lead kết thúc ở Plan ready. Bạn bấm "Hand off to <squad>" trong request nếu muốn; nút đó đi qua cùng service |
| `ask` | Server tạo một proposal (kind `squad_handoff`) kèm gói bàn giao; duyệt thì service chạy. Hợp với giai đoạn mới bật, chưa tin hoàn toàn |
| `auto` | Service chạy ngay |

**Service `SquadHandoffService.Handoff(request)`**, trong một transaction, trả về cùng kết quả khi gọi lặp:

1. Request nguồn do squad nguồn giữ, đang ở Plan ready (hoặc trạng thái kết thúc planning), chưa có handoff (kiểm bằng
   `origin_id`), nên gọi hai lần không tạo hai request.
2. Có rule `source → target`, squad đích còn sống, và squad đích **có dự án mở** (`GetOpenSquadProject`); không có thì trả
   lỗi rõ ràng và request nguồn kèm cảnh báo, không tạo gì.
3. Gói bàn giao có đủ trường bắt buộc và link verdict PASS (3.4); thiếu thì từ chối và nói thiếu gì.
4. Chặn vòng: rule không được tạo chu trình (A→B→A), và một request đã sinh từ handoff không được tự handoff tiếp quá một
   cấp; đích SDLC không có rule đi ra ở bản đầu.
5. Kiểm điều khiển công ty như một lần giao việc thường: `company_state` paused hoặc quiet thì tạo request ở trạng thái
   chờ, budget còn, autonomy của squad đích không phải L0 (L0 chỉ giữ routine, không giữ giao việc, nên không chặn).
6. Tạo issue trong dự án đang mở của squad đích: title `[Planning handoff] <title nguồn>`, mô tả là gói bàn giao + đường dẫn
   tài liệu + câu "This run is unattended", `assignee = squad đích`, `parent = request nguồn`, `origin = squad_request`.
   Squad leader trigger sẵn có đánh thức SDLC Lead.
7. Comment trên request nguồn với link sang request đích; ghi governance event.

Service tạo issue bằng actor hệ thống, nên không phụ thuộc quyền của agent (điều chưa kiểm ở phương án A).

**CLI cho lead.** `multica squad handoff <request-id>`: gọi service và in một trong `handed_off <issue>`, `pending_approval
<proposal>`, `off`, hoặc lỗi cụ thể. `planning-squad-lead` chỉ có một chỉ thị: "At the end, after Plan review PASS and the
packet is posted, run `multica squad handoff <request>` and follow its answer. Never create the build request yourself."

**API và UI.**

- `GET/PUT /api/squads/{id}/handoff-rules` (admin hoặc owner của squad; agent không đổi được rule, như autonomy).
- `POST /api/issues/{id}/handoff` cho nút bấm tay.
- Công tắc nằm ở hai chỗ: tab Governance của squad (cạnh autonomy, vì cùng loại quyết định), và overlay phòng trong tab
  Company, hiện "When planning is done: Off / Ask me / Hand off automatically → [SDLC squad]". Hành vi mặc định `off`.
- Trên request nguồn: chip trạng thái "Handed off to SDLC Greenfield" kèm link, hoặc nút "Hand off" khi chưa chuyển.

### 4.4 Phía squad nhận

SDLC Lead đã làm đúng việc này: một request tới squad, đọc repo và tài liệu, Plan A nếu có BMAD epics, tạo issue Build, queue,
verify. Cần ba thay đổi nhỏ trong skill `sdlc-squad-lead`, không đổi server:

1. Nhận ra request có `origin = squad_request` từ squad planning: đọc gói bàn giao trong mô tả và comment của request nguồn
   như nguồn triage chính, nhưng vẫn mở file và kiểm bằng chứng (một gói là lời khai, không phải bằng chứng).
2. Epic trong gói đánh dấu "Delivered and untouched" thì tạo như đã delivered, có link bằng chứng.
3. Không hỏi lại boss những điều gói đã ghi thành giả định.

### 4.5 Thư mục dùng chung và hàng đợi

Hai squad cần cùng một repo. Cách làm: dự án planning và dự án Build của SDLC cùng trỏ tới một `local_directory`. Planning
xong trước khi handoff chạy nên hai squad không ghi cùng lúc trong một yêu cầu. Rủi ro còn lại là yêu cầu thứ hai vào
planning khi SDLC đang build ở thư mục `in_place`. Biện pháp: planning chỉ commit đường dẫn tài liệu của mình (quy tắc
"commit only your own paths" đã có). Bản thân request handoff chỉ đánh thức SDLC Lead để triage (đọc, không sửa file); các
issue Lead tạo sau đó đã chịu quy tắc "issues you add later wait for the last issue still queued or running" của
`sdlc-squad-lead`, nên không chạy chồng lên build đang chạy.

### 4.6 Khi nào dùng pipeline initiative thay vì rule

Initiative (spec 3.1) đã có Plan → Build với `exit.mode = auto` và `startBuild` tạo dự án Build cùng một issue khởi động.
Nó phù hợp khi mỗi sản phẩm là một initiative có boss duyệt. Hai điểm khiến nó không thay được handoff rule cho nhu cầu
của bạn: nó giao issue khởi động cho **developer**, không phải SDLC Lead, nên bỏ qua triage và kế hoạch của Lead; và yêu
cầu ad hoc kiểu "bổ sung tính năng X" không đi qua một initiative. Hai cơ chế cùng tồn tại được: bước Plan của initiative
có thể có chủ là squad BMAD Planning, và lúc đó handoff của initiative vẫn do pipeline lo, rule chỉ áp cho yêu cầu ad hoc.

### 4.7 Đóng vòng (tuỳ chọn, giai đoạn sau)

Khi request đích xong, comment ngược lên request nguồn ("Delivered: <kết quả>, bằng chứng") để người xem một chỗ thấy cả
đường đi. Không bắt buộc cho bản đầu.

## 5. Thay đổi theo thành phần

| Thành phần | Thay đổi |
| --- | --- |
| `packages/core/squads/` | Template, role, workflow, instruction, skill như 3.6; `SDLC_SKILL_NAMES` tường minh |
| `server/internal/handler/squad_project.go`, `service/initiative_build.go` | `AssignSquadProject` chọn workflow theo squad: roster SDLC → SDLC Auto như hiện nay; roster BMAD Planning → workflow "BMAD Planning Auto" **tra theo tên trong workspace** (do quickstart tạo), không nhân bản định nghĩa sang Go |
| `server/migrations/` | `squad_handoff_rule` (một migration bảng, một migration cho mỗi index `CONCURRENTLY`); `squad_handoff` vào danh sách loại sự kiện governance và loại proposal |
| `server/internal/service/` | `SquadHandoffService`; kiểm gói; ghi governance; proposal kind `squad_handoff` |
| `server/internal/handler/`, `server/cmd/multica/` | Handler rule và nút bấm tay; lệnh `multica squad handoff`; chạy `make sqlc` sau khi thêm SQL |
| `packages/core/` | Zod schema và `parseWithFallback` cho response rule, kèm test response lỗi (quy tắc API compatibility của `AGENTS.md`); query và mutation; khoá query có `wsId` |
| `packages/views/` | Công tắc ở Governance và room overlay; chip và nút trên request; i18n 7 ngôn ngữ; áp dụng cho cả web và desktop |
| `sdlc-skills/sdlc-squad-lead` | Ba thay đổi ở 4.4 |
| Docs | Phần này trong `digital-company.md` (spec 3.5 mở rộng), mô tả công tắc cho người dùng |

## 6. Giai đoạn triển khai

Mỗi giai đoạn tự chạy được, giai đoạn sau không làm hỏng giai đoạn trước.

1. **Template và skill (chỉ `packages/core` và views).** Sửa `SDLC_SKILL_NAMES`; thêm skill nhóm 2 và 3; role, workflow,
   charter, instruction; thẻ quickstart. Kiểm: test của core và views; chạy quickstart hai lần không tạo trùng; SDLC squad
   không nhận skill planning. Ở bước này squad dùng được bằng tay: bạn giao request cho squad, nó ra tài liệu và dừng ở
   Plan ready.
2. **Dự án cho squad planning (server nhỏ).** `AssignSquadProject` theo roster. Kiểm: gán dự án cho squad BMAD Planning
   thành công, squad SDLC không đổi hành vi, xung đột workflow vẫn trả `project_workflow_set`.
3. **Handoff rule (`off` và nút bấm tay trước, rồi `auto`).** Bảng, service, CLI, API, công tắc. Làm `off` + tay trước để
   kiểm service và audit, rồi bật `ask`, cuối cùng `auto`.
4. **Bên nhận và diễn tập.** Sửa `sdlc-squad-lead`; diễn tập trên agent thật (mục 7).

Có thể dừng sau giai đoạn 1 hoặc 2 và vẫn có một squad planning dùng được; giai đoạn 3 là phần bạn cần cho "tự chuyển khi
bật công tắc".

## 7. Kiểm chứng

- Test nội dung skill, như `sdlc-skills.test.ts`: mỗi role load đúng skill; bảng sở hữu tài liệu; mẫu gói bàn giao; quy tắc
  sửa tại chỗ và ID ổn định.
- Service handoff (Go, dùng `server/internal/testutil`): idempotent khi gọi hai lần; từ chối thiếu gói, thiếu dự án đích,
  chu trình; `off`, `ask`, `auto` mỗi chế độ một ca; `paused`; không đụng tới dòng của squad khác (kiểm theo id fixture của
  mình, không theo tổng).
- Diễn tập trên agent thật (cần bạn cho phép chạy, vì tốn quota):
  - R1, Mode X: "bổ sung tính năng X" trên một repo có sẵn PRD, architecture, UX, epics. Kỳ vọng: impact map; chỉ tài liệu
    bị chạm có issue; ID cũ giữ nguyên; epic mới ở cuối; Plan review PASS.
  - R2, Mode N: ý tưởng trống, có yêu cầu nghiên cứu thị trường. Kỳ vọng: báo cáo nghiên cứu có nguồn và ngày, brief, PRD.
  - R3: `off`; request dừng ở Plan ready, không có request nào cho SDLC.
  - R4: `auto`; request đích xuất hiện trong dự án Build, SDLC Lead đọc gói, tạo issue theo Plan A, không hỏi lại boss.
  - R5: yêu cầu đảo ngược một quyết định đã chấp nhận; squad ra sprint change proposal và dừng chờ boss, không handoff.
- Số liệu trước/sau cho R4: thời gian từ Plan ready đến issue Build đầu tiên chạy; số lần boss phải can thiệp.

## 8. Rủi ro

| Rủi ro | Giảm thiểu |
| --- | --- |
| Handoff tự động đẩy một kế hoạch tồi vào build, tốn quota và tạo code sai | Handoff chỉ chạy sau Plan review PASS có link; `ask` làm bước đệm; công tắc mặc định `off`; yêu cầu đảo quyết định cũ không bao giờ tự chuyển |
| Skill BMAD hỏi người dùng nên agent treo | Bọc bằng `planning-*`: headless khi có, mặc định khuyến nghị và ghi giả định khi không |
| Sửa tại chỗ làm vỡ ID hoặc xoá phần đã build | Quy tắc ID ổn định, change log bắt buộc, Reviewer kiểm theo impact map, không xoá epic đã delivered |
| Thêm skill vào thư mục chung làm SDLC nhận skill thừa | Danh sách skill tường minh theo template, có test |
| Tên agent trùng starter company | Tiền tố `Blueprint`, có test chống trùng tên với `company-template.ts` |
| Hai squad ghi chung một thư mục | Commit đúng đường dẫn; request handoff xếp hàng; planning xong mới handoff |
| Định nghĩa workflow nhân bản giữa TS và Go | `AssignSquadProject` tra workflow theo tên, không thêm bản Go thứ hai cho planning |
| Giấy phép của skill vendor vào bundle | Kiểm từng skill nhóm 2 như đã làm với các skill khác; loại skill không phân phối lại được |

## 9. Quyết định cần bạn xác nhận

1. **Tên và số agent.** 7 agent (có Researcher riêng) hay 6 (gộp Researcher vào Analyst)? Đề xuất 7.
2. **Phạm vi handoff bản đầu.** Cặp cố định BMAD Planning → một squad SDLC, hay cho cấu hình bất kỳ cặp squad nào? Đề xuất
   bất kỳ cặp ở tầng dữ liệu (bảng đã tổng quát), nhưng UI bản đầu chỉ hiện đích là squad có dự án mở.
3. **Chế độ `ask`.** Giữ làm bước đệm giữa `off` và `auto`, hay chỉ `off` và `auto` như bạn mô tả? Đề xuất giữ `ask`, vì chi
   phí thêm nhỏ (proposal sẵn có) và nó là cách an toàn để bật lần đầu.
4. **Phần chưa kiểm chứng.** (a) Skill nhóm 2 có phân phối lại trong bundle được không (giấy phép); (b) request đích tạo bằng
   actor hệ thống có làm gì khác với trigger squad leader so với người dùng giao việc không; (c) trạng thái cuối của
   request planning trong workflow "BMAD Planning Auto" là `Plan ready` như SDLC. Ba điều này kiểm ở đầu giai đoạn 1 và 3
   trước khi viết code.
