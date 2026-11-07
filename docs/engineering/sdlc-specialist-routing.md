# SDLC squad: the Lead routes every problem to the specialist who owns it

Spec. The build order is in [sdlc-specialist-routing-plan.md](sdlc-specialist-routing-plan.md).
It changes the SDLC squads (Greenfield and Maintenance) created by the quickstart. Related:
[issue-wakeups.md](issue-wakeups.md), [digital-company.md](digital-company.md).

Ngôn ngữ: tài liệu viết bằng tiếng Việt để người dùng đọc; tên lệnh, tên status, tên file và
các đoạn văn bản dành cho agent giữ nguyên tiếng Anh, vì skill của agent viết bằng tiếng Anh.

## 1. Tóm tắt

Hiện tại một squad SDLC vận hành như sau: Lead tạo issue, Developer làm, Reviewer duyệt, QA kiểm.
Khi có gì chặn, issue sang `blocked`, Lead được gọi, và **nước đi duy nhất của Lead là gửi issue
về `in_progress` cho Developer với "một cách khác"**, hoặc xin ý kiến người dùng bằng một proposal.
DevOps, Architect, Product Analyst, UX, Security chỉ làm việc trong giai đoạn lập kế hoạch hoặc cuối
dự án, và trong dự án thật chúng chưa chạy lần nào.

Spec này biến Lead thành một người điều phối thật: khi một vấn đề xuất hiện, bất kể loại gì
(hạ tầng, công cụ, nghiệp vụ, kiến trúc, trải nghiệm, bảo mật, kiểm thử, chất lượng review), Lead
**phân loại, giao cho đúng chuyên gia bằng một issue có ranh giới rõ, chờ phán quyết (ruling) có
bằng chứng, kiểm tra phán quyết, rồi cho Developer chạy tiếp**. Người dùng chỉ được hỏi khi việc đó
thực sự không thể đảo ngược hoặc chỉ con người mới có thẩm quyền.

Ba điểm mấu chốt:

1. **Một phân loại blocker chung** (blocker class) mà mọi vai trò dùng khi báo bị chặn, và một bảng
   định tuyến lớp → chuyên gia mà Lead dùng.
2. **Mỗi chuyên gia có thêm "Ruling mode"**: chế độ trả lời một câu hỏi hẹp, ra một phán quyết có
   bằng chứng và có lệnh để tiếp tục. Riêng DevOps có thêm chế độ Environment (dựng và giữ môi trường
   dev/test), tách khỏi chế độ Release.
3. **Điều phối chủ động**: Lead kiểm tra mức sẵn sàng (môi trường, kiến trúc, yêu cầu, bảo mật thiết
   kế) *trước* khi epic bắt đầu, và coi một vòng review/QA bị trả lại lặp lại là tín hiệu để mời
   chuyên gia, không phải để Developer đoán tiếp.

## 2. Bằng chứng từ dự án thật (mua_ngon_ecommerce)

Đọc trực tiếp từ database của máy này, chỉ đọc. Dự án chạy từ 2026-11-05 đến 2026-11-07.

