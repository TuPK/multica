# SDLC squad: hiệu chỉnh Harness (S0)

Kết quả bước S0 của [sdlc-harness-plan.md](sdlc-harness-plan.md). Chạy ngày 2026-11-12, chỉ đọc, bằng `check-harness.mjs` **v1** (trước khi sửa gì).
Cập nhật thêm sau S2 và S7 ở cuối tài liệu.

## 1. Baseline test (trước khi sửa)

Lệnh: `pnpm --filter @multica/core exec vitest run squads/sdlc-skills.test.ts squads/sdlc-harness-check.test.ts squads/upgrade.test.ts`
Kết quả: 3 file, **75 test pass**, 0 lỗi. `upgrade.test.ts` đang có thay đổi chưa commit của việc khác nhưng pass.

## 2. Checker v1 trên các repo cục bộ

Không dùng mua_ngon_ecommerce. `multica-kitviet` là bản trùng của multica (kết quả giống hệt) nên chỉ tính một lần.

| Repo | AGENTS.md | Kiểm tra chặn fail | Đánh giá từng lỗi |
| --- | --- | --- | --- |
| multica | 130 dòng, 13.814 byte | H1, H2, H6, H8, H10 | H1: đúng luật nhưng luật 12 KB là tùy ý. H2, H6, H8, H10: thiếu thứ **mẫu của ta** đòi, harness thực tế tốt. **Chặn nhầm cả 5** |
| vnpt-software-factory | 92 dòng | H1, H2, H3, H6, H7, H8, H10 | H1 đúng (CLAUDE.md không import AGENTS.md). H3 đúng (`scripts/dev.sh`, `docs/vnpt-flow` không tồn tại). **H7 nhầm**: "MUST be implemented in" dùng động từ. H2, H6, H8, H10: thiếu thứ mẫu đòi |
| 9router | không có | tất cả (không có AGENTS.md) | Đúng: không có harness |
| shots-rail | không có | tất cả | Đúng: không có harness |

Tổng: trong 2 repo có harness thật, **12 lượt fail chặn**, trong đó chỉ 2 là chỉ dẫn sai thật (H1 và H3 ở vnpt) và 1 là báo nhầm
(H7 ở vnpt). Còn lại là chênh lệch so với mẫu. Kết luận: nhóm `gate` {H1, H3, H4} bắt đúng lỗi thật (n nhỏ, 2 repo), nhóm còn lại không nên chặn Developer.

H11 (cảnh báo) trên multica: báo `.env` được commit vì `apps/mobile/.env.production`, `.env.staging`, `apps/desktop/.env.staging` bị commit.
Chỉ xem tên khóa: `EXPO_PUBLIC_API_URL`, `EXPO_PUBLIC_WEB_URL`, `VITE_API_URL`, `VITE_APP_URL`, đều tiền tố công khai. **Báo nhầm.** Sửa ở S2.

## 3. Tên trường dữ liệu cho chỉ số (đọc mã, không chạy lên DB)

Nguồn: `server/internal/handler/agent.go` (`AgentTaskResponse`), `daemon.go` (`GetIssueUsage`), `activity.go` (`TimelineEntry`).

| Lệnh | Trường dùng |
| --- | --- |
| `multica issue runs <id> --output json` | mỗi run: `id`, `status`, `started_at`, `completed_at`, `attempt`, `failure_reason`, `error` |
| `multica issue usage <id> --output json` | `total_input_tokens`, `total_output_tokens`, `total_cache_read_tokens`, `total_cache_write_tokens` |
| `multica issue timeline <id> --activity-only --output json` | mỗi mục: `type`, `action`, `created_at`, `details` (với `status_changed`: `from`, `to`) |
| `multica issue comment list <id> --output json` | `content`, `created_at` (đọc dòng `Blocker-class:`) |

Giới hạn đã biết: `details.from/to` của `status_changed` được viết tự do (comment trong `cmd_issue_timeline.go` nói "free-form"); timeline có thể bị cắt
(`X-Timeline-Truncated`). M2 (đi lùi workflow) vì vậy là xấp xỉ khi thiếu `from`/`to`. Bao phủ token có thể thiếu (`>=` trong `issue usage`).

## 4. Sau S1 và S2: checker v2 trên cùng các repo

Chạy `check-harness.mjs --root <repo>` (chế độ mặc định) sau khi sửa. Không còn kiểm tra nào chặn nhầm Developer trên hai repo có harness thật.

