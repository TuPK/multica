# SDLC squad: Harness bootstrap

Spec. Bổ sung cho [sdlc-autonomy.md](sdlc-autonomy.md) và [sdlc-specialist-routing.md](sdlc-specialist-routing.md).
Quyết định lấy từ bản phân tích so sánh harness của multica và mua_ngon (report.md mục 5). Tài liệu này mô tả thứ đã
được triển khai, không phải đề xuất.

Ngôn ngữ: viết bằng tiếng Việt để người dùng đọc; tên lệnh, status, file và đoạn văn bản dành cho agent giữ nguyên
tiếng Anh, vì skill của agent viết bằng tiếng Anh.

## 1. Vấn đề

"Harness" là những gì một agent đọc trước khi chạm vào repo: `AGENTS.md`, `CLAUDE.md` import nó, các lệnh nó nêu, và
`docs/sdlc/environment.md`. Squad SDLC chỉ yêu cầu agent **đọc** các file này; không bước nào **tạo**, **kiểm tra** hay
**làm mới** chúng. Ở mua_ngon, `AGENTS.md` được viết một lần (2026-11-03) trước khi squad chạy, rồi 113 commit sau đó không
sửa. Nó mâu thuẫn với chính nó (mục trạng thái), không nhắc lệnh dựng môi trường mà squad dùng thật, và không có cách nào
để một agent phát hiện điều đó. `bmad-project-context` có trong bundle nhưng không stage nào bắt chạy.

## 2. Quyết định

| # | Quyết định |
| --- | --- |
| 1 | Status mới `Harness` trong `SDLC_AUTO_WORKFLOW`, ngay sau `Environment`, trước Epic 1. Handler: Architect. Kết thúc ở Stage done. |
| 2 | Skill mới `sdlc-harness`, không vai mới. Nó dùng lại `references/` của `bmad-project-context` (không sửa bản vendored) và thêm chế độ headless. |
| 3 | `check-harness.mjs`: Node thuần, in JSON, có mã thoát. 13 kiểm tra H1 đến H13, 8 kiểm tra chặn. |
| 4 | Gating chỉ bằng luật trong skill: Lead không tạo issue `todo` cho Developer/Bug Fixer khi còn kiểm tra chặn fail; Developer chạy `check-harness --quick` ở bước Read. Không đổi server engine. |
| 5 | Không thêm lớp blocker. Harness hỏng là `tooling`, kèm id kiểm tra; Lead chuyển cho Architect với start status `harness`. |
| 6 | Làm mới (Mode 3) khi: sau retrospective được chấp nhận; manifest đổi so với SHA provenance; ruling `environment`/`tooling`/`rework-loop` lặp lần thứ hai cùng nguyên nhân. |

## 3. Stage Harness

Chuỗi issue của Lead cho một repo có phụ thuộc runtime:

```
Environment bring-up (DevOps) -> Harness (Architect) -> Harness smoke (Developer) -> Epic 1 ...
```

- Lead chạy `check-harness` đầy đủ trước khi lập kế hoạch. Pass: không tạo issue Harness, ghi `Harness: pass` ở dòng
  `Readiness`. Fail một tiêu chí chặn: tạo issue Harness (`--start-status harness`) và Harness smoke, epic đầu chờ smoke (`--after`).
- Architect làm theo Mode 1 của `sdlc-harness`: khám phá lệnh từ manifest, chạy từng lệnh định ghi, viết `AGENTS.md` từ
  mẫu, đặt `CLAUDE.md` là `@AGENTS.md`, chạy lại `check-harness` cho tới khi hết lỗi chặn, ghi `docs/sdlc/harness-cert.md`.
- DevOps (Mode 1 Environment) viết dòng `Ensure command:` (lệnh nằm trong backtick) trong `docs/sdlc/environment.md`; H10 đọc dòng này.
  Nếu lệnh ensure chưa có, Architect báo `tooling` thay vì tự viết hạ tầng.
- `sdlc_maintenance` không có workflow riêng: Lead dùng **Route 0** trong `reference.md` (chạy `check-harness`, chỉ tạo
  issue Harness khi fail tiêu chí chặn).
- Dự án đi qua squad `bmad_planning`: bước 7 của `planning-architecture` giờ yêu cầu `AGENTS.md` pass các kiểm tra chặn.

