# SDLC squad: Harness bootstrap (v2, hợp đồng trước)

Spec đã triển khai. Bổ sung cho [sdlc-autonomy.md](sdlc-autonomy.md) và [sdlc-specialist-routing.md](sdlc-specialist-routing.md).
Căn cứ chọn thiết kế: [sdlc-harness-research.md](sdlc-harness-research.md). Đặc tả đầy đủ: [sdlc-harness-spec.md](sdlc-harness-spec.md).
Kế hoạch và trạng thái từng bước: [sdlc-harness-plan.md](sdlc-harness-plan.md). Số liệu hiệu chỉnh: [sdlc-harness-calibration.md](sdlc-harness-calibration.md).

Ngôn ngữ: viết bằng tiếng Việt để người dùng đọc; tên lệnh, status, file và đoạn văn bản dành cho agent giữ nguyên
tiếng Anh, vì skill của agent viết bằng tiếng Anh.

Tài liệu này thay bản v1 (13 kiểm tra văn bản H1 đến H13, 8 kiểm tra chặn, `AGENTS.md` có phần định hướng và bảng ranh giới). Lý do thay: bản v1
chặn nhầm cả harness tốt (chạy trên multica: 5 trong 8 kiểm tra chặn fail), có tổng quan repo mà nghiên cứu cho là vô ích, và không có cách đo.

## 1. Vấn đề

"Harness" là những gì một agent đọc và chạy trước khi chạm vào repo: `AGENTS.md`, `CLAUDE.md` import nó, lệnh Ensure (dựng và kiểm tra môi trường)
và lệnh Done (phép kiểm chứng của dự án). Squad SDLC chỉ yêu cầu agent **đọc**; không bước nào **tạo**, **chứng minh** hay **làm mới** chúng. Ở
mua_ngon, `AGENTS.md` được viết một lần, rồi 113 commit sau đó không sửa, và không có cách nào để agent phát hiện nó sai.

Bằng chứng nghiên cứu (research): file ngữ cảnh nhìn chung không làm tăng tỉ lệ thành công của agent và làm tăng chi phí khoảng 20%; phần tổng quan
repo không giúp; thứ có tác dụng là lệnh kiểm chứng chạy được và sự kiện vận hành agent không tự suy ra được. Vì vậy v2 giữ file nhỏ và đẩy phần
cốt lõi sang thực thi.

## 2. Quyết định

| # | Quyết định |
| --- | --- |
| 1 | Status `Harness` sau `Environment`, trước Epic 1. Handler: Architect. Không đổi (templates.ts, bản Go và brief giữ nguyên). |
| 2 | Skill `sdlc-harness`, không vai mới. Chế độ headless. Dùng lại `references/` của `bmad-project-context` (không sửa bản vendored). |
| 3 | `AGENTS.md` chỉ có Verify, Rules (có `Enforced by`), Known pitfalls (có bằng chứng), Read when (con trỏ có điều kiện). Không tổng quan, không trạng thái, không chép lệnh. Mục tiêu 80 dòng. |
| 4 | Ba mức kiểm tra: `gate` (H1, H3, H4) chặn Developer; `cert` (H6, H7, H10, H14, E1, E2) là điều kiện chứng nhận stage; `warning` còn lại. |
| 5 | Kiểm tra thực thi `--run`: Ensure (E1) và Done (E2) phải thoát 0 trên cây git sạch. Chỉ Architect chạy, chỉ trên repo của squad. |
| 6 | Provenance nằm ở `docs/sdlc/harness-cert.json` (schema 1), không nằm trong `AGENTS.md`. |
| 7 | Gating chỉ bằng luật trong skill, không đổi server. Developer chạy `check-harness --quick` (chỉ `gate`); lỗi `cert`/`warning` không chặn. |
| 8 | Harness hỏng là `tooling` kèm id kiểm tra; không thêm lớp blocker. |
| 9 | Làm mới (Mode 3) khi `--refresh` báo `needsRefresh`, sau retrospective, sau ruling lặp lần hai, hoặc khi Developer chặn vì `gate`. Luật tăng trưởng, cắt tỉa, ngân sách, nâng lên mã (spec mục 9). |
| 10 | Đo hiệu quả bằng `scripts/harness-metrics.mjs` từ dữ liệu có sẵn (runs, usage, timeline, comment). Chỉ mô tả, không chứng minh nhân quả. |
| 11 | Squad `bmad_planning` không viết `AGENTS.md`; để lại hai mục "Non-default conventions" và "Verification strategy" trong architecture. |

## 3. Stage Harness

Chuỗi issue của Lead cho một repo có phụ thuộc runtime:

```
Environment bring-up (DevOps) -> Harness (Architect) -> Harness smoke (Developer) -> Epic 1 ...
```

- Lead chạy `check-harness` tĩnh (không `--run`) trước khi lập kế hoạch. `gate` fail hoặc chưa có chứng nhận trong dự án mới: tạo issue Harness
  (`--start-status harness`) và Harness smoke, epic đầu chờ smoke (`--after`). Repo đang chạy được mà chỉ `cert`/`warning` fail: tạo issue Harness (Mode 3)
  xếp hàng sau công việc, không chặn ai.