| Quan sát | Số liệu |
| --- | --- |
| Số lần chạy theo agent | Developer 67, Code Reviewer 29, Lead 24, QA 12 |
| Agent chưa chạy lần nào | Architect, Product Analyst, UX Designer, DevOps, Security Reviewer (0 lần) |
| Số lần chuyển sang `blocked` (epic 1 đến 7) | 10 lần trên 5 issue (#79, #80 ×3, #95 ×2, #96 ×2, #97 ×2) |
| Nguyên nhân của 10 lần đó | 8 lần là môi trường, công cụ hoặc năng lực review: Docker/Postgres/Supabase local không chạy, `ProjectRefNotLinkedError`, thiếu `render_skill.py` của BMAD, không có công cụ dispatch subagent, không có reviewer độc lập. 2 lần là cây thư mục bẩn |
| Ai xử lý các lần block đó | Lead lập proposal xin người dùng (TUPK-86, 107, 108, 111) hoặc người dùng tự chuyển trạng thái. Không lần nào có chuyên gia được gọi |
| Vòng review/QA bị trả lại | 13 lần `in_review` → `in_progress`, 2 lần `qa` → `in_progress`. Riêng Epic 1: 25 lần chạy, 5 lần review trả lại |
| Nội dung các lần trả lại | Nhiều finding thuộc thiết kế và yêu cầu chứ không phải lỗi gõ code: MOQ vượt số lượng yêu cầu, health surface báo RLS tốt khi bảng đã tắt RLS, audit không bất biến, AC "chạy thật với 9router" không thể kiểm bằng stand-in, thiếu trải nghiệm đồng ý của người mua. Tất cả quay về Developer, Architect và Analyst không được hỏi |

Suy luận (không phải số đo): các finding nêu ở dòng cuối cho thấy cùng một lỗ hổng điều phối ở
vùng nghiệp vụ và kiến trúc, không chỉ ở hạ tầng. Hạ tầng chỉ là nơi nó lộ ra sớm nhất vì block
môi trường dừng cứng.

## 3. Nguyên nhân gốc

| # | Nguyên nhân | Nơi nó nằm |
| --- | --- | --- |
| R1 | Lead chỉ có một nước đi cho build bị chặn: về `in_progress` cho Developer với cách khác. "Đổi thành viên" chỉ là một gạch đầu dòng, không có quy trình | [sdlc-squad-lead/SKILL.md](../../packages/core/squads/sdlc-skills/sdlc-squad-lead/SKILL.md) Step 7, `SDLC_BLOCKED_INSTRUCTIONS` trong [templates.ts](../../packages/core/squads/templates.ts) |
| R2 | Không có phân loại blocker. Mọi role báo bằng văn bản tự do, Lead phải đoán, và đoán là "cần quyết định của boss" | các skill role, `sdlc-delivery-core` |
| R3 | Luật `in_place` xếp mọi issue Lead thêm sau vào cuối chuỗi: "issue thêm sau chờ issue cuối cùng còn xếp hàng hoặc đang chạy". Một issue gỡ block xếp theo luật này chạy sau tất cả các epic và không bao giờ gỡ được block hiện tại | Lead skill Step 3 và `reference.md` |
| R4 | DevOps chỉ có chế độ release. Điều kiện tiên quyết ("đã qua review, test, security") và luật "khác thiết kế là drift, dừng" không thể áp cho việc dựng môi trường. Status `release_prep` đòi version/digest/rollback mới được xong | [sdlc-devops-release/SKILL.md](../../packages/core/squads/sdlc-skills/sdlc-devops-release/SKILL.md) |
| R5 | Triết lý "unattended: lấy mặc định" đã giao mọi quyết định còn bỏ ngỏ cho Developer, kể cả quyết định ràng buộc (schema, hợp đồng API, ý nghĩa của AC). Architect cũng được dặn "lấy mặc định, squad không chờ". Không còn đường chính thức để hỏi một chuyên gia | `sdlc-delivery-core` "Decisions and assumptions", `sdlc-architecture` bước 2 |
| R6 | Review/QA trả lại luôn đi thẳng về Developer theo `back_status_key`. Lead không được gọi, nên không ai nhận ra một finding thuộc thiết kế hay yêu cầu, hay một vòng lặp đang lặp lại | workflow `in_review`, `qa` có `back: in_progress` |
| R7 | Lead không có tri thức cố định về đội: ai sở hữu gì, ai quyết gì, ai trả lời loại câu hỏi nào. Kiến thức nằm rải rác trong từng skill | không có tài liệu tổng hợp |
| R8 | Cách phối hợp chỉ có ở lúc lập kế hoạch. Plan A (đã có epic BMAD) bỏ qua mọi bước phân tích, kiến trúc, UX, và không có bước chuẩn bị môi trường nào trước khi build | Lead skill Step 2 và Plan A |

Hai rủi ro bổ sung phải xử lý trong thiết kế:

- **Năng lực, không chỉ vai trò.** Tất cả agent SDLC chạy trên cùng runtime Codex của máy này, và
  một số block ghi `docker ps` timeout. Chưa có bằng chứng sandbox chặn Docker, nhưng nếu có, DevOps
  cũng không làm được. Thiết kế có bước *capability probe* và một lớp blocker riêng (`capability`)
  dành cho việc này.
- **Dịch vụ chết giữa các lần chạy.** Ở TUPK-95, stack Supabase có lúc chạy rồi sau đó không còn
  phục vụ PostgreSQL. Dựng môi trường một lần là không đủ: môi trường phải tự dựng lại được.

## 4. Mục tiêu và ngoài phạm vi

Mục tiêu:

- G1. Mọi blocker thuộc một lớp xác định và được chuyên gia sở hữu lớp đó xử lý trước khi hỏi người dùng.
- G2. Mỗi vòng chuyên gia có đầu vào, đầu ra, và bằng chứng kiểm được; Lead không chấp nhận phán quyết suông.
- G3. Developer, Reviewer, QA biết khi nào được tự quyết và khi nào phải xin phán quyết.
- G4. Vòng lặp có giới hạn: không có chuyên gia hỏi chuyên gia vô hạn, không có chi phí chạy mất kiểm soát.
- G5. Môi trường dev/test được dựng, ghi lại, tự kiểm và tự dựng lại bằng một lệnh.
- G6. Lead chủ động: kiểm tra mức sẵn sàng trước epic và nhận ra vòng trả lại lặp lại.
- G7. Squad đã tạo (như SDLC Greenfield của dự án mua_ngon_ecommerce) nâng cấp được mà không tạo lại.

Ngoài phạm vi:

- Thay đổi quyền của người dùng (boss): production deploy, xoá dữ liệu, chi tiền, chấp nhận rủi ro
  bảo mật, cấp credential vẫn chỉ thuộc con người.
- Thay đổi mô hình hàng đợi `run_queued`/`run_after_issue_ids` hoặc cơ chế khoá thư mục `in_place`.
- Chuyên gia mới (không thêm vai trò). Tám vai trò hiện có đủ cho các lớp ở mục 6.1.
- Tự động hoá phía server cho phân loại (xem Giai đoạn 4 trong plan, chỉ làm khi có buy-in).

## 5. Nguyên tắc thiết kế

1. **Đúng người, đúng câu hỏi.** Mỗi lớp vấn đề có một chủ. Developer không đoán thay chủ.
2. **Hỏi hẹp, trả lời có bằng chứng.** Issue giao cho chuyên gia chứa một câu hỏi, các ràng buộc
   không được đổi, đầu ra mong đợi và lệnh để tiếp tục. Phán quyết phải có file/dòng hoặc output lệnh.
3. **Lead không tự làm việc của chuyên gia.** Lead vẫn không viết code, thiết kế, spec, test. Nó
   phân loại, giao, kiểm tra, cho tiếp tục.
4. **Unattended vẫn đúng.** Chuyên gia ra phán quyết chứ không chờ người; phán quyết được ghi để
   đảo ngược được (người, lý do, chi phí đảo ngược). Người dùng chỉ vào cuộc ở các lớp dành cho con người.
5. **Một nguồn sự thật.** Quy trình sống trong skill. Brief của workflow chỉ trỏ tới skill, không
   chép lại quy trình (hiện bản chép trong `templates.ts` và `initiative_build.go` đã có nguy cơ lệch).
6. **Có giới hạn và có lối thoát.** Số vòng, độ sâu, thời gian đều bị chặn; hết giới hạn thì lên
   người dùng *kèm phán quyết đã có và khuyến nghị*, không phải một câu hỏi trần.
7. **Ghi lại để đội nhớ.** Phán quyết, giả định và môi trường nằm trong repo (`docs/sdlc/`), mọi
   role đọc trước khi hỏi lại.

## 6. Thiết kế

### 6.1 Phân loại blocker và bảng định tuyến

| Class | Dấu hiệu | Chuyên gia | Start status | Đầu ra phán quyết |
| --- | --- | --- | --- | --- |
| `environment` | Service, DB, cổng, container, biến môi trường, seed, runner không chạy hoặc không tái dựng được | DevOps | `environment` (mới) | Môi trường chạy được + `docs/sdlc/environment.md` + lệnh `env-ensure` + health output |
| `tooling` | Thiếu công cụ/skill/script (ví dụ `render_skill.py`, CLI, trình duyệt), công cụ chạy sai | DevOps | `environment` | Công cụ cài/đặt/thay thế được, hoặc cách làm tương đương, có bằng chứng chạy |
| `requirement` | AC mơ hồ, mâu thuẫn, không đo được; thiếu quy tắc nghiệp vụ; hai nguồn yêu cầu đối nghịch | Product Analyst | `analysis` | Quy tắc/AC được làm rõ hoặc sửa trong spec, có id, lý do, chi phí đảo ngược, danh sách phần đã build bị ảnh hưởng |
| `design` | Chọn kiến trúc, hợp đồng API, mô hình dữ liệu, ranh giới thành phần, đánh đổi NFR; finding lặp lại có gốc thiết kế | Architect | `architecture` | ADR (accepted bởi squad), hợp đồng đã đóng băng, kế hoạch thay đổi nhỏ nhất cho Developer |
| `ux` | Trạng thái, luồng, nội dung, truy cập thiếu hoặc mâu thuẫn | UX Designer | `design` | Spec màn hình/trạng thái/copy được cập nhật |
| `security` | Finding bảo mật, thiết kế kiểm soát truy cập, xử lý dữ liệu cá nhân, câu hỏi tuân thủ | Security Reviewer | `security_review` | Phán quyết kiểm soát cần có, bằng chứng, owner của từng fix. Không cấp waiver |
| `verification` | Không có cách kiểm AC; oracle không rõ; test hoặc bằng chứng không đủ | QA Tester | `qa` | Kế hoạch kiểm tra thực thi được hoặc finding "AC không đo được" trả cho Analyst |
| `defect` | Lỗi chưa rõ nguyên nhân hoặc phạm vi | Bug Fixer, hoặc Architect nếu ảnh hưởng rộng | `todo` / `architecture` | Nguyên nhân gốc + sửa nhỏ nhất, hoặc phân tích tác động |
| `rework-loop` | Cùng một vấn đề bị trả lại từ review/QA từ lần thứ ba | Chuyên gia theo gốc: Architect (thiết kế), Analyst (AC), QA (kiểm thử) | theo lớp | Phán quyết nguyên nhân gốc và thay đổi cách tiếp cận, kèm tiêu chí thoát vòng |
| `review-capacity` | Cần reviewer độc lập, ý kiến thứ hai | Code Reviewer (lượt chạy mới, không thấy verdict cũ) | `plan_review` | Verdict độc lập cho cùng khoảng commit |
| `capability` | Runtime/sandbox/host của agent không cho làm việc cần thiết (docker socket, mạng, quyền, model) | DevOps *probe* trước, rồi người dùng | `environment` | Kết quả probe + yêu cầu cụ thể cho người dùng (cấu hình nào cần đổi) |
| `irreversible` | Xoá dữ liệu, production, chi tiền, credential/tài khoản của con người, waiver rủi ro, quyết định pháp lý | Người dùng (boss) | proposal | Quyết định của người dùng |

Quy tắc phân loại:

- Role báo block phải ghi một class. Lead được phép phân loại lại có lý do.
- Một blocker có nhiều lớp thì tạo một issue cho mỗi lớp, thứ tự theo phụ thuộc
  (ví dụ `requirement` trước `design`, `environment` trước `verification`).
- Không có chuyên gia trong roster cho một lớp: dùng bảng thay thế ở 6.9, nếu không thì lên người dùng.
- Cây thư mục bẩn không phải một lớp blocker. Luật hiện có ("không bao giờ là blocker") giữ nguyên.

### 6.2 Quyền quyết định (decision rights)

Thay đoạn "Business, product, naming, and design choices ... are yours" trong
`sdlc-delivery-core` bằng bảng sau.

| Loại quyết định | Ai quyết | Cách |
| --- | --- | --- |
| Cục bộ: đặt tên trong module, cấu trúc nội bộ, thư viện đã nằm trong ADR, copy theo spec, cấu trúc test | Role đang làm | Tự quyết, ghi assumption |
| Ràng buộc: thêm/đổi schema ảnh hưởng dữ liệu hiện có, hợp đồng API công khai, mô hình quyền, dịch vụ hoặc phụ thuộc ngoài mới, thành phần hạ tầng mới, đánh đổi NFR | Architect | Có ADR/ruling sẵn thì dùng. Chưa có thì `Blocker-class: design` |
| Ràng buộc: ý nghĩa của AC, quy tắc nghiệp vụ (giá, đồng ý, hoa hồng, hạn mức), cắt hoặc thêm phạm vi | Product Analyst | Có spec/ruling sẵn thì dùng. Chưa có thì `Blocker-class: requirement` |
| Trải nghiệm: luồng, trạng thái, nội dung người dùng thấy ngoài spec | UX Designer | tương tự |
| Kiểm soát bảo mật, xử lý dữ liệu cá nhân | Security Reviewer | tương tự |
| Cách dựng, chạy, kiểm môi trường | DevOps | `Blocker-class: environment` |
| Không thể đảo ngược, chi tiền, credential, waiver | Người dùng | proposal có khuyến nghị |

Giữ nguyên luật hiện có làm lối thoát cho phần còn lại: *khi tồn tại phương án không đổi schema, API,
hành vi đã phát hành thì chọn nó và ghi assumption.* Chỉ khi mọi phương án đều chạm vào các thứ đó
thì mới cần phán quyết. Điều này giữ chi phí ở mức thấp: hầu hết quyết định nhỏ không sinh thêm lượt chạy nào.

### 6.3 Báo cáo blocker (mọi role)

Thêm vào `sdlc-delivery-core`, mục "When you cannot proceed". Đây là phần đầu của comment khi set
`blocked` (sau `multica issue status <id> blocked`, như luật hiện có):

```
BLOCKED
Blocker-class: environment | tooling | requirement | design | ux | security | verification | defect | rework-loop | review-capacity | capability | irreversible
Blocked-from: <status you were working in>
Symptom: <exact command and error, or the two conflicting sources with file:line>
Tried: <approach, result> (every approach tried on this issue)
Question: <the one question the specialist must answer, or the one action needed>
Resume-check: <the command or check that proves this is resolved>
```

Lý do mỗi trường: `Blocker-class` cho Lead định tuyến mà không đoán; `Blocked-from` cho Lead biết đưa
issue về đâu (Lead không thể đọc lịch sử trạng thái đáng tin cậy từ trong run); `Question` buộc
người báo hỏi một câu hẹp; `Resume-check` là điều kiện kiểm được để biết đã xong.

### 6.4 Giao việc cho chuyên gia (Lead)

Issue gỡ block (gọi là *unblock issue*) do Lead tạo, thay Step 7 hiện tại.

Tạo issue:

- Cùng Build project, **không có parent** (tránh ngữ nghĩa cha/con của hàng đợi và
  `reconcileUnmovedWorkflowRun`; xem mục 11 câu hỏi mở Q3), tiêu đề `[Unblock <identifier>] <class>: <question rút gọn>`.
- Gán theo id từ Squad Roster. Tạo ở `backlog`, rồi `multica issue queue <issue> --start-status <status>`
  **không có `--after`**, đúng đường đã chạy ổn ở Step 3. Issue bị block đã rời `backlog` nên server
  đã xoá `run_queued` và các lần chờ của nó (commit `c29c33d65`); các epic xếp hàng phía sau vẫn chờ
  *chính epic đó* và không bị ảnh hưởng.
- **Ngoại lệ có chủ đích với luật `in_place`.** Luật "issue thêm sau chờ issue cuối" áp dụng cho
  gap/fix issue của dòng build. Unblock issue không chờ gì: epic bị block đang đỗ (không có run
  nào), nên không có hai run sửa cùng một working copy. Server giữ các issue đã xếp hàng khác trong
  lúc một issue không xếp hàng của project đang chạy agent (`ListReleasableQueuedIssues`), nên
  tính một-việc-một-lúc vẫn đúng.
- Mô tả theo mẫu:

```
Unblock ruling for <mention://issue/...> (<class>, round <n> of 2)
Question: <from the blocker report>
Evidence: <the blocker report, file:line, command output>
Do not change: <acceptance criteria, accepted decisions, applied migrations, ...>
Output: <ruling artifact paths per the class table> and a Ruling comment (format below)
Resume: <status to put the blocked issue in> and <Resume-check command>
Load: sdlc-delivery-core and <role skill>; follow its "Ruling mode".
This run is unattended. Do not wait for approval; take the recommended default and record it as an assumption.
```

Đăng ký đánh thức:
`multica issue wakeup create <blocked issue> --until-issue <unblock issue> --until-issue-state ended --expires-in 72h --on-timeout wake --instruction "<read the ruling; verify; resume or route again>"`.
Epic bị block đang giao cho Lead (bước Blocked tự giao), nên wakeup chạy Lead trên đúng issue đó.
Lead kết thúc run của mình ngay sau khi đăng ký; không có gì chờ trong run.

### 6.5 Phán quyết (ruling) của chuyên gia

Cuối issue, một comment duy nhất (thêm vào `sdlc-delivery-core`, "Report format"):

```
Ruling: <one-line decision, or the state achieved>
Class: <class>  For: <mention://issue/...>
Basis: <evidence: file:line, command and output, source documents>
Binding artifacts: <ADR / spec / runbook / commit ids>
Impact: <what already built must change, and what must not change>
Resume: <status> and <Resume-check, with the command>
Residual risk / not verified: <list>
Needs next: <another class, if the ruling exposes one, else "none">
```

Chuyên gia ghi một dòng vào `docs/sdlc/rulings.md` (id R-n, class, câu hỏi, phán quyết, issue, ngày,
cách đảo ngược). Mọi role đọc file này trước khi hỏi (luật mục 6.2 "có ruling sẵn thì dùng").

### 6.6 Kiểm tra và tiếp tục (Lead)

Khi được đánh thức:

1. Mở các artifact trong `Binding artifacts` và chạy được `Resume-check` hoặc xác nhận bằng chứng
   của nó. Phán quyết không có bằng chứng tính là một vòng thất bại (đúng với luật hiện có "báo cáo
   rỗng là không tiến triển").
2. Kiểm tra phán quyết không sửa AC hay quyết định đã chấp nhận nếu không phải Analyst/Architect có
   thẩm quyền, và không mâu thuẫn `rulings.md` hoặc ADR. Mâu thuẫn thì tạo issue cho đúng chuyên gia
   giải quyết, không tự chọn.
3. Đưa issue về `Blocked-from` (thường `in_progress`; `in_review` hoặc `qa` khi chỉ thiếu một kiểm
   tra), bằng `multica issue status`. Comment: phán quyết, đường dẫn, lệnh Developer/QA phải chạy
   trước tiên, và điều **không được** đổi.
4. `Needs next` khác `none` thì quay lại 6.4 cho lớp đó trước khi tiếp tục.

### 6.7 Giới hạn và lên người dùng

| Giới hạn | Giá trị |
| --- | --- |
| Vòng phán quyết cho một lớp trên một issue | tối đa 2 |
| Tổng vòng chuyên gia cho một issue | tối đa 4 |
| Độ sâu (chuyên gia báo blocked tiếp) | tối đa 2 |
| Thời gian chờ một unblock issue | 72 giờ, rồi wakeup đánh thức Lead |

Hết giới hạn, hoặc lớp là `irreversible` / `capability` (phần chỉ con người làm được): Lead lập
proposal bằng `multica proposal create --squad`, và proposal **phải** chứa: blocker, các phán quyết
đã có, phương án Lead khuyến nghị, và câu hỏi hẹp. Dẫn issue bị block trong evidence để
nền tảng tự đưa issue về trạng thái khởi đầu khi người dùng quyết (cơ chế đã có,
commit `1b78a2a2e`).

### 6.8 Điều phối chủ động

**Kiểm tra mức sẵn sàng khi khởi động (Lead, Step 2 mới).** Trước khi xếp epic đầu tiên, Lead đánh
giá các điều kiện sau từ bằng chứng trong repo, ghi kết quả và lý do bỏ qua vào comment kế hoạch:

| Điều kiện kích hoạt | Issue được tạo | Chuyên gia |
| --- | --- | --- |
| Repo có phụ thuộc chạy (compose, supabase, migration, script `*:start`, `.env.example`, DB) | Environment bring-up, đứng đầu chuỗi | DevOps |
| Có epic nhưng không có tài liệu kiến trúc/ADR, hoặc epic nhắc thành phần không có trong kiến trúc | Architecture conformance check | Architect |
| AC thiếu, mơ hồ, hoặc `bmad-sprint-planning` readiness check chưa chạy | Requirements readiness check | Product Analyst |
| Sản phẩm có UI mà chưa có spec màn hình/trạng thái | UX readiness | UX Designer |

Điều kiện không kích hoạt thì không tạo issue: chi phí chỉ phát sinh khi bằng chứng cần.

**Kiểm tra trước epic (nhẹ, bước Lead đọc trước khi xếp epic).** Lead đọc epic và tạo thêm issue
chuyên gia chỉ khi có một trong: thành phần hạ tầng mới (DevOps delta), thay đổi hợp đồng/schema
xuyên epic (Architect), nhà cung cấp ngoài mới, hoặc epic thuộc risk tier High-risk (auth, tiền,
dữ liệu cá nhân, migration, hợp đồng công khai, kiểm soát bảo mật): *Security design review* ở
chế độ threat-model trước khi build, không chỉ ở cuối dự án.

**Tín hiệu vòng lặp (Reviewer, QA, Developer).**

- Mỗi finding chặn của Reviewer/QA có trường `Owner-role` (developer, architect, analyst, ux,
  security, devops, qa). Nếu có finding chặn mà owner khác developer, role đó **không** dùng
  REQUEST_CHANGES/NO GO thuần: set `blocked` với class tương ứng, liệt kê cả finding của developer.
- Developer đếm các comment `REQUEST_CHANGES`/`NO GO` trên issue. Từ lần trả lại thứ ba cho cùng
  một nguyên nhân, nó set `blocked` với `Blocker-class: rework-loop` thay vì tiếp tục đoán.

**Kiểm toán assumption.** Khi epic xong, Lead đọc các assumption mới trong `docs/sdlc/assumptions.md`;
assumption nằm trong bảng "ràng buộc" của 6.2 được giao cho chuyên gia tương ứng xác nhận (ADR
chuyển từ Proposed sang Accepted-by-squad, hoặc bị đổi). Hiện ADR ở trạng thái Proposed mãi vì không
ai chấp nhận trong squad không người.

### 6.9 Lead biết đội: Team Playbook và sổ dự án

Thêm file `sdlc-squad-lead/team-playbook.md` (cùng thư mục skill, được đóng gói như `reference.md`):

1. **Bản đồ năng lực** mỗi vai trò: sở hữu gì, quyết gì, nhận câu hỏi dạng nào, đầu vào, đầu ra,
   start status, không được làm gì. Đây là nơi duy nhất Lead tra "chuyên gia nào trả lời việc này".
2. **Bảng định tuyến** 6.1 và **bảng quyền quyết định** 6.2 (bản gốc nằm ở đây; `sdlc-delivery-core`
   trỏ về đây hoặc lặp lại phần ngắn, có test chống lệch ở mục 8).
3. **Mẫu** mô tả unblock issue (6.4) và phán quyết (6.5).
4. **Bảng thay thế khi thiếu vai trò trong roster:** `environment`/`tooling` → Developer chạy
   skill `sdlc-development` ở chế độ environment, ghi rõ trong comment; `design` → Code Reviewer
   làm design review và Developer ghi ADR Proposed; `requirement`/`ux`/`security` → lên người dùng
   (không có người thay thế an toàn). Quickstart luôn tạo đủ các vai trò, nên bảng này chủ yếu cho
   project do initiative tạo (server chỉ bảo đảm lead, developer, reviewer, QA).

Sổ dự án trong repo (Lead đọc đầu mỗi run, các role đọc trước khi hỏi):

| File | Nội dung | Ai ghi |
| --- | --- | --- |
| `docs/sdlc/environment.md` | cách dựng/chạy/kiểm/reset, cổng, stack cô lập cho QA, giới hạn đã probe | DevOps |
| `docs/sdlc/rulings.md` | mục lục các phán quyết | chuyên gia ra phán quyết |
| `docs/sdlc/assumptions.md` | đã có | mọi role |

### 6.10 Chế độ Environment của DevOps

`sdlc-devops-release` có hai chế độ. **Release** giữ nguyên toàn bộ luật hiện tại (điều kiện tiên
quyết, digest, phê duyệt production). **Environment** là chế độ mới, các luật Release (đã qua review,
drift, digest) *không áp dụng*:

0. **Capability probe.** Chạy và ghi kết quả: `docker version`, `docker ps`, mạng ra ngoài,
   cổng trống, đĩa, quyền ghi vào thư mục repo, các công cụ repo cần. Thiếu năng lực mà agent không
   tự sửa được (ví dụ sandbox chặn docker socket): đây là `capability`, lên người dùng với yêu cầu
   cụ thể (cấu hình nào cần đổi), không phải thử lại.
1. **Kiểm kê.** Từ README, AGENTS.md, `package.json`, compose, cấu hình Supabase, `.env.example`:
   dịch vụ, cổng, biến (chỉ tên, không bao giờ in giá trị bí mật), seed, migration.
2. **Dựng theo đúng cách repo tài liệu hoá.** Ưu tiên dịch vụ chạy trong container/daemon
   (`docker compose up -d`, `supabase start`), vì tiến trình nền khởi động trong một run agent có thể
   không sống qua khi run kết thúc.
3. **Môi trường là code, tự phục hồi.** Nếu repo chưa có, tạo một lệnh idempotent (ví dụ
   `scripts/env-ensure.sh` hoặc một target Makefile) làm: kiểm tra sức khoẻ, dựng lại phần thiếu,
   in kết quả. Mọi role chạy lệnh này làm bước đầu của kiểm tra runtime. Chỉ được sửa file hạ tầng/
   tooling/tài liệu, không sửa mã ứng dụng.
4. **Stack cô lập cho QA.** Cung cấp stack thứ hai khác cổng/project id để suite cô lập của QA không
   cần dừng stack dùng chung. Không bao giờ dừng hay sửa một stack mà nó không tự khởi động
   (khớp với runner của dự án: "Stop local Supabase before running this isolated suite").
5. **Bằng chứng.** Health check chạy được, output lưu, và chạy lại từ trạng thái dừng để chứng minh
   dựng lại được.
6. **Ghi `docs/sdlc/environment.md`** và comment Ruling với `Resume-check` là lệnh `env-ensure`.

Không làm: đụng credential/tài khoản thật, deploy staging hay production, đổi mã ứng dụng, in bí mật.

### 6.11 Ruling mode của từng chuyên gia

Mỗi skill thêm một mục "Ruling mode (unblock issues)" ngắn, cùng cấu trúc: *đầu vào* (issue mô tả),
*việc làm*, *ranh giới*, *đầu ra*, *tự kiểm*.

| Skill | Ruling mode làm gì | Ranh giới |
| --- | --- | --- |
| `sdlc-product-analysis` | Làm rõ/sửa AC hoặc quy tắc nghiệp vụ trong spec với id, lý do, chi phí đảo ngược; cập nhật traceability; liệt kê phần đã build bị ảnh hưởng. Thay đổi cỡ Medium trở lên đi qua `bmad-correct-course` | Không đưa giải pháp/công nghệ vào yêu cầu. Đây là kênh *chính thức* duy nhất để sửa một AC đã chấp nhận (hiện bị cấm không có đường đi) |
| `sdlc-architecture` | Ra ADR (Accepted-by-squad) cho quyết định ràng buộc, đóng băng hợp đồng, phân tích tác động và gốc rễ cho `rework-loop`, kế hoạch thay đổi nhỏ nhất cho Developer | Không đổi yêu cầu, không viết code sản xuất |
| `sdlc-ux-design` | Cập nhật spec màn hình, trạng thái, nội dung | Không đổi yêu cầu |
| `sdlc-security-review` | Phán quyết kiểm soát cần có, phân loại finding, threat-model thiết kế trước khi build | Không cấp waiver hay chấp nhận rủi ro (thuộc người) |
| `sdlc-qa-verification` | Thiết kế cách kiểm cho AC khó đo, oracle, hoặc finding "AC không đo được" trả Analyst | Không sửa mã sản phẩm |
| `sdlc-bug-fix` | Nguyên nhân gốc và sửa nhỏ nhất cho `defect` | Sửa cỡ Medium trở lên thì escalate |
| `sdlc-code-review` | Ý kiến thứ hai độc lập trên cùng khoảng commit | Không thấy verdict trước; không sửa code |
| `sdlc-devops-release` | Chế độ Environment ở 6.10 | Như 6.10 |
| `sdlc-development` | Không có ruling mode. Thêm: khi nào tự quyết và khi nào chặn (6.2), đếm vòng trả lại (6.8), chạy `env-ensure` đầu kiểm tra runtime | Developer vẫn không đổi AC, không bỏ test |

## 7. Thay đổi theo thành phần

| Thành phần | Thay đổi |
| --- | --- |
| Skills (`packages/core/squads/sdlc-skills/*`) | Mọi mục ở 6.2 đến 6.11. File mới: `sdlc-squad-lead/team-playbook.md`. Viết lại Step 7 của Lead, thêm bước khởi động và kiểm tra trước epic, sửa ngoại lệ `in_place` trong Step 3 và `reference.md` |
| `sdlc-skills-data.ts` | Sinh lại bằng `pnpm generate:sdlc-skills`, không sửa tay |
| `templates.ts` (`SDLC_AUTO_WORKFLOW`) | Thêm step `Environment` (handler `devops`, `next: stage_done`, `back: blocked`). Viết lại `planningBrief` để mô tả "planning stage hoặc unblock ruling, mô tả issue nói loại nào". `SDLC_BLOCKED_INSTRUCTIONS` rút xuống vài dòng trỏ tới skill Lead. Brief của `in_review` và `qa` thêm yêu cầu Blocker report và `Owner-role` |
| `sdlc-agent-instructions.ts` | Instruction của Lead bỏ phần nhắc lại quy trình trùng skill, trỏ tới Blocked work; Developer, DevOps (giới hạn "digest, approval" thuộc chế độ Release) cập nhật |
| `initiative_build.go` | Dựng lại `sdlcSteps` cho bằng bản TypeScript: thêm các status lập kế hoạch và `environment`, tạo status còn thiếu như đã làm với QA (`ensureQAStatus`), và chỉ chép brief ngắn. Hiện bản server chỉ có build + blocked, nên Lead của project do initiative tạo không có status hợp lệ để giao chuyên gia (`start_status must be a status of the project's workflow`). `sdlcRoleAgents` giữ yêu cầu tối thiểu, các chuyên gia thiếu thì dùng bảng thay thế |
| Quickstart (`quickstart.ts`) | Hiện workflow đã tồn tại thì **không bao giờ được sửa** (`outcome.workflow = existing`), nên squad đã tạo không nhận được step mới. Thêm đường nâng cấp (mục 9) |
| Tests | Mục 8 |
| Docs | Cập nhật `docs/engineering/digital-company.md` nếu nó mô tả luồng blocked của SDLC |

## 8. Kiểm chứng và chống lệch

Test tự động (không chạy agent CLI thật, đúng quy định AGENTS.md):

- `sdlc-skills.test.ts`: bundle sinh ra khớp thư mục (đã có); mọi skill chuyên gia có tiêu đề "Ruling mode";
  `team-playbook.md` nằm trong bundle của Lead.
- Test mới "routing table": parse bảng định tuyến trong `team-playbook.md`; mọi `start status` tồn
  tại trong `SDLC_AUTO_WORKFLOW` (hoặc là proposal), mọi vai trò đích có trong `SQUAD_ROLES` của
  template Greenfield và Maintenance.
- Test chống lệch danh sách class: tập `Blocker-class` trong `sdlc-delivery-core`, `team-playbook.md`
  và các brief workflow là cùng một tập (một hằng số trong test).
- `templates.test.ts`: step `environment` tồn tại, handler `devops`, `back: blocked`; brief blocked không
  còn chép quy trình; step build vẫn như cũ.
- Go (`initiative_build_steps_test.go`): `sdlcSteps` có đủ status và handler như TypeScript; blocked vẫn do lead xử lý;
  các status được tạo.
- Test nâng cấp workflow (mục 9) trên dữ liệu có sẵn: thêm step thiếu, giữ handler đã tuỳ biến, không
  ghi đè brief do người dùng sửa.

Diễn tập thật (thủ công, trên project thử, không phải test tự động) ở Giai đoạn 3 của plan: sáu kịch
bản, mỗi kịch bản có kỳ vọng rõ về chuyên gia được gọi.

Chỉ số thành công, đọc lại từ chính database như ở mục 2:

- Số lần chạy của mỗi chuyên gia > 0 khi có blocker thuộc lớp của họ.
- Số proposal xin hạ tầng/công cụ/làm rõ nghiệp vụ gửi người dùng: giảm về 0 cho các lớp có chuyên gia.
- Số lần `blocked` lặp lại cùng `Symptom` sau một phán quyết: 0 (hai lần là cờ đỏ).
- Số vòng `in_review` → `in_progress` trên mỗi epic: giảm, và mỗi vòng thứ ba trở đi đều có `rework-loop`.
- Thời gian từ `blocked` đến `in_progress` hợp lệ trung bình: ghi lại trước và sau.

## 9. Nâng cấp squad đã tạo

Squad SDLC Greenfield của dự án mua_ngon_ecommerce được tạo từ phiên bản trước. Đã xác nhận trong DB:
skill của squad khớp bản trong repo (chỉ khác ký tự xuống dòng cuối), workflow `SDLC Auto` có
đủ các status lập kế hoạch và `blocked` do `SDLC Lead` xử lý.

| Thứ cần nâng | Cách | Rủi ro |
| --- | --- | --- |
| Skill | Chạy lại quickstart; `attachBuiltinSkills` cập nhật skill do quickstart tạo ra (cờ `sdlc_built_in`) | Skill do người dùng tự sửa cùng tên sẽ không bị ghi đè và được báo lỗi |
| Workflow | Đường mới: so sánh workflow hiện tại với template, **thêm** step thiếu (`environment`), thay brief chỉ khi brief hiện tại đúng bằng một bản cũ đã biết (băm), còn lại giữ và báo. Dùng `dryRunIssueWorkflowUpdate` trước, rồi `updateIssueWorkflow` | Cần quyền admin để tạo status `Environment` ("workspace admin must allow") |
| Instruction của agent và của squad | Đã xác minh: quickstart **không** cập nhật agent đã tồn tại (`status: "reused"`) và không sửa `squad.instructions`. Đường nâng cấp thêm bước này, chỉ ghi đè khi instruction hiện tại bằng một bản cũ đã biết (băm), còn lại giữ và báo | Lead hiện còn chứa quy trình trùng skill |
| Status | Tạo `Environment` (category `started`) nếu chưa có | Như trên |

Không cần di chuyển dữ liệu issue: các issue đang chạy giữ nguyên status của chúng.

## 10. Rủi ro và giảm thiểu

| Rủi ro | Giảm thiểu |
| --- | --- |
| Chi phí chạy tăng vì mỗi blocker sinh thêm lượt chuyên gia | Chỉ quyết định *ràng buộc* mới cần ruling (6.2); điều kiện kích hoạt chủ động chỉ theo bằng chứng (6.8); giới hạn vòng (6.7). So sánh số lượt chạy trước/sau theo mục 8 |
| Developer chặn quá dễ để tránh quyết định | Bảng 6.2 liệt kê chính xác loại ràng buộc; luật "có phương án không đổi schema/API thì chọn nó" giữ nguyên |
| Phân loại sai | Lead được phân loại lại; `Question` và `Symptom` bắt buộc; hai vòng sai thì đổi chuyên gia |
| Chuyên gia ra phán quyết suông | Lead kiểm artifact và `Resume-check`; không có bằng chứng thì tính là vòng thất bại |
| Hai chuyên gia mâu thuẫn | Trình cả hai cho người dùng, không tự chọn (luật hiện có "two reviewers disagree") |
| Chuyên gia chưa từng chạy có thể lỗi ở đường chưa dùng (0 lần chạy trong dữ liệu thật) | Diễn tập Giai đoạn 3 chạm từng chuyên gia một lần trước khi tin cậy |
| Agent `offline` trên bảng (UX, Architect, Security, DevOps) | Runtime của chúng đang `online`, nhưng việc giao cho agent offline chưa được kiểm trong dữ liệu thật. Diễn tập xác minh (Q2) |
| Sandbox Codex chặn Docker/mạng, DevOps cũng không làm được | Capability probe + lớp `capability` với yêu cầu cụ thể cho người dùng |
| Dịch vụ chết giữa các run | Lệnh `env-ensure` idempotent, mọi role chạy đầu kiểm tra runtime |
| Vòng lặp giữa các chuyên gia | Độ sâu tối đa 2, tổng 4 vòng, `Needs next` do Lead xử lý chứ không tự giao nhau |
| Brief workflow lệch skill | Brief chỉ trỏ tới skill; test chống lệch; server và client dùng cùng nội dung ngắn |

## 11. Quyết định đã chọn và phương án đã loại

| # | Quyết định | Phương án bị loại và lý do |
| --- | --- | --- |
| D1 | Unblock là issue riêng, không parent, tạo bằng `queue` không `--after` | Gán lại *chính issue bị block* sang chuyên gia: workflow chỉ có một `next` cố định cho mỗi step (`stage_done` là trạng thái *xong*, kết thúc epic), không có "quay về nơi đã đến". Issue con có parent: ngữ nghĩa cha/con của hàng đợi và `reconcileUnmovedWorkflowRun` chưa được kiểm (Q3) |
| D2 | Quy trình sống trong skill, brief chỉ trỏ tới | Phân loại và giao việc ở server: cứng, khó thay đổi, và AGENTS.md yêu cầu giải thích chi phí trước khi thêm phức tạp. Để dành cho Giai đoạn 4 khi có số liệu |
| D3 | Thêm status `Environment` | Dùng lại `release_prep`: brief của nó đòi version/digest/rollback, trái với việc dựng môi trường |
| D4 | Phân loại bằng văn bản có cấu trúc trong comment | Lưu `blocker_class` ở `issue.properties` ngay: tốt cho thống kê nhưng cần thay đổi API, schema và client; chưa có bằng chứng cần thiết. Giai đoạn 4 |
| D5 | Quyết định ràng buộc đi qua chuyên gia, quyết định cục bộ vẫn tự quyết | Mọi quyết định đều hỏi chuyên gia: chi phí quá cao, phá tinh thần unattended |
| D6 | Giữ 8 vai trò hiện có | Thêm vai trò mới (ví dụ "Platform engineer"): DevOps ở chế độ Environment đã phủ lớp này |

### Câu hỏi mở (phải xác minh trước hoặc trong lúc làm)

- **Q1 (đã giải quyết).** Quickstart không cập nhật `instructions` của agent hay squad đã tồn tại; xem mục 9.
- **Q2.** Một issue giao cho agent đang `offline` nhưng runtime `online` có được dispatch không? Dữ liệu thật chưa có ca nào cho UX, Architect, Security, DevOps.
- **Q3.** Tạo unblock issue có parent là issue bị block có gây ra rào cản giai đoạn (stage barrier), `isSquadRequest`
  hay `reconcileUnmovedWorkflowRun` không? Thiết kế chọn không parent để khỏi phải trả lời; chỉ đổi nếu thấy lợi ích khi truy vết.
- **Q4.** Wakeup `--until-issue` gắn vào issue đang giao cho Lead (khác với cách dùng hiện tại, gắn vào request) có hoạt động đúng không.
- **Q5.** Sandbox của runtime Codex có cho `docker`, mạng, và ghi ngoài thư mục làm việc không. Quyết định khả năng làm của DevOps.