## 4. check-harness

```
node <skill>/scripts/check-harness.mjs [--root <dir>] [--quick] [--refresh]
```

Mã thoát: 0 không có kiểm tra chặn nào fail; 1 có; 2 sai tham số. `--quick` chỉ chạy tám kiểm tra chặn (dưới 1 giây trên
multica). `--refresh` thêm mục `refresh`: SHA provenance, các manifest đổi từ SHA đó (`package.json`, `Makefile`, CI,
`supabase/config.toml`, `.env.example`, compose, `AGENTS.md`), và `needsRefresh`.

| ID | Kiểm tra | Mức |
| --- | --- | --- |
| H1 | `AGENTS.md` tồn tại, tối đa 200 dòng và 12 KB; `CLAUDE.md` chứa `@AGENTS.md` và tối đa một câu | chặn |
| H2 | Dòng `Verified <date> against <sha>`; SHA là commit có thật | chặn |
| H3 | Mọi đường dẫn trong code span/link tồn tại (bỏ qua URL, glob, tên gói, thư mục sinh ra, đường dẫn bị git ignore) | chặn |
| H4 | Mọi script `npm`/`pnpm`/`yarn` và target `make` trong code span/khối code tồn tại | chặn |
| H5 | Có file pin phiên bản và CI dùng cùng major với `.nvmrc` | cảnh báo |
| H6 | Đúng một dòng `Done = ...` (mẫu trong `agents-md-template.md`) nêu lệnh có thật và evidence level L1 đến L3 | chặn |
| H7 | Không có từ ngữ trạng thái trong luật (implemented, scaffold, TODO, WIP, dấu tick); mục Greenfield được miễn | chặn |
| H8 | Có bảng ranh giới với cột `Enforced by` và ít nhất một hàng | chặn |
| H9 | Có CONTRIBUTING, PR template hoặc quy ước commit | cảnh báo |
| H10 | `docs/sdlc/environment.md` có dòng `Ensure command:`; `AGENTS.md` nêu file đó và lệnh đó | chặn |
| H11 | `.env` và `.multica/` bị ignore, có `.env.example`, không có mẫu bí mật trong file tracked | cảnh báo |
| H12 | Mỗi dòng Known pitfalls có tham chiếu bằng chứng | cảnh báo |
| H13 | Các file "must read" tổng cộng tối đa 60 KB, hoặc có chỉ mục `Read when:` | cảnh báo |

Chỉ tám tiêu chí chặn để squad không tự đặt cổng ngoài tiêu chí (bài học của `sdlc-autonomy.md`). Nếu một tiêu chí chặn
không thể đạt vì lý do hợp lệ, Architect ghi một assumption nêu tên kiểm tra và lý do, rồi tiếp tục: hạ bậc có ghi chép,
không dừng. Đường dẫn sinh ra sau này có thể khai trong `docs/sdlc/harness.json` (`{ "ignorePaths": ["generated/"] }`).

Giới hạn đã biết: kiểm tra là heuristic trên văn bản. H3 chỉ xét token trong backtick hoặc link; H4 không phân giải lệnh có
`--filter`, `-C` hay biến; H7 có thể báo nhầm một từ như "scaffold" dùng như động từ (sửa câu chữ, đừng nới kiểm tra);
H11 quét mẫu khóa phổ biến, bỏ qua file test, fixture và `*.example`.

## 5. Harness smoke

Issue nhỏ do Lead tạo ngay sau Harness, giao Developer: một thay đổi có test trong đúng một vùng ranh giới, chỉ theo
`AGENTS.md`, không hỏi ai. Lead đọc kết quả từ comment, run và lịch sử status.

| Số đo | Ngưỡng (giả thuyết, cần hiệu chỉnh) |
| --- | --- |
| Số run cho tới Stage done | 2 hoặc ít hơn |
| Số lần chuyển `blocked` | 0 |
| Số lần review/QA trả lại | 0 |
| Cần người can thiệp | 0 |
| `Done` exit 0 trên tree sạch | có |

Lệnh Developer phải tự khám phá mà `AGENTS.md` lẽ ra nêu thì thành một dòng trong file đó hoặc một kiểm tra. Trượt thì
Architect sửa chỗ thiếu và chạy lại một lần; trượt lần hai thì ghi vào `docs/sdlc/parked.md` và epic tiếp tục.