| Repo | `gate` fail | `cert` fail | `warning` fail | Ghi chú |
| --- | --- | --- | --- | --- |
| multica | không | H6, H10 | H2, H5, H11, H14 | `--quick` thoát 0 (v1: thoát 1). H14 là cảnh báo mềm (130 dòng, 13.814 byte). H15, H16, H8 pass (H8 pass vì không có mục Rules/Policy). H11 còn lại: `pixtuoid/justfile` khớp mẫu bí mật, chưa xem; không còn báo `.env` public |
| vnpt-software-factory | H1, H3 | H6, H10 | H2, H5, H11, H14 | Hai `gate` fail là chỉ dẫn sai thật. H7 hết báo nhầm ("MUST be implemented in") |

Footprint multica: 130 dòng, 60 mục danh sách. vnpt: 92 dòng, 27 mục.
n = 2 repo có harness; tỉ lệ báo nhầm của H15, H16 chưa thể kết luận (cả hai đều pass), cần thêm mẫu ở S8.

## 5. S8: thí điểm trên repo greenfield (môi trường cô lập)

Chạy ngày 2026-11-12, theo mục S8 của [sdlc-harness-plan.md](sdlc-harness-plan.md). Kết quả **thực hiện một phần**: pipeline SDLC chạy hết các giai đoạn, nhưng smoke không đạt AC14 (xem 5.4).

### 5.1 Môi trường

| Hạng mục | Giá trị |
| --- | --- |
| Mã nguồn | Snapshot `HEAD` `aa3b2a239` xuất bằng `git archive` vào `~/.cache/claude-delegate/multica-s8`. Không dùng `git worktree add`, vì quy tắc của phiên cấm lệnh làm đổi trạng thái git |
| Cấu hình | `.env.worktree` sinh bởi `scripts/init-worktree-env.sh`. Snapshot không có `.env` |
| Cơ sở dữ liệu | `multica_multica_s8_720` trên Postgres dùng chung `localhost:5432`. Không dùng DB `multica` |
| API | `http://localhost:18800` |
| Web | `http://localhost:13720`. Khi kết thúc phiên, web không phản hồi trong 40 giây (tiến trình còn chạy). Chưa khởi động lại, vì pipeline chỉ dùng API và daemon |
| Daemon | Đang chạy, profile `dev-multica_s8-720`, sáu runtime |
| Đăng ký | `make up` với `--name s8-pilot --ttl 72` (owner `agent`). Dữ liệu giữ nguyên sau khi kết thúc |
| Repo mẫu | `~/.cache/claude-delegate/s8-greenfield-repo`: Node 24, không phụ thuộc, `npm test` chạy `node --test`. Bản bare và remote: `~/.cache/claude-delegate/s8-remotes/s8-greenfield-todo.git`, phục vụ bởi `~/.cache/claude-delegate/s8-driver/git-smart-http.py` tại `http://127.0.0.1:8765` |

Các cổng `8080` và `30000` đang nghe lúc đầu phiên. Khi kiểm tra lại giữa phiên, không còn tiến trình nào trên hai cổng. Registry ghi `multica-51` là `stopped`. Phiên này không gửi lệnh nào tới các tiến trình đó. Nguyên nhân chưa xác định.

Thiết lập squad: `SDLC Greenfield` (template `sdlc_greenfield`, 9 agent, runtime `claude`, workflow `SDLC Auto`). Project Build `8e9dd76f-…` gắn repo và workflow này. Lần đầu thiếu project Build, Lead bị chặn ở bước đầu (DEV-1). Đây là lỗi thiết lập của phiên, không phải phát hiện về harness.

### 5.2 Lượt chạy squad

Tổng **48 lượt** (ngân sách 30 đến 120 của plan). Đếm bằng CLI trên profile cô lập.

| Nhóm | Issue | Trạng thái cuối | Lượt |
| --- | --- | --- | --- |
| Yêu cầu | DEV-1 | `todo` (Lead không chuyển sang `Plan ready`) | 2 |
| Environment | DEV-2 | `stage_done` | 1 |
| Harness | DEV-3 | `stage_done` | 1 |
| Harness smoke | DEV-4 | `done` | 7 |
| Mở khóa harness | DEV-14 | `stage_done` | 1 |
| Epic 1 `removeItem` | DEV-5 dev, DEV-6 review, DEV-7 QA | `done` | 4, 2, 7 |
| Mở khóa Epic 1 | DEV-15 | `done` | 3 |
| Epic 2 `searchOpen` | DEV-8 dev, DEV-9 review, DEV-10 QA | `done` | 4, 2, 7 |
| Mở khóa Epic 2 | DEV-16 | `done` | 4 |
| Cuối | DEV-11 QA, DEV-12 security, DEV-13 DevOps | `done`, `stage_done`, `stage_done` | 1, 1, 1 |