- Architect làm theo Mode 1 của `sdlc-harness`: chạy Ensure và từng lệnh định ghi, làm cho Done có thật, ghi lại mọi chỗ vấp thành ứng viên Known pitfalls,
  viết `AGENTS.md` theo mẫu v2, đặt `CLAUDE.md` là `@AGENTS.md`, commit đường dẫn của mình, chạy `check-harness --cert --run --write-cert` cho tới khi
  chứng nhận, commit `harness-cert.json`.
- DevOps viết dòng ``- Ensure command: `<lệnh>` `` trong `docs/sdlc/environment.md`; H10 đọc và E1 chạy dòng này. Lệnh phải idempotent. Thiếu lệnh ensure:
  Architect báo `tooling` thay vì tự viết hạ tầng.
- `sdlc_maintenance` dùng **Route 0**: chạy `check-harness`, chỉ tạo issue Harness chạy trước khi `gate` fail. Không viết lại `AGENTS.md` của người.
- Dự án đi qua `bmad_planning`: bước 7 của `planning-architecture` giờ **không** viết `AGENTS.md`.

## 4. check-harness

```
node <skill>/scripts/check-harness.mjs [--root <dir>] [--quick] [--cert] [--run [--timeout <s>]] [--write-cert] [--accept <ID>=<A-n>] [--refresh]
```

Mã thoát: 0 không có gì chặn ở chế độ này; 1 có; 2 sai tham số. Chế độ mặc định: `gate` chặn. `--quick`: chỉ `gate`, dưới 1 giây. `--cert`: `gate` và `cert` đều chặn.
`--run`: thêm E1, E2; ngầm hiểu `--cert`; không dùng với `--quick`. `--write-cert`: cần `--run`, chỉ ghi khi chứng nhận. `--accept H6=A-3`: hạ bậc một kiểm tra
`cert` có ghi chép (gate không hạ bậc được). Báo cáo có `ok`, `certified`, `blocking`, `cert`, `accepted`, `warnings`, `footprint`, `executed`.

| ID | Kiểm tra | Mức |
| --- | --- | --- |
| H1 | `AGENTS.md` tồn tại; `CLAUDE.md` chứa `@AGENTS.md` và tối đa một câu | gate |
| H3 | Mọi đường dẫn trong code span/link tồn tại (bỏ URL, glob, tên gói, thư mục sinh ra, đường dẫn git ignore) | gate |
| H4 | Mọi script `npm`/`pnpm`/`yarn` và target `make` tồn tại. Lệnh của công cụ khác không được phân giải | gate |
| H6 | Đúng một dòng `Done = ...` nêu lệnh và mức L1 đến L3 | cert |
| H7 | Không từ ngữ trạng thái trong luật ("is implemented" bị bắt, "must be implemented in" thì không) | cert |
| H10 | `docs/sdlc/environment.md` có dòng `Ensure command:`; `AGENTS.md` nêu file và lệnh đó | cert |
| H14 | `AGENTS.md` tối đa 200 dòng và 24 KiB (cảnh báo ở hơn 120 dòng hoặc 8 KiB) | cert |
| E1, E2 | `--run`: lệnh Ensure và lệnh Done thoát 0 (E2 trên cây git sạch) | cert |
| H2 | `harness-cert.json` hợp lệ và `sha` là commit thật (chấp nhận dòng `Verified` của v1 nếu chưa có) | warning |
| H5 | Có file pin phiên bản và CI dùng cùng major | warning |
| H8 | Mỗi Rule kết thúc bằng `Enforced by:`; đích là đường dẫn/lệnh thì phải có thật | warning |
| H9 | Có CONTRIBUTING, PR template hoặc quy ước commit | warning |
| H11 | `.env` và `.multica/` bị ignore, có `.env.example`, không mẫu bí mật (file `.env.*` chỉ có khóa tiền tố công khai không bị báo) | warning |
| H12 | Mỗi dòng Known pitfalls (hoặc Gotchas) có tham chiếu bằng chứng | warning |
| H13 | Các file "must read" tối đa 32 KiB, hoặc có chỉ mục `Read when:` | warning |
| H15 | Mỗi con trỏ tới tài liệu có điều kiện | warning |
| H16 | Không dấu hiệu tổng quan: cây thư mục, tiêu đề Overview/Project structure, đoạn văn xuôi | warning |

Đường dẫn sinh ra sau này khai trong `docs/sdlc/harness.json` (`{ "ignorePaths": ["generated/"], "timeoutSeconds": 900, "profile": ["web"] }`).

Giới hạn đã biết: kiểm tra tĩnh là heuristic trên văn bản. H3 chỉ xét token trong backtick hoặc link; H4 không phân giải lệnh có `--filter`, `-C` hay biến và không phân
giải công cụ ngoài bốn công cụ trên (E1, E2 bù); H15, H16 chỉ có hai repo để hiệu chỉnh, chưa biết tỉ lệ báo nhầm; E2 trên repo trống có thể qua vì chưa có gì để kiểm.