## 6. Các file đã đổi

| File | Thay đổi |
| --- | --- |
| `packages/core/squads/sdlc-skills/sdlc-harness/SKILL.md` | Skill mới: Mode 1 (stage), 2 (audit), 3 (refresh), smoke, headless |
| `packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs` | Bộ kiểm tra H1 đến H13 |
| `packages/core/squads/sdlc-skills/sdlc-harness/references/` | `agents-md-template.md`, `profile-web/backend/baas/mobile.md` |
| `.../sdlc-squad-lead/SKILL.md`, `reference.md`, `team-playbook.md` | Hàng Readiness, Harness gate, start status `harness`, refresh, Route 0, định tuyến `tooling` có id kiểm tra |
| `.../sdlc-delivery-core`, `sdlc-development` | `check-harness --quick` trước khi đổi code; lỗi chặn là `tooling` |
| `.../sdlc-devops-release`, `sdlc-architecture`, `planning-architecture` | Dòng `Ensure command:`; Architect sở hữu nội dung `AGENTS.md`; bước 7 planning |
| `packages/core/squads/templates.ts` | Step `Harness` sau `Environment` |
| `packages/core/squads/upgrade.ts`, `scripts/upgrade-sdlc-squad.mjs` | Thông báo quyền admin liệt kê đúng các status mới; ghi chú đầu script |
| `packages/core/squads/sdlc-skills-data.ts` | Sinh bằng `pnpm generate:sdlc-skills` |
| Test | `sdlc-harness-check.test.ts` (mới), `sdlc-skills.test.ts`, `templates.test.ts`, `quickstart.test.ts`, `upgrade.test.ts` |

Không đổi: `sdlc-agent-instructions.ts` (Lead còn khoảng 70 ký tự), thư mục skill Lead vẫn chỉ có `reference.md` và
`team-playbook.md`, `sdlc-previous-texts.ts` (không brief hay instruction có sẵn nào bị sửa), `server/internal/issueworkflow/`.

## 7. Nâng cấp squad đã tạo

`node scripts/upgrade-sdlc-squad.mjs` (dry run) rồi `--apply`. `mergeWorkflowSteps` thêm step còn thiếu trước step cuối
của template theo thứ tự template, nên workflow đã có `Environment` chỉ nhận thêm `Harness`; skill `sdlc-harness` cùng
script của nó được nạp qua `attachBuiltinSkills`. Tạo status mới cần quyền admin workspace: nếu thiếu, kết quả nêu tên
status (ví dụ `harness`) để admin cho phép.

## 8. Còn lại, chưa làm trong lần này

- **Workflow initiative build trên server**: đã làm. `server/internal/service/initiative_build.go` có status `harness`
  ("Harness", role `architect`, skill `sdlc-harness`) trong `sdlcStageStatuses`, cùng brief và map skill, đặt sau
  `Environment`; brief giống `templates.ts`. Mô tả và màu status do Go tự đặt vì TypeScript không có hai trường này.
  Kiểm chứng bằng `initiative_build_steps_test.go` (handler, next/back, vắng agent thì không có step, status phủ đủ step),
  không cần DB. Chưa chạy: tạo squad thật qua đường initiative build trên DB, và test cần DB.
- Chặn ở tầng engine (`todo` -> `in_progress` khi chưa có chứng nhận harness): chưa đề xuất, vì tăng độ phức tạp.
- Hiệu chỉnh ngưỡng smoke bằng số liệu của 3 đến 5 dự án.
- Áp dụng cho mua_ngon (làm mới `AGENTS.md`, `.nvmrc`, ignore `.multica/`, `verify:runtime`) là việc riêng, qua chính stage Harness.

## 9. Kiểm chứng

`packages/core/squads/sdlc-harness-check.test.ts` dựng repo tạm có git và kiểm tra từng tiêu chí (pass và fail), `--quick`,
`--refresh`, mã thoát. `sdlc-skills.test.ts` khẳng định bảng chặn/cảnh báo trong skill khớp danh sách tám tiêu chí, luật
gating của Lead và Developer, và danh sách `BLOCKER_CLASSES` không đổi. `upgrade.test.ts` khẳng định thêm đúng step và nạp skill.
