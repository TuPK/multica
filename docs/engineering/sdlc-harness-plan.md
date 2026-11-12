# SDLC squad: kế hoạch triển khai Harness v2

Kế hoạch triển khai cho [sdlc-harness-spec.md](sdlc-harness-spec.md). Căn cứ chọn phương án ở
[sdlc-harness-research.md](sdlc-harness-research.md). Thiết kế đang chạy ở [sdlc-harness.md](sdlc-harness.md).

Trạng thái: **kế hoạch, chưa thực hiện bước nào.** Tài liệu này được viết khi working tree có thay đổi chưa commit của việc khác
(mục 2). Trước khi bắt đầu bất kỳ bước nào, chạy lại `git status --short` và đối chiếu với mục 2.

Ngôn ngữ: viết bằng tiếng Việt để người dùng đọc; tên lệnh, file, tên kiểm tra và thuật ngữ kỹ thuật giữ nguyên tiếng Anh.

## 0. Trạng thái thực hiện (cập nhật 2026-11-12)

S0 đến S7 đã làm trực tiếp trong checkout này; S8 và S9 chưa làm (cần người dùng đồng ý). Chưa commit: việc thực hiện bị cấm chạy `git add`/`git commit`, nên
các thay đổi nằm trong working tree, chưa có commit nguyên tử theo từng bước. Phần "Kiểm tra bước xong" của mỗi bước dưới đây là lệnh gốc của plan; kết quả thật
ghi ở báo cáo cuối của lần thực hiện.

| Bước | Trạng thái | Khác với plan |
| --- | --- | --- |
| S0 | Xong | Repo hiệu chỉnh: multica, vnpt-software-factory, 9router, shots-rail (multica-kitviet trùng multica) vì người dùng không chỉ định repo khác. Tạo `sdlc-harness-calibration.md` |
| S1 | Xong | Như plan |
| S2 | Xong | Sửa thêm H7: bắt "is/are/was implemented" thay vì mọi "implemented". Fixture test chuyển sang mẫu v2 |
| S3 | Xong | Sửa lỗi thật của v1: `judgeCommand` xử lý mọi lệnh không phải `make` như `npm/pnpm/yarn`, nên `sh scripts/verify.sh` hay `go test` bị coi là thiếu script. Giờ chỉ bốn công cụ được phân giải |
| S4 | Xong | Thêm `--accept <ID>=<A-n>` (hạ bậc `cert` có ghi chép) và các trường `accepted`, `certificate` trong báo cáo; cert JSON có thêm `accepted` |
| S5 | Xong | Thêm vào test: mẫu v2 điền xong phải chứng nhận trong 80 dòng |
| S6 | Xong | Dòng đo cuối epic của Lead thêm vào `sdlc-squad-lead/SKILL.md`. Script chưa chạy trên server thật |
| S7 | Xong | `sdlc-harness.md` viết lại tại chỗ (không viết tài liệu thay thế) |
| S8, S9 | Chưa làm | Cần đồng ý |

## 1. Cách đọc

- **Lượt chạy agent** (ước lượng): một phiên agent triển khai đọc code, sửa, chạy test hẹp, tự sửa lỗi và dừng ở một commit. Khoảng
  ước lượng, không phải cam kết. Riêng S8 và S9 còn có "lượt chạy của squad" (nền tảng chạy agent thật) được ghi tách.