## 5. Harness smoke

Issue nhỏ do Lead tạo, giao Developer, chỉ theo `AGENTS.md`: chạy Ensure hai lần, một thay đổi có test trong một vùng, Quick rồi Done, **bước âm tính** (làm hỏng cố ý,
Done phải thoát khác 0, rồi hoàn tác), commit theo quy ước. Ngưỡng (giả thuyết, cần hiệu chỉnh): tối đa 2 run, 0 `blocked`, 0 lần review/QA trả lại, 0 can thiệp của người.
Lệnh Developer phải tự khám phá thành một dòng trong `AGENTS.md`. Trượt thì Architect sửa và chạy lại một lần; trượt lần hai ghi `docs/sdlc/parked.md`. Smoke là kiểm tra chức năng đạt/trượt,
không phải phép đo thống kê.

## 6. Các file đã đổi

| File | Thay đổi |
| --- | --- |
| `packages/core/squads/sdlc-skills/sdlc-harness/SKILL.md` | Viết lại: bốn loại nội dung, luật nội dung, ba mức, Mode 1 (execution-first), 2, 3, smoke v2 |
| `.../sdlc-harness/scripts/check-harness.mjs` | Ba mức, H14 đến H16, `--cert`, `--run`, `--write-cert`, `--accept`, `--refresh` đọc chứng nhận |
| `.../sdlc-harness/scripts/harness-metrics.mjs` | Mới: M2 đến M7 từ runs, usage, timeline, comment |
| `.../sdlc-harness/references/` | `agents-md-template.md` v2; `profile-*.md` thành danh sách thứ cần thăm dò |
| `.../sdlc-squad-lead/SKILL.md`, `reference.md`, `team-playbook.md` | Harness gate theo `gate`, Route 0, làm mới theo `needsRefresh`, đo cuối epic |
| `.../sdlc-development`, `sdlc-delivery-core` | `check-harness --quick`: chỉ `gate` chặn |
| `.../planning-architecture`, `sdlc-architecture`, `sdlc-devops-release` | Bước 7 không viết `AGENTS.md`; hai mục đầu vào cho Architect; Ensure idempotent |
| `packages/core/squads/sdlc-skills-data.ts` | Sinh bằng `pnpm generate:sdlc-skills` |
| Test | `sdlc-harness-check.test.ts`, `sdlc-harness-metrics.test.ts` (mới), `sdlc-skills.test.ts` |

Không đổi: `templates.ts`, `server/internal/service/initiative_build.go`, `upgrade.ts`, `upgrade.test.ts`, brief của stage, status workflow, `server/`.

## 7. Nâng cấp squad đã tạo

`node scripts/upgrade-sdlc-squad.mjs` (dry run) rồi `--apply`. Không thêm step mới nên workflow không đổi. Skill `sdlc-harness`, các skill role và hai script được đồng bộ vì
`importBuiltinSkills` gọi `updateSkill` khi nội dung khác bản built-in. Repo đã có `AGENTS.md` v1 vẫn chạy: H2 chấp nhận dòng `Verified`, và chỉ `gate` chặn.

## 8. Còn lại, chưa làm

- **Thí điểm trên dự án thật (S8)** và **eval pack ngoại tuyến (S9)**: cần người dùng đồng ý chi phí (plan, spec mục 13). Ngưỡng smoke, kích thước và các quy tắc quyết định DR1 đến DR5 chưa được hiệu chỉnh bằng số.
- `harness-metrics.mjs` chưa chạy trên dữ liệu thật; hình dạng JSON của `issue comment list` và `issue usage` chưa kiểm với server thật.
- E1, E2 chưa chạy trên một repo thật của squad.
- Bộ chuyển cưỡng chế (job CI, git hook, hook `Stop` của Claude Code): chưa làm, quyết định sau S8.
- Chặn ở tầng engine: chưa đề xuất.
- Áp dụng cho mua_ngon (làm mới `AGENTS.md`, `.nvmrc`, ignore `.multica/`) là việc riêng, qua chính stage Harness.

## 9. Kiểm chứng

`sdlc-harness-check.test.ts` dựng repo tạm có git và kiểm tra từng tiêu chí (đạt và trượt), ba mức và mã thoát, `--run` (Ensure/Done thoát 1, cây bẩn, hết giờ và dừng cả tiến trình con,
che bí mật), `--write-cert`, `--accept`, `--refresh`, và rằng mẫu v2 điền xong thì chứng nhận được trong 80 dòng. `sdlc-harness-metrics.test.ts` tái tạo M2 đến M7 từ dữ liệu mẫu.
`sdlc-skills.test.ts` khẳng định bảng mức trong skill, luật gating của Lead và Developer, bước 7 của planning, bước âm tính của smoke, và danh sách `BLOCKER_CLASSES` không đổi.