Kết quả trên `main` (commit `0ca88a2`): `removeItem` và `searchOpen` có test. Clone mới của `main` chạy `node --test`: 10 test, 10 pass, 0 fail. Developer commit trên nhánh cục bộ của daemon. Ba issue mở khóa đưa commit lên `main`.

### 5.3 Số liệu M1 đến M9

Đo bằng `harness-metrics.mjs` với thứ tự `backlog,todo,in_progress,in_review,qa,done`. Đầu vào: 15 issue DEV-2 đến DEV-16 (`--parent` trả về 0 vì Lead tạo issue trong project Build, không gắn parent). n = 15 issue, 10 đã `done`.

| Chỉ số | Giá trị | Ghi chú |
| --- | --- | --- |
| M1 chi phí dựng harness | 9 lượt; output 65.582 token; cache read 6,88 triệu | Issue DEV-3, DEV-4, DEV-14 (n = 3 issue) |
| M2 first-pass | 80% (8/10) | Hai issue không qua ngay là QA `DEV-7` và `DEV-10`, đều đi `qa → in_progress`. Riêng hai issue Developer (`DEV-5`, `DEV-8`): 100%, n = 2 |
| M3 lượt mỗi issue giao xong | trung vị 4, p90 7 (n = 10); lượt lỗi 0 | |
| M4 token mỗi issue giao xong | trung vị 22.929, p90 40.980 (n = 10) | Token do `issue usage` báo, có thể thiếu một phần |
| M5 block mỗi issue | 0,20 (3 sự kiện trên 15 issue) | Tooling 1 (DEV-4), environment 1 (DEV-7), environment và delivery 1 (DEV-10). Xem 5.6 về cách đếm |
| M6 thời gian chu kỳ | trung vị 0 giờ, p90 0,3 giờ (n = 10) | Làm tròn theo giờ, không đủ độ phân giải để đọc |
| M7 block do harness | **1** | DEV-4 báo `tooling`, nêu H1, H3, H4. Nguyên nhân là remote không nhận push, xem 5.5 |
| M8 footprint | 18 dòng, 908 byte, 9 chỉ dẫn, 4 luật văn xuôi | Trên `main`. Ngày từ khi chứng nhận: 0. Manifest đổi sau chứng nhận: không |
| M9 smoke | Đạt sau 7 lượt. Lượt 1 bị block; sáu lượt sau chạy liên tiếp, mỗi lượt 20 đến 30 giây | Không có bước âm tính |

Ghi chú về M4: `cost_usd_ticks` bằng 0 ở mọi issue, nên chi phí tiền không đo được. Chỉ có số token.

### 5.4 Smoke và AC14

| Tiêu chí AC14 | Kết quả |
| --- | --- |
| Smoke đạt trong ≤ 2 lượt | **Không đạt**: 7 lượt. Lượt 1 bị block vì `main` thiếu `AGENTS.md`. Sáu lượt sau chạy liên tiếp và chưa rõ tại sao |
| M7 = 0 | **Không đạt**: M7 = 1 |
| `footprint` ≤ 80 dòng | Đạt: 18 dòng |
| Bước âm tính của smoke | **Không có**: comment kết luận của smoke không nhắc bước làm hỏng cố ý nào. Skill yêu cầu bước này (AC10), nên đây là lỗi thực thi của agent hoặc của skill cần kiểm tra |

### 5.5 check-harness: có báo nhầm không

- Repo trống lúc đầu (baseline): H1, H3, H4 fail. Đúng, vì không có `AGENTS.md`.
- Lúc smoke bị block: H1, H3, H4 fail trên `main`. Đúng về trạng thái của `main`. Harness đã đạt trên nhánh `agent/sdlc-architect/…`, nhưng chưa được push, vì remote HTTP tĩnh lúc đầu không nhận push. Lỗi nằm ở hạ tầng pilot, không ở checker. Sau đó remote đã nhận push (smart HTTP).
- Cuối pilot, trên clone sạch của `main` (`0ca88a2`): `ok: true`, `certified: true`, không có kiểm tra chặn, không có kiểm tra `cert` fail. `--cert --run`: E1 (`npm test`) và E2 (`npm test`) đều thoát 0 trên cây sạch. `--refresh`: `needsRefresh: false`.
- Warning còn lại: H5 (không có ghim phiên bản), H9 (không có CONTRIBUTING), H11 (không có `.env.example`). Cả ba đều đúng với repo này.