- Mọi lệnh chạy từ gốc repo `multica/`. Test chạy **hẹp** theo file, không chạy cả bộ `pnpm test`.
- Mỗi bước là một commit nguyên tử, theo conventional commits (ví dụ `feat(squads): ...`, `docs(squads): ...`), theo `AGENTS.md` của repo.
- Mỗi bước chạm thư mục `packages/core/squads/sdlc-skills/` phải chạy `pnpm generate:sdlc-skills` và commit
  `packages/core/squads/sdlc-skills-data.ts` cùng bước đó. Bài test trong `sdlc-skills.test.ts` ("This test fails when the generated file is
  stale") sẽ đỏ nếu quên. Kiểm tra bằng `git diff --stat -- packages/core/squads/sdlc-skills-data.ts`.
- **Đơn vị phát hành là S1 đến S5** (script, test và văn bản skill phải khớp nhau). Có thể commit riêng từng bước nhưng chỉ nâng cấp squad
  (`node scripts/upgrade-sdlc-squad.mjs --apply`) sau khi S5 xong.
- Không bước nào đổi `server/`, `packages/core/squads/templates.ts`, `server/internal/service/initiative_build.go`,
  `packages/core/squads/upgrade.ts` hay `packages/core/squads/upgrade.test.ts`: brief và status của stage Harness giữ nguyên (spec AC12).

## 2. Trạng thái git và những gì chưa đụng được

Kết quả `git status --short` ngày 2026-11-12 (chỉ đọc), HEAD `d56d8f516`, chụp lại ở cuối lúc viết tài liệu này. Danh sách **đang thay đổi
theo thời gian** vì việc Workflow tab vẫn chạy song song (lúc bắt đầu viết chỉ có 19 mục, lúc kết thúc có thêm các file `packages/views/**`):

```
 M packages/core/api/client.ts
 M packages/core/api/schemas.ts
 M packages/core/issue-workflows/queries.ts
 M packages/core/issue-workflows/schemas.test.ts
 M packages/core/squads/kitviet-agent-instructions.ts
 M packages/core/squads/kitviet-previous-texts.ts
 M packages/core/squads/kitviet-skills-data/common.ts
 M packages/core/squads/kitviet-skills/kv-squad-lead/SKILL.md
 M packages/core/squads/kitviet-stop-header.test.ts
 M packages/core/squads/upgrade.test.ts
 M packages/core/types/issue-workflow.ts
 M packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/workflow.json
 M packages/views/workflow/designer/workflow-designer-page.tsx
 M packages/views/workflow/designer/workflow-designer-page.test.tsx
 M server/cmd/server/router.go
 M server/internal/handler/issue_workflow.go
 M server/internal/handler/issue_workflow_test.go
 M server/internal/service/company_template_data.json
 M server/internal/service/issue.go
?? packages/views/workflow/designer/{clone-workflow-dialog,clone-workflow-dialog.test,clone-workflow-utils,clone-workflow-utils.test,
   discard-confirm-dialog,start-point-bar,template-mock,template-picker-dialog,template-picker-dialog.test}.ts(x)
?? server/internal/issueworkflow/templates.go
?? server/internal/issueworkflow/templates_test.go
?? server/internal/service/issue_create_enqueue_test.go
```

Đây là công việc của Workflow tab và Kit Việt. Không bước nào của plan chạm các file trên, trừ một ràng buộc:

| File chưa commit | Quan hệ với plan | Cách xử lý |
| --- | --- | --- |
| `packages/core/squads/upgrade.test.ts` | Có các khẳng định về harness (các dòng khoảng 73 đến 165: step `Harness` được thêm, skill `sdlc-harness` và `scripts/check-harness.mjs` được nạp). Plan **không sửa** file này. Thay đổi skill của plan giữ các khẳng định đó đúng (vẫn có `scripts/check-harness.mjs`, brief không đổi) | **Không sửa.** Chạy nó ở S0 để ghi baseline và chạy lại sau S5, S6 để so. Nếu cần sửa nó (ví dụ sau này đổi thông báo status của upgrade), bước đó **bị chặn** tới khi việc Workflow tab commit xong |
| `packages/core/squads/kitviet-*`, `kitviet-skills-data/common.ts`, `kitviet-skills/kv-squad-lead/SKILL.md` | Squad khác. `pnpm generate:sdlc-skills` chỉ đọc `sdlc-skills/`, không đụng chúng | Bỏ qua. Sau khi generate, `git diff --stat` không được thấy file `kitviet-*` đổi do plan |
| `packages/core/api/*`, `packages/core/issue-workflows/*`, `packages/core/types/issue-workflow.ts`, `packages/views/**`, `server/**` ở danh sách trên | Workflow tab | Bỏ qua. Nếu `pnpm --filter @multica/core typecheck` đỏ do các file này, ghi vào baseline S0 là lỗi nền, không sửa |

Các file plan **sẽ** sửa đều sạch ở thời điểm viết: `check-harness.mjs`, `sdlc-harness-check.test.ts`, `sdlc-skills.test.ts`,
`sdlc-skills-data.ts`, mọi `SKILL.md`/`reference.md`/`team-playbook.md` trong `sdlc-skills/`, `docs/engineering/sdlc-harness.md`.

Quy tắc commit: chỉ `git add` đường dẫn của bước (không `git add -A`), vì working tree có file của việc khác.

## 3. Tổng quan

| Bước | Tên | Phụ thuộc | Lượt chạy agent | Tính chất |
| --- | --- | --- | --- | --- |
| S0 | Baseline và hiệu chỉnh (chỉ đọc) | Quyết định mục 13.2 của spec | 1 đến 2 | Chuẩn bị |
| S1 | Ba mức `gate`/`cert`/`warning` và chế độ chạy | S0 | 2 đến 3 | Phát hành |
| S2 | Kiểm tra mới H8', H14, H15, H16, footprint | S1 | 2 | Phát hành |
| S3 | Kiểm tra thực thi `--run` (E1, E2) | S1 | 2 đến 3 | Phát hành |
| S4 | Chứng nhận JSON và làm mới | S3 | 2 | Phát hành |
| S5 | Skill, mẫu, profile, văn bản gating, test skill | S1 đến S4 | 3 đến 4 | Phát hành |
| S6 | Script đo `harness-metrics.mjs` | S0, S5 | 2 | Đo đạc |
| S7 | Tài liệu và đường nâng cấp | S5, S6 | 1 | Tài liệu |
| S8 | Thí điểm và hiệu chỉnh ngưỡng | S5 đến S7, người dùng đồng ý | 2 đến 3 (cộng 30 đến 120 lượt squad) | Đo thật |
| S9 | Eval pack ngoại tuyến (tùy chọn) | S8, người dùng đồng ý | 3 đến 4 (cộng khoảng 225 lượt squad) | Đo thật |

Tổng lượt chạy agent triển khai S0 đến S7: **15 đến 21** (làm tròn 14 đến 22). Lượt chạy của squad ở S8, S9 tính riêng và cần người dùng đồng ý.

### Thứ tự khuyến nghị

```
S0 ─► S1 ─► S2 ─┐
       └──► S3 ─► S4 ─► S5 ─► S7 ─► S8 ─► S9
S0 ─► (viết S6 song song) ───► tích hợp S6 sau S5 ─┘
```

- S2 và S3 cùng sửa `check-harness.mjs` nên làm tuần tự, không song song (xung đột hợp nhất). Thứ tự khuyến nghị: S1, S2, S3, S4.
- S6 viết được song song từ sau S0 (file mới, không đụng file của S1 đến S4). Chỉ phần sửa `sdlc-skills.test.ts` (thêm file vào danh sách) phải đợi S5.
- Dừng để người dùng xem sau S5 (đơn vị phát hành) và sau S8 (trước khi quyết định S9 và bộ chuyển cưỡng chế).

### Hoàn tác

Mỗi bước là một commit riêng: hoàn tác bằng `git revert <sha>`. S1 đến S5 hoàn tác như một nhóm (script và văn bản skill phải khớp). Squad đã
nâng cấp lên skill v2 nhận lại skill cũ khi chạy `upgrade-sdlc-squad.mjs --apply` từ commit trước S1 (vì `importBuiltinSkills` cập nhật khi nội dung khác).

## 4. Chi tiết từng bước

### S0. Baseline và hiệu chỉnh (chỉ đọc)

- **Mục đích:** có số liệu thật về tỉ lệ chặn sai của kiểm tra hiện tại và v2, và biết tên trường dữ liệu để đo.
- **File đổi:** tạo mới `docs/engineering/sdlc-harness-calibration.md`. Không sửa file nào khác.
- **Thay đổi cụ thể:**
  1. Chạy checker hiện tại trên multica (đã thấy 5 trong 8 kiểm tra chặn fail, research mục 8.1) và trên ít nhất **hai repo tốt khác** do người dùng chỉ định
     (không phải multica, không phải mua_ngon_ecommerce). Ghi bảng: repo, kiểm tra nào fail, người đọc đánh giá "đúng" hay "báo nhầm".
  2. Ghi baseline test: chạy hẹp `squads/sdlc-skills.test.ts`, `squads/sdlc-harness-check.test.ts`, `squads/upgrade.test.ts` và `typecheck`, ghi cái nào đỏ trước khi sửa gì.
  3. Đọc (không chạy lên DB) struct trả về của handler cho `issue runs`, `issue usage`, `issue timeline`, `issue comment list`, `issue children` trong
     `server/internal/handler/` và `server/cmd/multica/cmd_issue.go`, ghi tên trường JSON thật dùng cho M2 đến M7, và chỗ nào thiếu.
  4. Kiểm nội dung các file `.env.*` được commit mà H11 báo (ví dụ `apps/mobile/.env.production`): cấu hình công khai hay bí mật.
- **Kiểm tra bước xong:**
  ```
  for r in . <repo2> <repo3>; do node packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs --root "$r" > "/tmp/cal-$(basename "$(cd "$r" && pwd)").json"; echo "$r exit=$?"; done
  pnpm --filter @multica/core exec vitest run squads/sdlc-skills.test.ts squads/sdlc-harness-check.test.ts squads/upgrade.test.ts
  pnpm --filter @multica/core typecheck
  test -s docs/engineering/sdlc-harness-calibration.md && echo ok
  ```
  File hiệu chỉnh có bảng ít nhất 3 repo và danh sách trường JSON; baseline test ghi rõ lỗi nền.
- **Phụ thuộc:** cần người dùng chọn repo (spec mục 13.2).
- **Rủi ro riêng:** chạy checker trên repo của người khác chỉ đọc nhưng phải tránh `--run` (chưa có). Chọn quá ít repo thì số "tỉ lệ chặn sai" vô nghĩa; ghi `n` rõ ràng.
- **Lượt chạy agent:** 1 đến 2.

### S1. Ba mức và chế độ chạy

- **Mục đích:** `gate`/`cert`/`warning`, `--quick` chỉ gate, `--cert`, báo cáo mới; Developer không còn bị chặn bởi chất lượng thấp.
- **File đổi:**
  - `packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs`
  - `packages/core/squads/sdlc-harness-check.test.ts`
  - `packages/core/squads/sdlc-skills-data.ts` (sinh ra)
- **Thay đổi cụ thể:**
  1. Trong mảng `CHECKS`, đổi `level` thành `"gate" | "cert" | "warning"` theo spec 7.3: H1, H3, H4 là `gate`; H6, H7, H10 là `cert`; H2, H8, H5, H9, H11, H12, H13 là `warning`.
  2. H1 chỉ kiểm: `AGENTS.md` tồn tại; `CLAUDE.md` chứa `@AGENTS.md` và tối đa một câu. Chuyển phần kích thước sang H14.
  3. Thêm H14 (kích thước): mềm (hơn 120 dòng hoặc hơn 8 KiB) là `warning`; cứng (hơn 200 dòng hoặc hơn 24 KiB) là `cert`. Mức do `run` trả về động.
  4. Hàm `checkHarness(root, opts)` nhận `quick`, `cert`, `refresh`. Báo cáo thêm `certified`, `cert`, `mode`; `blocking` tính theo chế độ (spec 7.1). `ok` = không `gate` fail.
  5. `--quick` chỉ chạy `gate`, còn lại `skipped`. Thêm `--cert`. `--quick --cert` hợp lệ như `--quick`. Tham số lạ thoát 2.
  6. H13: ngân sách 32 KiB. H2 giữ nguyên nội dung nhưng đổi sang `warning` (cert JSON thêm ở S4).
  7. Cập nhật `sdlc-harness-check.test.ts`: `BLOCKING` thành ba danh sách (`GATE`, `CERT`, `WARN`); kiểm "gate đúng ba, cert đúng sáu (kể cả E1/E2 ở S3)"; fixture kiểu multica (130 dòng, không Done, không environment.md) thì `--quick` thoát 0, `--cert` thoát 1 và liệt kê H6, H10; thư mục trống `--quick` thoát 1.
- **Kiểm tra bước xong:**
  ```
  pnpm --filter @multica/core exec vitest run squads/sdlc-harness-check.test.ts
  node packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs --root . --quick; echo "exit=$?"   # kỳ vọng 0
  node packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs --root . --cert;  echo "exit=$?"   # kỳ vọng 1: H6, H10
  node packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs --root . --quick | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).checks.filter(c=>c.status!=="skipped").map(c=>c.id).join(",")))'   # H1,H3,H4
  pnpm generate:sdlc-skills && git diff --stat -- packages/core/squads/sdlc-skills-data.ts
  ```
- **Phụ thuộc:** S0 (dùng tỉ lệ báo nhầm để xác nhận danh sách `gate`).
- **Rủi ro riêng:** `sdlc-skills.test.ts` vẫn khẳng định bảng "blocks/warns" của văn bản skill (chưa đổi ở S1) nên vẫn xanh nhưng skill và script tạm thời lệch nhau
  trong lịch sử commit; vì thế S1 đến S5 phát hành cùng nhau. Đổi tên `level` có thể làm vỡ người dùng khác của JSON: tìm mọi chỗ đọc `.level`/`blocking`
  trước khi sửa (`grep -rn "check-harness" packages scripts --include=*.ts --include=*.mjs --include=*.md`).
- **Lượt chạy agent:** 2 đến 3.

### S2. Kiểm tra mới và footprint

- **Mục đích:** bắt tổng quan, con trỏ trần, Rule thiếu `Enforced by`; đo footprint để hiệu chỉnh.
- **File đổi:** `check-harness.mjs`, `sdlc-harness-check.test.ts`, `sdlc-skills-data.ts` (sinh ra). Cùng đường dẫn như S1.
- **Thay đổi cụ thể:**
  1. H8 định nghĩa lại (`warning`): trong mục Rules (hoặc Policy), mỗi mục danh sách kết thúc bằng `Enforced by: <đích>`; đích khác `prose` phải là đường dẫn hoặc lệnh có thật (tái dùng `judgeCommand` và `pathCandidates`).
  2. H15 (`warning`): con trỏ tới đường dẫn hoặc link phải đi cùng từ điều kiện (`when`, `before`, `if`, `changing`, `after`, `read when`); bỏ qua mục Verify.
  3. H16 (`warning`): bắt ký tự cây (`├──`, `└──`), tiêu đề khớp `overview|architecture|project structure|tech stack|directory (layout|structure)`, và đoạn văn xuôi từ ba dòng liên tiếp ngoài danh sách; miễn mục Greenfield.
  4. H12 chấp nhận tiêu đề `Known pitfalls` hoặc `Gotchas`.
  5. `footprint` trong báo cáo: `lines`, `bytes`, `instructions` (số mục danh sách ngoài khối mã), `proseRules`.
  6. H11: không báo `.env.*` được commit nếu mọi dòng khóa có tiền tố công khai (`EXPO_PUBLIC_`, `NEXT_PUBLIC_`, `VITE_`, `PUBLIC_`), nếu S0 cho thấy báo nhầm.
  7. Test: mỗi kiểm tra có fixture đạt và fixture trượt (ví dụ cây thư mục, tiêu đề Overview, con trỏ trần, Rule thiếu `Enforced by`, mục Greenfield được miễn).
- **Kiểm tra bước xong:**
  ```
  pnpm --filter @multica/core exec vitest run squads/sdlc-harness-check.test.ts
  node packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs --root . | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const r=JSON.parse(s);console.log(JSON.stringify(r.footprint),r.warnings.join(","))})'
  pnpm generate:sdlc-skills && git diff --stat -- packages/core/squads/sdlc-skills-data.ts
  ```
  Ghi kết quả H15, H16, H8 trên multica vào `sdlc-harness-calibration.md` (không đặt kỳ vọng pass; cần biết tỉ lệ báo nhầm).
- **Phụ thuộc:** S1.
- **Rủi ro riêng:** H15 và H16 là heuristic văn bản; giữ `warning`. Nếu tỉ lệ báo nhầm trên các repo ở S0 cao, giảm độ nhạy thay vì nâng mức.
- **Lượt chạy agent:** 2.

### S3. Kiểm tra thực thi (`--run`)

- **Mục đích:** chứng minh Ensure và Done chạy được, thay vì chỉ đọc văn bản.
- **File đổi:** `check-harness.mjs`, `sdlc-harness-check.test.ts`, `sdlc-skills-data.ts` (sinh ra).
- **Thay đổi cụ thể:**
  1. Tham số `--run` và `--timeout <giây>` (mặc định 900; đọc thêm `timeoutSeconds` từ `docs/sdlc/harness.json`). `--run` ngầm hiểu `--cert`. `--quick --run` thoát 2.
  2. E1: lấy lệnh từ dòng `Ensure command:` (đã có trong H10). Chạy `sh -c` ở gốc repo, `CI=1`, tạo nhóm tiến trình riêng để dừng khi quá hạn. Ghi `exit`, `seconds`, đuôi 20 dòng. In lệnh ra stderr trước khi chạy.
  3. E2: lấy lệnh từ dòng `Done = ...`. Điều kiện trước: `git status --porcelain` rỗng (bỏ qua file bị ignore), nếu không thì fail kèm danh sách file. Chạy như E1. Ghi `treeClean`.
  4. Che bí mật trong đuôi đầu ra bằng `SECRET_PATTERNS` có sẵn.
  5. Mục `executed` trong báo cáo; E1, E2 là `cert`. Kiểm tra tĩnh H6/H10 phải qua thì mới chạy E1/E2 (không chạy lệnh không xác định).
  6. Test bằng fixture chạy các script shell nhỏ: Ensure thoát 1, Done thoát 1, cây bẩn, quá hạn với `sleep` (kiểm tra tiến trình con bị dừng), đầu ra có mẫu khóa bị che.
- **Kiểm tra bước xong:**
  ```
  pnpm --filter @multica/core exec vitest run squads/sdlc-harness-check.test.ts
  node packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs --root . --quick --run; echo "exit=$?"   # kỳ vọng 2
  pnpm generate:sdlc-skills && git diff --stat -- packages/core/squads/sdlc-skills-data.ts
  ```
  Không chạy `--run` trên multica hay repo thật trong bước này (chỉ fixture trong thư mục tạm).
- **Phụ thuộc:** S1.
- **Rủi ro riêng:** chạy lệnh tùy ý: mọi thử nghiệm chỉ trên fixture tự tạo trong `os.tmpdir()`. Hành vi nhóm tiến trình khác giữa Linux, macOS và Windows: CI của repo là Linux/macOS; ghi chú Windows không hỗ trợ `--run` và test bỏ qua trên `win32`.
  Test có `sleep` làm test chậm: dùng timeout 1 giây.
- **Lượt chạy agent:** 2 đến 3.

### S4. Chứng nhận JSON và làm mới

- **Mục đích:** provenance rời khỏi `AGENTS.md`; làm mới đọc cert.
- **File đổi:** `check-harness.mjs`, `sdlc-harness-check.test.ts`, `sdlc-skills-data.ts` (sinh ra).
- **Thay đổi cụ thể:**
  1. `--write-cert`: ghi `docs/sdlc/harness-cert.json` (schema ở spec phụ lục B) chỉ khi `certified` và đã `--run`; thất bại thì thoát 1 và không ghi.
  2. H2 (`warning`): đọc cert JSON, kiểm `schema`, `sha` là commit thật; nếu chưa có cert thì chấp nhận dòng `Verified ... against <sha>` của v1.
  3. `--refresh`: lấy `certSha` từ cert (dự phòng v1), thêm `staleness` (ngày kể từ cert, `sha` có là tổ tiên của `HEAD` không), `manifestsChanged` kể cả `docs/sdlc/environment.md`, `needsRefresh` và lý do (trigger T1 đến T4 của spec).
  4. Test: ghi và đọc cert; sửa `package.json` rồi commit thì `needsRefresh`; xóa cert thì T4; repo v1 với dòng `Verified` vẫn chạy.
- **Kiểm tra bước xong:**
  ```
  pnpm --filter @multica/core exec vitest run squads/sdlc-harness-check.test.ts
  pnpm generate:sdlc-skills && git diff --stat -- packages/core/squads/sdlc-skills-data.ts
  ```
- **Phụ thuộc:** S3 (cần `executed` để ghi cert).
- **Rủi ro riêng:** hợp đồng JSON mới là thứ Lead đọc: đặt `schema: 1` và test từ chối `schema` lạ.
- **Lượt chạy agent:** 2.

### S5. Skill, mẫu, profile và văn bản gating

- **Mục đích:** mọi vai và mọi tài liệu của skill khớp spec; đây là bước làm hành vi agent thay đổi thật.
- **File đổi (đều thuộc `packages/core/squads/sdlc-skills/` trừ test):**
  - `sdlc-harness/SKILL.md` (Mode 1 theo spec 8.2; bảng kiểm tra `gate`/`cert`/`warning`; Mode 3 theo mục 9; smoke v2; luật nội dung R1 đến R13; còn dưới 500 dòng)
  - `sdlc-harness/references/agents-md-template.md` (mẫu v2, bỏ định hướng, bảng "Where things are" và dòng provenance; thêm ví dụ phân loại)
  - `sdlc-harness/references/profile-web.md`, `profile-backend.md`, `profile-baas.md`, `profile-mobile.md` (đổi thành danh sách thứ cần thăm dò)
  - `sdlc-squad-lead/SKILL.md`, `sdlc-squad-lead/reference.md`, `sdlc-squad-lead/team-playbook.md` (Harness gate chỉ theo `gate`; Route 0; chạy `--refresh`; đọc cert; báo cáo chỉ số M2 đến M7 sau S6)
  - `sdlc-development/SKILL.md`, `sdlc-delivery-core/SKILL.md` (`check-harness --quick` chỉ chặn `gate`; `cert` và `warning` không chặn)
  - `planning-architecture/SKILL.md` (bước 7 không viết `AGENTS.md`; thêm mục "Non-default conventions" và "Verification strategy" trong `architecture.md`)
  - `sdlc-architecture/SKILL.md` (Architect sở hữu `AGENTS.md`, cùng hai mục trên làm đầu vào)
  - `sdlc-devops-release/SKILL.md` (nhắc dòng `Ensure command:` phải idempotent và cô lập theo worktree; không đổi cấu trúc mode)
  - `packages/core/squads/sdlc-skills.test.ts` (cập nhật các khẳng định khoảng dòng 256 đến 291; xem dưới)
  - `packages/core/squads/sdlc-skills-data.ts` (sinh ra)
  - `packages/core/squads/sdlc-harness-check.test.ts` (thêm test mẫu v2: điền placeholder rồi chạy `--cert`)
- **Thay đổi cụ thể:**
  1. Viết lại văn bản theo spec, giữ giọng điệu ngắn và tiếng Anh cho phần dành agent. Giữ `## Headless`, `## Mode 1`, `## Mode 2`, `## Mode 3`, `## Harness smoke`.
  2. Sửa `sdlc-skills.test.ts`: bảng trong `SKILL.md` khớp `H* | ... | gate|cert|warns`; khẳng định `planning-architecture` đổi thành "does not write AGENTS.md" (hoặc cụm tương đương bạn chọn); Developer chỉ bị `gate` chặn; Lead không tạo `todo` khi `gate` fail;
     smoke có bước âm tính; danh sách file của skill (vẫn 6 file cho tới S6).
  3. Mẫu v2 phải điền được cho từng profile và vượt `--cert` trên fixture, tối đa 80 dòng (spec AC9): test đọc khối mã trong `agents-md-template.md`, thay placeholder, chạy checker.
  4. Không đổi `templates.ts`, brief, status. Kiểm `git diff --stat` chỉ gồm các file ở trên.
- **Kiểm tra bước xong:**
  ```
  pnpm generate:sdlc-skills
  pnpm --filter @multica/core exec vitest run squads/sdlc-skills.test.ts squads/sdlc-harness-check.test.ts squads/sdlc-agent-instructions.test.ts squads/planning-skills.test.ts squads/templates.test.ts
  pnpm --filter @multica/core typecheck
  wc -l packages/core/squads/sdlc-skills/sdlc-harness/SKILL.md          # dưới 500
  git diff --stat -- packages/core/squads/templates.ts server packages/core/squads/upgrade.ts packages/core/squads/upgrade.test.ts   # rỗng
  pnpm --filter @multica/core exec vitest run squads/upgrade.test.ts      # so với baseline S0, không thêm lỗi mới
  ```
- **Phụ thuộc:** S1 đến S4 (tên kiểm tra, cờ, mức).
- **Rủi ro riêng:** (a) `planning-architecture` còn được squad `bmad_planning` dùng nên chạy `planning-skills.test.ts`; (b) lệch chữ nghĩa giữa các skill gây mâu thuẫn (R12): đọc lại toàn bộ chỗ nhắc `check-harness`
  bằng `grep -rn "check-harness\|blocking check" packages/core/squads/sdlc-skills --include=*.md`; (c) giới hạn độ dài chỉ dẫn agent của Lead (2.300 ký tự) nằm ở `sdlc-agent-instructions.ts`, không đổi file đó;
  (d) văn bản skill ảnh hưởng hành vi nhiều agent: thêm một lượt đọc lại độc lập (Reviewer) trước khi merge.
- **Lượt chạy agent:** 3 đến 4.

### S6. Script đo `harness-metrics.mjs`

- **Mục đích:** Lead lấy M2 đến M7 từ dữ liệu nền tảng sẵn có, không đổi server.
- **File đổi:**
  - tạo `packages/core/squads/sdlc-skills/sdlc-harness/scripts/harness-metrics.mjs`
  - tạo `packages/core/squads/sdlc-harness-metrics.test.ts`
  - `packages/core/squads/sdlc-skills.test.ts` (thêm `scripts/harness-metrics.mjs` vào danh sách file của skill; **sau S5 để khỏi xung đột**)
  - `packages/core/squads/sdlc-skills/sdlc-harness/SKILL.md` (một dòng nói cách chạy; sau S5)
  - `packages/core/squads/sdlc-skills-data.ts` (sinh ra)
- **Thay đổi cụ thể:**
  1. Hàm thuần nhận JSON đã tải (runs, usage, timeline, comments) cho từng issue và trả M2 đến M7 (trung vị, p90, `n`). Thứ tự workflow lấy từ tham số `--order` (mặc định danh sách status của SDLC) để nhận ra "đi lùi".
  2. Lớp ngoài: `--issues <id,...>` hoặc `--parent <epic>`; gọi `multica issue runs|usage|timeline|comment list <id> --output json` (tên trường theo kết quả S0). `--from-dir <thư mục>` đọc JSON đã lưu thay vì gọi CLI.
  3. Đầu ra: JSON và một bảng markdown để dán vào báo cáo. Không ghi file.
  4. Test với dữ liệu mẫu nhỏ nhúng trong test: tái tạo đúng M2 đến M7 đã biết, thiếu trường thì báo rõ chỗ nào xấp xỉ, issue chưa `done` bị loại khỏi mẫu số.
- **Kiểm tra bước xong:**
  ```
  pnpm --filter @multica/core exec vitest run squads/sdlc-harness-metrics.test.ts squads/sdlc-skills.test.ts
  pnpm generate:sdlc-skills && git diff --stat -- packages/core/squads/sdlc-skills-data.ts
  node packages/core/squads/sdlc-skills/sdlc-harness/scripts/harness-metrics.mjs --help; echo "exit=$?"
  ```
  **Không** chạy script lên DB hoặc server dùng chung trong CI hay trong bước này. Lần chạy đầu trên dữ liệu thật thuộc S8, trên môi trường cô lập.
- **Phụ thuộc:** S0 (tên trường), S5 (cho phần sửa `sdlc-skills.test.ts`, `SKILL.md`).
- **Rủi ro riêng:** tên trường CLI đổi giữa các phiên bản; ghi phiên bản `multica` trong đầu ra. Một số trường (lùi workflow) có thể không đủ trong timeline: dùng xấp xỉ và ghi nhãn "xấp xỉ".
- **Lượt chạy agent:** 2.

### S7. Tài liệu và đường nâng cấp

- **Mục đích:** tài liệu thiết kế khớp mã; squad cũ nâng cấp được.
- **File đổi:** `docs/engineering/sdlc-harness.md` (cập nhật mục 2 đến 6, 8, 9 theo v2, thêm liên kết tới spec và research; hoặc đánh dấu thay thế nếu người dùng chọn viết bản mới).
  Có thể thêm một dòng liên kết ở `docs/engineering/sdlc-autonomy.md` mục 4 nếu cần (không bắt buộc).
- **Thay đổi cụ thể:**
  1. Ghi trạng thái "đã triển khai" đúng những gì S1 đến S6 làm; nhắc lại điều chưa làm (hook cưỡng chế, S8, S9).
  2. Kiểm tra bằng đọc mã `scripts/upgrade-sdlc-squad.mjs` và `packages/core/squads/upgrade.ts` rằng nâng cấp squad cũ chỉ đồng bộ skill (không thêm status). Không sửa hai file đó.
  3. Ghi vào tài liệu cách nâng cấp: `node scripts/upgrade-sdlc-squad.mjs` (dry run) rồi `--apply`.
- **Kiểm tra bước xong:**
  ```
  git diff --check -- docs/engineering
  for f in docs/engineering/sdlc-harness*.md; do grep -o '](\([^)#]*\.md\)' "$f" | sed 's/](//' | while read -r l; do test -f "docs/engineering/$l" || echo "BROKEN: $f -> $l"; done; done
  ```
  (không in `BROKEN`). Ghi rõ trong commit: chưa chạy test mã (chỉ tài liệu), theo `AGENTS.md`.
- **Phụ thuộc:** S5, S6.
- **Rủi ro riêng:** tài liệu nói quá những gì chưa làm. Ghi trạng thái từng mục theo bằng chứng (commit và test).
- **Lượt chạy agent:** 1.

### S8. Thí điểm và hiệu chỉnh ngưỡng (cần người dùng đồng ý)

- **Mục đích:** biết v2 hoạt động ra sao trên dự án thật và đặt lại ngưỡng bằng số.
- **File đổi:** cập nhật `docs/engineering/sdlc-harness-calibration.md`; có thể một commit nhỏ chỉnh hằng số H14 và ngưỡng smoke trong `check-harness.mjs`/`SKILL.md` theo DR5 (kéo theo `pnpm generate:sdlc-skills`).
- **Thay đổi cụ thể:**
  1. Chọn một dự án greenfield nhỏ không phải mua_ngon_ecommerce (hoặc hai bản sao cùng đặc tả để đối chiếu v1 và v2). Dựng trên **môi trường cô lập** theo worktree của repo (`make up`, dữ liệu riêng), không dùng DB dùng chung.
  2. Chạy qua squad SDLC: Environment, Harness, Harness smoke, hai epic nhỏ. Thu M1 đến M9 bằng `harness-metrics.mjs` và `check-harness --refresh`.
  3. Ghi vào tài liệu hiệu chỉnh: kết quả smoke, footprint, số block do harness, chi phí token, các lệnh tự khám phá; áp DR4 và DR5.
  4. Nếu kiểm tra `gate` hay `cert` báo nhầm, sửa kiểm tra (không nới ngưỡng); mỗi sửa là một commit có test.
- **Kiểm tra bước xong:**
  ```
  node packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs --root <repo thí điểm> --cert --run   # trong repo của dự án thí điểm
  node packages/core/squads/sdlc-skills/sdlc-harness/scripts/harness-metrics.mjs --parent <epic> --output json
  ```
  Tiêu chí AC14 của spec: smoke đạt trong ≤2 run, M7 = 0, `AGENTS.md` ≤ 80 dòng; bảng số liệu có trong tài liệu hiệu chỉnh.
- **Phụ thuộc:** S5 đến S7; người dùng đồng ý chi phí và môi trường (spec mục 13.3).
- **Rủi ro riêng:** chi phí 30 đến 120 lượt chạy; kết quả nhiễu (research C8) nên chỉ đọc như mô tả; môi trường cô lập phải được dựng trước, nếu không agent dùng nhầm DB dùng chung.
  Một dự án không đủ để quyết định DR1 (cần ≥5 dự án).
- **Lượt chạy agent:** 2 đến 3 cho phần triển khai; **30 đến 120 lượt chạy của squad** cho phần thí điểm.

### S9. Eval pack ngoại tuyến (tùy chọn)

- **Mục đích:** trả lời "AGENTS.md v2 có đáng giữ không" bằng đối chứng nhỏ (spec 10.4, DR2, DR3).
- **File đổi:** tạo thư mục `scripts/harness-eval/` (script chạy và chấm) và `docs/engineering/sdlc-harness-eval.md` (kết quả); hoặc đặt dưới `packages/core/squads/` nếu cần test trong bộ vitest. Quyết định vị trí khi bắt đầu, vì phụ thuộc cách chạy agent ngoại tuyến.
- **Thay đổi cụ thể:**
  1. Chọn 20 đến 30 tác vụ lấy từ lỗi thật: issue bị trả lại, `rework-loop`, `environment`, `tooling` của các dự án SDLC (người dùng chỉ định nguồn; mặc định không dùng mua_ngon_ecommerce). Mỗi tác vụ lưu: sha, nội dung issue, lệnh chấm tất định.
  2. Dựng ba biến thể `AGENTS.md` cho mỗi repo: V0' (chỉ Verify), V1 (mẫu v1), V2 (mẫu v2). V0 tùy chọn.
  3. Chạy ít nhất 3 lượt mỗi ô; thu pass@1, pass^3, token, số bước; bootstrap theo cụm tác vụ; đọc transcript mẫu thất bại.
  4. Áp DR2 và DR3 và ghi quyết định.
- **Kiểm tra bước xong:** tài liệu kết quả có bảng tác vụ × biến thể × lượt, khoảng tin cậy và quyết định theo DR; script chấm có test với tác vụ giả.
- **Phụ thuộc:** S8; người dùng đồng ý chi phí (spec 13.4).
- **Rủi ro riêng:** chi phí khoảng 225 lượt; cỡ mẫu nhỏ (research C3) nên có thể không phân biệt; nhiễu giữa lượt (C8). Rò rỉ nội dung dự án thật vào kết quả: che thông tin nhạy cảm.
- **Lượt chạy agent:** 3 đến 4 triển khai; **khoảng 225 lượt chạy của squad** (25 tác vụ × 3 biến thể × 3 lượt), thêm 75 nếu chạy V0.

## 5. Điều dừng và quyết định cần người

| Sau bước | Cần người quyết |
| --- | --- |
| Trước S0 | Repo hiệu chỉnh (spec 13.2); nhóm đối chứng (spec 13.8) |
| Trước S1 | Chấp nhận mô hình ba mức (spec 13.1) |
| Sau S5 | Xem lại đơn vị phát hành; đồng ý nâng cấp squad bằng `upgrade-sdlc-squad.mjs --apply`; đồng ý bỏ viết `AGENTS.md` khỏi bước 7 `planning-architecture` (spec 13.6) |
| Trước S8 | Môi trường cô lập và chi phí (spec 13.3) |
| Sau S8 | S9 có chạy không (spec 13.4); runtime của daemon để quyết định bộ chuyển hook (spec 13.5) |

## 6. Rủi ro tổng thể của kế hoạch

| # | Rủi ro | Giảm thiểu |
| --- | --- | --- |
| 1 | Tree có thay đổi chưa commit của việc khác; commit nhầm file | `git add` theo đường dẫn; chạy `git status --short` trước và sau mỗi bước; so với mục 2 |
| 2 | Quên sinh lại `sdlc-skills-data.ts` | Mỗi bước chạm `sdlc-skills/` có lệnh `pnpm generate:sdlc-skills` và `git diff --stat` trong phần kiểm tra |
| 3 | Test của `upgrade.test.ts` đỏ vì lý do không liên quan | Baseline ở S0; chỉ so sánh chênh lệch |
| 4 | Văn bản skill mâu thuẫn nhau sau S5 | Đọc lại toàn bộ chỗ nhắc `check-harness`; Reviewer đọc độc lập |
| 5 | Kế hoạch dùng số ngưỡng tạm thời như thật | Gọi rõ "tạm thời" trong spec và tài liệu; S8 hiệu chỉnh |
| 6 | Chi phí S8, S9 vượt dự tính | Mỗi bước cần đồng ý riêng; có thể dừng sau S7 mà vẫn giữ được giá trị chính (kiểm tra thực thi, gate nhẹ hơn) |
| 7 | Phạm vi lan sang Kit Việt hoặc template khác | Không bước nào chạm `kitviet-*`, `templates.ts`, `initiative_build.go`; AC12 kiểm bằng `git diff --stat` |

## 7. Ước lượng tổng hợp

| Nhóm | Bước | Lượt chạy agent triển khai | Lượt chạy của squad |
| --- | --- | --- | --- |
| Phát hành tối thiểu (kiểm tra thực thi, gate nhẹ, skill mới) | S0 đến S5 | 12 đến 17 | 0 |
| Cộng đo đạc và tài liệu | S6, S7 | 3 | 0 |
| Cộng thí điểm | S8 | 2 đến 3 | 30 đến 120 |
| Cộng eval pack | S9 | 3 đến 4 | khoảng 225 |

Mốc dừng hợp lý nhất để người dùng đánh giá: sau S5 (harness v2 chạy được, chưa đo), rồi sau S8 (có số đo đầu tiên).