Kết luận: trong pilot này không quan sát được báo nhầm của `gate` hay `cert`. Chỉ có một repo, nên đây không phải bằng chứng về tỉ lệ báo nhầm.

### 5.6 Lỗi trong công cụ đo

Đã phát hiện khi đối chiếu số liệu với comment và timeline. Chưa sửa trong multica, vì sửa cần commit mà phiên này cấm. Bản sửa thử để đo chỉ nằm ở `~/.cache/claude-delegate/s8-driver/harness-metrics-patched.mjs`.

1. **M5 đếm sai theo hai cách.** Regex `blockerClasses` không khớp dòng có markdown (`**`, backtick), và mỗi comment chỉ lấy được một lớp. Kết quả: 0,13 (bản gốc), 0,33 (sau khi sửa regex, nhưng vẫn đếm theo comment; một block có nhiều comment BLOCKED, routed và unblock nên bị đếm lặp), 0,20 (đếm sự kiện `status_changed` sang `blocked` trong timeline). Đề xuất: đếm theo sự kiện sang `blocked`, lấy lớp từ comment `Blocker-class:` gần nhất trước đó.
2. **M2 thiếu trạng thái `qa`** trong thứ tự mặc định, nên chuyển ngược `qa → in_progress` không được tính. Đề xuất thêm `qa` vào mặc định hoặc đọc thứ tự từ workflow của project.
3. **`--parent` không thấy issue của project Build.** Skill yêu cầu Lead tạo issue không có parent, nên `--parent` trả về 0. Đề xuất thêm `--project <id>`.
4. **Script gọi `multica` theo PATH**, nên mặc định dùng profile của người dùng. Phiên này phải đặt wrapper ở đầu PATH để giữ mọi lệnh trong profile cô lập. Đề xuất biến `MULTICA_BIN`.

### 5.7 Phát hiện về quy trình (không thuộc harness)

- **Commit không lên `origin`.** Ba lần block (DEV-4, DEV-7, DEV-10) và ba issue mở khóa (DEV-14, DEV-15, DEV-16) đều bắt nguồn từ việc commit nằm trên nhánh cục bộ. Developer và Architect không push lên `origin`. Cả ba block trong pilot đều truy về lỗi này, nên đây là nguồn block lớn nhất của luồng. Cần quyết định chính sách push của squad SDLC. Quyết định này nằm ngoài harness.

**Đã chốt (người dùng):** squad SDLC không được push lên `origin`, chỉ commit. Skill `sdlc-delivery-core` đã thêm quy tắc này. Hệ quả chưa được giải quyết: commit trên nhánh cục bộ của daemon không tự vào `main`, nên QA và bước giao hàng có thể vẫn bị block. Cần thiết kế cách commit đến nhánh đích mà không push (ví dụ merge cục bộ vào một nhánh tích hợp). Chưa làm.
- **Lead không đóng request.** Sau khi các epic xong, DEV-1 vẫn `todo`. Skill yêu cầu chuyển sang `Plan ready`. Chưa rõ nguyên nhân.
- **Lượt đánh thức lặp.** Sau khi smoke được mở khóa, DEV-4 chạy sáu lượt liên tiếp trong vài phút. Chưa xác định trigger.

### 5.8 Điều chỉnh ngưỡng (DR4, DR5)

- **DR4 (lệnh tự khám phá):** không có lệnh nào ngoài `npm test`. Không thêm vào mẫu profile.
- **DR5 (đặt lại ngưỡng H14):** không đặt lại. DR5 cần phân vị 90 của nhiều `AGENTS.md` đã qua smoke. Pilot chỉ có một repo, 18 dòng, dưới cả ngưỡng mềm 120 và ngưỡng cứng 200. Giữ ngưỡng tạm thời.
- Không đổi ngưỡng để làm kết quả đẹp hơn. Không có kiểm tra `gate` hay `cert` nào được sửa trong S8.

### 5.9 Chưa kiểm chứng

- Một repo greenfield, một lần chạy. Không có đối chứng với v1 hay với dự án khác.
- Không có chi phí tiền. Token có thể thiếu một phần.
- Smoke không có bước âm tính, nên chưa biết kiểm tra có bắt được lỗi cố ý hay không.
- AC14 chỉ đạt 1 trên 3 tiêu chí.
