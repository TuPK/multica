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

**Đã đổi (người dùng):** squad được phép push lên `origin` để không bị block. Lần đầu, quy tắc "chỉ commit" đã được thêm vào skill `sdlc-delivery-core` và sau đó gỡ bỏ. Commit của Developer và Architect phải được push để QA và bước giao hàng thấy được. Chưa kiểm chứng sau khi đổi, vì chưa chạy lại pilot.
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

### 5.10 Pilot lần hai: có mở quyền push thì QA và bước giao hàng có qua được không

Ngày 2026-11-12, sau commit `4581de249` (cho phép squad push). Không có lượt squad mới nào trong bước này. Kết luận dựa trên dữ liệu lượt đầu (5.2) và lần nâng cấp skill.

**Đính chính mục 5.7.** Mục 5.7 ghi "Developer và Architect không push lên `origin`". Câu này sai. Remote pilot nhận push từ khoảng 09:40. Sau đó Architect push `agent/sdlc-architect/dfcd6a4cee88` (09:40), Developer push `agent/sdlc-developer/6734d7cfcbd6` (09:48, `removeItem`) và `agent/sdlc-developer/98a54326babd` (09:53, `searchOpen`). Mục 5.7 giữ nguyên như bản ghi lúc viết. Phần đúng là dưới đây.

**Môi trường lúc kiểm tra**
- API `localhost:18800` healthy (pid 2979739).
- Daemon profile `dev-multica_s8-720` đã dừng lúc 11:10:55 với `context canceled`. Log không có lỗi panic và không có lý do khác. Không rõ ai hoặc cái gì gửi tín hiệu dừng. Tôi khởi động lại bằng `make up C=daemon ARGS="--name s8-pilot"` (pid 3549561).
- Web `localhost:13720` vẫn không phản hồi (HTTP 000). Chưa khởi động lại vì pipeline không dùng web.
- Remote smart HTTP `127.0.0.1:8765` (pid 3092556) vẫn chạy.
- Server người dùng `8080` và `30000` trả HTTP 200. Không gửi lệnh nào tới chúng.

**Nâng cấp skill của squad pilot.** Snapshot được đưa lên cây của `4581de249` (`git archive`). Chạy `scripts/upgrade-sdlc-squad.mjs --template sdlc_greenfield`, dry-run trước, rồi `--apply`. Biến `MULTICA_SERVER_URL`, `MULTICA_WORKSPACE_ID`, `MULTICA_TOKEN` trỏ vào workspace cô lập. Kết quả: squad `current`, skill `ready`, workflow `current` không thêm hay sửa bước nào. Đọc skill `sdlc-delivery-core` qua API: không còn luật "Never push", và `updated_at` không đổi. Nội dung đã giống bản mới.

**Phát hiện: squad pilot không bao giờ bị cấm push.** Luật "Never push to any remote" chỉ có trong commit `6f91c2fd5` (11:26). Commit `4581de249` (11:28) gỡ luật đó. Skill `sdlc-delivery-core` ở `aa3b2a239` (nguồn của squad pilot lúc tạo) giống hệt bản `4581de249`. Chỉ có reviewer bị cấm push, và cấm này nằm trong chỉ dẫn của reviewer ("You do not edit, commit, or push code"). Vì vậy việc "mở quyền push" không thay đổi hành vi của squad pilot. Quyền này đã có sẵn, và Developer đã dùng nó.

**Block trong lượt đầu và nguyên nhân thật**

| Issue | Vị trí | Lớp | Nguyên nhân thật | Gỡ bằng |
| --- | --- | --- | --- | --- |
| DEV-4 | Harness smoke, lượt 1 | tooling | `AGENTS.md` chỉ nằm trên nhánh chưa push (lúc đó remote chưa nhận push) | DEV-14: Architect tạo lại harness từ `main` và push |
| DEV-7 | QA Epic 1 | environment | `origin/main` ở `2c0e7a5`. Commit `e1e0bb4` đã push nhưng nằm trên nhánh `agent/sdlc-developer/…`, không nằm trên `main` | DEV-15: DevOps fast-forward `main` tới `e1e0bb4` |
| DEV-10 | QA Epic 2 | environment, delivery | `origin/main` thiếu `searchOpen`. Commit `0ca88a2` đã push nhưng không vào `main`. Comment ghi việc đưa lên `main` là "a lead decision" | DEV-16: delivery land `0ca88a2` lên `main` bằng ff-only |

Cả ba block đều xảy ra sau khi commit đã được push. Push không sửa được chúng, vì QA và bước giao hàng đọc `main`. Không có bước nào trong skill nói ai đưa nhánh của Developer lên `main`. Lead phải quyết định, và các issue DEV-15, DEV-16 đã làm việc đó bằng tay.

**Số liệu đo lại** (`harness-metrics.mjs` đã commit, không dùng bản vá tạm; dữ liệu 15 issue DEV-2 đến DEV-16, n = 10 đã done)

| Chỉ số | Giá trị |
| --- | --- |
| M2 first-pass | 80% (8/10). Hai issue QA lùi `qa → in_progress`. Hai issue Developer: 100%, n = 2 |
| M3 lượt mỗi issue giao xong | trung vị 4, p90 7 |
| M4 token mỗi issue giao xong | trung vị 22.929, p90 40.980 |
| M5 block | 3 sự kiện chuyển sang `blocked` (0,20 mỗi issue). Đếm theo comment `Blocker-class` cho 0,27 vì một block có nhiều comment |
| M6 cycle time | trung vị 0 giờ, p90 0,3 giờ |
| M7 block do harness | 1 (DEV-4) |
| M8 footprint | 18 dòng, không cần refresh |

**So sánh với lần đầu.** Không có lượt mới, nên không đo được tác động của việc mở push. Số liệu lần đầu không đổi (48 lượt, 3 sự kiện block, M2 80%). Điều đã thấy được: push có xảy ra ngay trong lượt đầu, và QA cùng bước giao hàng vẫn bị block sau đó.

**Kiểm tra `main` lần cuối** (clone mới của bare repo pilot, `main` ở `0ca88a2`)
- `node --test`: exit 0, 10 tests, 10 pass, 0 fail.
- `check-harness --cert --run` (không ghi cert): exit 0, `ok: true`, `certified: true`, `blocking: []`, không có kiểm tra cert fail. E1 và E2 thoát 0 trên cây sạch. Warning: H5, H9, H11.
- `--refresh`: `needsRefresh: false`, cert sha `0e64df3`, không có manifest đổi.

**Trạng thái lượt và issue**
- Tổng 48 lượt, không đổi kể từ lần đầu (ngân sách 30 đến 120).
- Không có issue nào `blocked`. Có 10 issue `done`, 5 issue `stage_done`, DEV-1 `todo` (Lead chưa chuyển sang `Plan ready`).

**Việc cần quyết định (chưa làm)**
- Muốn biết luật đưa lên `main` có gỡ được block không, cần một lượt mới. Lượt đó cần tạo issue mới. Quy tắc phiên không cho tạo issue khi chưa có việc thiếu hẳn, nên chưa tạo. Ước tính: 15 đến 25 lượt, tổng sau đó khoảng 65 đến 75, còn trong trần 120.
- Nếu muốn thử luật, cần chọn một trong hai hướng: (a) skill yêu cầu delivery fast-forward `main` sau khi QA PASS, hoặc (b) skill yêu cầu Developer tự đưa commit lên `main` khi được giao. Tôi chưa sửa skill nào.

**Chưa kiểm chứng**
- Không biết Developer có tự push khi skill không nhắc tới push hay không. Chỉ biết là có push trong lượt đầu, khi không có luật cấm.
- Không biết luật đưa lên `main` có gỡ block hay không.
- Nguyên nhân daemon dừng lúc 11:10 chưa rõ.
- Web chưa khôi phục.

### 5.11 Pilot vòng 3: luật đưa commit lên `main`

Ngày 2026-11-12, sau mục 5.10. Chỉ dùng workspace pilot. Repo multica không bị sửa.

**Dịch vụ sau khôi phục**
- API `localhost:18800`: healthy (pid 2979739).
- Daemon `dev-multica_s8-720`: đã dừng hai lần trước đó (11:10:55 và 11:39:17, cả hai `context canceled`). Lần này khởi động bằng `setsid nohup ... daemon start --foreground` (sid 3582160). Sau khi các lệnh của phiên kết thúc, daemon vẫn chạy (kiểm tra lần cuối sau 17 phút).
- Web `localhost:13720`: không phản hồi do tiến trình `next-server` đã chết. Khởi động bằng `make up C=web ARGS="--name s8-pilot"`, HTTP 200.
- Remote pilot `127.0.0.1:8765` (pid 3092556): chạy.
- Server của người dùng: `8080` và `30000` trả HTTP 200 khi kiểm tra cuối. Một lần kiểm tra trước đó, `30000` trả HTTP 000 rồi lên lại. Không liên quan đến thao tác của phiên này.

**Trạng thái trước khi thử.** Remote `main` ở `0ca88a2`, có `removeItem` và `searchOpen`. DEV-7 và DEV-10 đều `done`, không còn block.

**Luật đã chọn.** QA kiểm tra đúng commit Developer đã push lên nhánh của nó, không đợi `main`. Sau GO, QA fast-forward `main` tới commit đó (`git push origin <sha>:main`), không force. Lý do chọn: luật "land sau QA PASS" đứng riêng sẽ deadlock. Ở DEV-7 và DEV-10, QA không cho GO vì `main` chưa có thay đổi. Nếu QA kiểm tra trên `main` và `main` chỉ được cập nhật sau GO, không bao giờ có GO.

**Áp dụng.** Sửa hai file skill trong snapshot pilot (repo multica không đổi):
- `sdlc-delivery-core/SKILL.md`: Developer push nhánh của mình (`git push origin HEAD:<nhánh>`) và nêu commit id, nhánh trong handoff. Không push `main`.
- `sdlc-qa-verification/SKILL.md`: thêm bước 8 của Mode 2 (kiểm tra đúng commit trên nhánh) và mục `## Landing (after GO)`.

Sinh lại bundle bằng `node scripts/generate-sdlc-skills.mjs` (77 skill). Áp vào pilot bằng `scripts/upgrade-sdlc-squad.mjs --template sdlc_greenfield --apply`, với biến môi trường trỏ tới workspace pilot. Xác nhận qua API đọc theo id: `sdlc-qa-verification` có mục Landing, `sdlc-delivery-core` có luật push lên nhánh, và không còn "Never push to any remote".

**Request.** DEV-17, giao cho squad: thêm `countOpen(list)` và test. Chuỗi issue: DEV-18 (Developer, bước 1), DEV-19 (Reviewer, bước 2), DEV-20 (QA, bước 3).

| Issue | Vai trò | Trạng thái | Lượt | Điều đã xảy ra |
| --- | --- | --- | --- | --- |
| DEV-18 | Developer, và cũng là chỗ QA của chuỗi | `done` | 4 | Commit `aa37f8d` lên nhánh `agent/sdlc-developer/afa87ccf4358` (11:50:03). Reviewer PASS (11:50:47). QA GO và land lên `main` (11:51:45) |
| DEV-19 | Reviewer trên commit đã push | `done` | 4 | GO trên `aa37f8d`, 14 test pass |
| DEV-20 | QA trên commit đã push | `done` | 1 | GO trên `aa37f8d`. Push `aa37f8d:main` trả "Everything up-to-date", vì `main` đã có commit đó |
| DEV-17 | Request | `todo` | 1 | Lead chưa chuyển sang `Plan ready` (giống DEV-1) |

**Kết quả đưa lên `main`.** Có đúng một lần fast-forward: `0ca88a2` → `aa37f8d`. `git merge-base --is-ancestor 0ca88a2 aa37f8d` xác nhận đây là fast-forward. Không có push bị từ chối và không có force.

**Block trong vòng 3: không có.** Trước đó, cùng loại thay đổi (`removeItem`, lần 1) mất 16 lượt, gồm cả 3 lượt của DEV-15 mở khóa QA. Vòng 3 mất 10 lượt cho `countOpen`. Đây là hai mẫu, mỗi bên n = 1, và nội dung khác nhau. Không nên coi đây là chứng cứ về hiệu quả.

**Phát hiện phụ**
- Chuỗi có hai lần QA: DEV-18 (QA ở bước `qa` của chính issue Developer) và DEV-20 (issue QA riêng). Cả hai kiểm tra cùng một commit. Lần thứ hai không làm gì thêm vì `main` đã có commit. Đây là việc làm lặp, cần Lead xử lý.
- Lead không đóng request (DEV-1, DEV-17 đều `todo`). Đây là lỗi lặp lại, không do luật push.

**Kiểm tra `main` lần cuối** (clone mới của remote, HEAD `aa37f8d`)
- `node --test`: exit 0, 14 test, 14 pass, 0 fail.
- `check-harness --cert --run` (không `--write-cert`): exit 0, `ok: true`, `certified: true`, `blocking: []`, không có kiểm tra `cert` fail. E1 và E2 thoát 0, cây sạch. Warning: H5, H9, H11.
- `--refresh`: `needsRefresh: false`, cert sha `0e64df3`, không có manifest đổi.

**Tổng lượt squad.** 58 lượt (48 lượt cũ + 10 lượt vòng 3), trần 120.

**Đề xuất nội dung skill để đưa vào repo (chưa sửa).**
1. `sdlc-delivery-core`: Developer push nhánh của mình sau mỗi commit. Không push `main`.
2. `sdlc-qa-verification`: QA kiểm tra đúng commit trên nhánh được nêu trong handoff. Sau GO, QA land commit đó lên `main` bằng fast-forward. Nếu bị từ chối, báo cáo và dừng, không force.
3. Lead: không đưa cùng một commit qua hai issue QA. Cần quyết định issue QA nào là chính.

**Chưa kiểm chứng**
- Mỗi nhánh chỉ có một lần chạy. Không có đối chứng ngẫu nhiên.
- Trong vòng 3, Developer push vì skill đã yêu cầu rõ. Vì vậy không thể nói gì về việc Developer có tự push khi không được yêu cầu.
- Đường từ chối khi fast-forward thất bại chưa được kích hoạt.
- Chưa biết luật có chạy ổn khi có hai thay đổi cùng đưa lên `main`.
- Lead không đóng request (DEV-1 và DEV-17 đều `todo`).

### 5.12 Việc còn lại của pilot: daemon, Lead không đóng request, comment QA trên issue Developer

Ngày 2026-11-12, sau mục 5.11. Tổng lượt squad: 61 (trần 120). Các thao tác của operator được ghi kèm thời điểm.

**1. Daemon dừng khi run kết thúc: nguyên nhân chưa xác định hoàn toàn**

Đã dừng ba lần: 11:10:55, 11:39:17, 12:00:40. Cả ba đều là `context canceled` ở vòng lặp poll, không có log tín hiệu hay log yêu cầu shutdown. Đã kiểm tra trong code:
- Daemon thoát khi root context bị hủy. Context này hủy khi nhận SIGINT/SIGTERM (`notifyShutdownContext`), hoặc khi endpoint HTTP `POST /shutdown` trên health port được gọi (`shutdownHandler` trong `internal/daemon/health.go`). Cả hai đường đều không ghi log, nên không phân biệt được từ log.
- Đã loại trừ: tự khởi động lại do binary đổi (`auto_reload`), vì binary `bin/multica` có mtime 11:34, trước lần khởi động 11:42, và không có log reload. Các lệnh `syscall.Kill(-pid, …)` trong `processtree` và `execenv` chỉ nhắm vào process group của tiến trình con, không giết daemon.
- Giả thuyết còn lại: run kiro kết thúc dọn các tiến trình mang biến môi trường đánh dấu run (`KIRO_SESSION_ID`, `CLAUDE_CODE_SESSION_ID`, `AI_AGENT`), và daemon kế thừa các biến này vì được khởi động từ shell của tool. `setsid` không ngắt được điều này, vì nó chỉ đổi session, không đổi env.

Thử cách mới: daemon chạy dưới `systemd-run --user --unit=s8-daemon`, bọc `strace -f -e trace=kill,tkill,tgkill,exit_group`. Unit này nằm ngoài `session-3000.scope` của shell và không có biến đánh dấu. `strace` ghi `si_pid` của người gửi mỗi khi daemon nhận signal. Trạng thái khi khởi động (12:35): daemon PID 3730715, cha là strace PID 3730712, cgroup `app.slice/s8-daemon.service`, không có env marker. Bản ghi nằm ở `~/.cache/claude-delegate/20261112-130000-kiro/daemon-check.txt`, và log signal ở `s8-daemon-strace.txt` cùng thư mục.

Chưa kiểm chứng: daemon này đã sống qua khi run kết thúc hay chưa. Lần kiểm tra cuối (13:08) daemon vẫn chạy, uptime 44 phút. Chỉ khi run hiện tại kết thúc mới biết được. Nếu daemon dừng, `s8-daemon-strace.txt` sẽ có dòng `--- SIGTERM {si_pid=...}` (hoặc tương tự) với PID của người gửi.

**2. Lead không đóng request (DEV-1, DEV-17): nguyên nhân là thiếu cơ chế đánh thức**

Lead lập kế hoạch và tạo 12 issue (DEV-1) và 3 issue (DEV-17) với `--parent` trống, đúng với skill. Sau đó không có lượt nào của Lead trong suốt chuỗi. Nguyên nhân trong code: quy tắc hệ thống `child_done` (`internal/service/issue_wakeup_system.go`) chỉ được tạo cho issue có sub-issue (`EnsureChildDoneRule`). Quy tắc này đánh thức assignee của parent khi một stage của sub-issue đóng. Vì issue con không có parent, không có quy tắc nào đánh thức Lead. CLI cũng ghi điều này: "The parent assignee is woken only when every sub-issue in a stage finishes" (cờ `--stage`). Skill Lead Bước 8 yêu cầu báo cáo cuối và chuyển `Plan ready` sau khi kiểm tra cuối pass, nhưng không có trigger nào để chạy bước này.

Thao tác của operator (12:47, 12:49 và 12:55):
- Gán DEV-1 và DEV-17 vào project Build `8e9dd76f` (cả hai trước đó `project_id` null, nên workflow `Default` không có `plan_ready`). Lần đầu gọi `issue update --project` hết thời gian chờ (HTTP timeout). Lần thử lại thành công.
- Đặt `plan_ready`. Không tạo issue, không đổi repo.
- Comment "Operator note" trên cả hai request, ghi rõ không phải Lead viết, và nêu bằng chứng kiểm tra cuối.

Hệ quả không dự định: comment của operator đã đánh thức Lead (attribution: comment, initiator dev@localhost). Lead chạy lại:
- DEV-1: một lượt `failed` ("task preparation timed out after 5m0s"), một lượt chờ (`dispatched`) chạy thành công. Báo cáo cuối lúc 13:08.
- DEV-17: một lượt chạy thành công. Báo cáo cuối lúc 13:07.

Lượt `failed` do hạ tầng, không do logic: từ 12:52 đến 13:02 load trung bình của máy là 92 đến 106 trên 12 nhân. Trong khoảng đó API `18800` không trả lời heartbeat và gia hạn lease của daemon, và truy vấn tới Postgres `5432` timeout. Không xác định được tiến trình nào chiếm tài nguyên. Ở thời điểm kiểm tra (13:07), API healthy, và các tiến trình riêng lẻ đều dùng CPU thấp.

Báo cáo của Lead cho thấy hai điều, mà operator đã bỏ sót:
- Release notes (`docs/sdlc/release-notes.md`, `docs/sdlc/run.md`) nằm ở commit `6052d0c` trên nhánh cục bộ của DevOps, không có trên `origin/main`.
- Bằng chứng kiểm tra cuối (QA 10/10, security PASS) là cho `0ca88a2`. `main` đã sang `aa37f8d` (countOpen) sau đó, nên `aa37f8d` chưa có kiểm tra cuối riêng. Operator đã đặt `plan_ready` mà không nêu rõ điều này. Đây là sai sót của operator.

Đề xuất sửa skill (chưa áp dụng, cần người duyệt):
- `sdlc-squad-lead/SKILL.md`, "The workflow brief" hoặc Bước 1: tạo các issue trong chuỗi với `--parent <request>` và `--stage <n>`, để quy tắc `child_done` đánh thức Lead khi mỗi stage đóng và khi chuỗi đóng. Phải sửa câu "Create every issue ... with no parent".
- Hoặc, nếu giữ "no parent": thêm một issue kiểm tra cuối có parent là request, xếp sau cùng, để kích hoạt đánh thức.
- Bước 8: nêu rõ kiểm tra cuối là gì và ai đánh thức Lead để chạy nó.

**3. Comment "Landed on main" trên DEV-18 (issue Developer)**

Đã đọc toàn bộ comment và timeline của DEV-18 và DEV-20. Tác giả của comment trên DEV-18 là các agent theo thứ tự: SDLC Developer (11:50:03, 11:50:22), SDLC Code Reviewer (11:50:47, PASS), SDLC QA Tester (11:51:45, GO và "Landed on main by fast-forward"). Lý do QA viết trên DEV-18: workflow SDLC Auto đẩy chính issue Developer qua trạng thái `in_review` rồi `qa`, nên Reviewer và QA được đánh thức trên đúng issue đó. Các issue DEV-19 và DEV-20 do Lead tạo thêm cho bước 2 và 3, nên kiểm tra bị lặp.

Có gây hại không: không thấy. Push của QA trên DEV-18 là fast-forward của đúng commit `aa37f8d`, không force. Lần push lặp của DEV-20 trả "Everything up-to-date". Chi phí của việc lặp: 5 lượt (DEV-19: 4, DEV-20: 1).

Đề xuất: Lead không tạo issue review và QA riêng cho một issue Developer đã đi qua các stage đó trong workflow. Hoặc workflow không đẩy issue Developer vào `in_review` và `qa` khi Lead đã tạo issue riêng cho các bước này.

**Tổng kết lượt**: 61 (48 cũ, 10 vòng 3, 3 của Lead trên DEV-1 và DEV-17 sau khi operator comment). S9 không dùng lượt nào.

**4. Web pilot.** Web `13720` đã không phản hồi lần thứ hai (khoảng 13:10), dù tiến trình `next-server` vẫn sống. `make up C=web` từ chối khởi động vì cổng bị giữ. Sau khi xác minh bằng `web.listener.pid` của registry rằng tiến trình thuộc môi trường s8-pilot (thư mục làm việc là snapshot pilot), dừng tiến trình đó và khởi động lại bằng `make up C=web ARGS="--name s8-pilot"`. Lần này HTTP 200 (0,2 giây).

### 5.13 Kiểm chứng luồng đóng request (DEV-21)

Ngày 2026-11-12, sau mục 5.12. Mục đích: xem sau khi chuỗi issue kết thúc, Lead có tự viết báo cáo cuối và chuyển request sang `plan_ready` không. Kết quả: **(a) FAIL, (b) FAIL, (c) không xảy ra.** Chuỗi bị kẹt ở `in_review`.

**Việc 1: đưa skill mới vào pilot.** PASS.
- HEAD `647d9e59e` được xuất bằng `git archive` vào `~/.cache/claude-delegate/multica-s8-647`. Không dùng `git worktree`, `checkout` hay `reset`.
- Chép `.env.worktree` của pilot (PORT 18800, FRONTEND_PORT 13720, `POSTGRES_DB=multica_multica_s8_720`). Không có `.env`. Không chạy `make up` trong thư mục mới, để không tạo môi trường thứ hai.
- `node_modules` là liên kết tới snapshot cũ, chỉ để dùng `esbuild`. `scripts/generate-sdlc-skills.mjs` sinh 77 skill.
- `scripts/upgrade-sdlc-squad.mjs`: dry-run (squad `current`, workflow `current`), rồi `--apply` (skills `ready`).
- Đọc qua API theo id: `sdlc-squad-lead` có `--until-children-done` và câu "children of the request so that the request's wait counts them". Câu này trong file nguồn bị xuống dòng giữa "the" và "request", nên tìm theo chuỗi nguyên văn sẽ không khớp.

**Việc 2: tạo request và theo dõi.** Request `DEV-21` (id `01a1ce90-a99d-77be-b30c-ec0b7324953d`), tên "S8 round 4: add sumOpen and test the close flow", project Build `8e9dd76f`, giao cho squad `691a1da9`, tạo lúc 14:02:36. Yêu cầu: một epic, thêm `sumOpen(list)` và test.

| Thời điểm | Sự kiện |
| --- | --- |
| 14:02:42 | Lead chạy lượt 1. Không tạo issue con, không tạo comment kế hoạch, không đăng ký wakeup. Giao việc cho Developer trên chính `DEV-21` |
| 14:03:35 | Developer nhận, `todo` → `in_progress` (14:04:01) |
| 14:04:23 | Developer chuyển `in_progress` → `in_review`, comment handoff |
| 14:05:12 | Lead được đánh thức (lượt 3). Ghi `squad_leader_evaluated` với `outcome: no_action`: "review belongs to the Code Reviewer step and the lead does not move status on this issue" |
| 14:05:34 đến 14:53 | Không có lượt nào mới trên `DEV-21`. Trạng thái giữ `in_review` |

**Điều kiện (a): issue con của request có `parent` = request.** FAIL. Không có issue nào có `parent` = `DEV-21`. Lead không tạo QA, security, DevOps. Skill yêu cầu Bước 4 bất kể quy mô, nên đây là lệch chuẩn của Lead: nó bỏ qua Bước 2 (chọn kế hoạch) đến Bước 4 và giao thẳng.

**Điều kiện (b): wakeup `--until-children-done` trên request.** FAIL. `multica issue wakeup list DEV-21` trả `[]`. Lead không đăng ký wakeup nào.

**Điều kiện (c): Lead được đánh thức, viết báo cáo cuối và chuyển `plan_ready`.** Không xảy ra. Nguyên nhân trực tiếp: không có wakeup (b) và không có issue con (a). Điều kiện (c) còn bị chặn bởi một lỗi khác, xem bên dưới.

**Lỗi thêm: chuỗi kẹt ở `in_review` mà không có lượt Reviewer.** Trạng thái `in_review` của workflow SDLC Auto có handler là SDLC Code Reviewer (`0ef038e0`). Sau khi Developer chuyển sang `in_review` lúc 14:04:23, không có lượt Reviewer nào được xếp hàng trong 50 phút. Đối chiếu với DEV-18 (làm trước, cũng đi qua `in_review` và `qa`): DEV-18 được giao cho từng agent, và khi chuyển trạng thái, assignee được đổi sang handler của bước (kết thúc là agent QA `5a111f69`). DEV-21 được giao cho squad, và assignee giữ nguyên là squad. Lead cũng không chuyển giao lại. Trong `applyIssueWorkflow` (`internal/handler/issue_workflow_entry.go`) có điều kiện bỏ qua handoff khi `assigneeTouched` hoặc `triage_state` có giá trị. Tôi không lần đến cùng để xác định cơ chế chính xác. Điều quan sát được là: issue giao cho squad không được đổi người giao khi chuyển bước, và không có lượt nào được đánh thức ở bước `in_review`.

**Bằng chứng số lượt.** Tổng 64 lượt (đếm lại với timeout, 0 dòng lỗi parse). Chuỗi DEV-21 dùng 3 lượt. Trần 120 chưa bị chạm.

**Không đặt `plan_ready` thủ công**, theo yêu cầu, để đo luồng thật. Không tạo issue thủ công.

**Daemon pilot.** Chạy dưới `s8-daemon.service` với strace (PID 3730715, cha strace 3730712, cgroup `app.slice/s8-daemon.service`). Lúc kiểm tra cuối (14:53): uptime 2 giờ 20 phút, còn chạy. Log strace (`~/.cache/claude-delegate/20261112-130000-kiro/s8-daemon-strace.txt`) ghi trong khoảng 12:35 đến 14:53:
- Không có signal nào kết thúc gửi tới daemon (không SIGTERM, SIGINT, SIGKILL). Phần lớn là SIGURG và SIGCHLD do chính Go runtime và các tiến trình con.
- Có 2 SIGTERM, do tiến trình PID 3787491 (con của daemon, đã thoát) gửi tới hai tiến trình con khác của nó. Đây là dọn dẹp tiến trình con của task, không phải dừng daemon.
- Khoảng thời gian này bao gồm ít nhất một ranh giới run kiro (run trước kết thúc khoảng 13:10). Đây là bằng chứng gián tiếp rằng unit `systemd --user` tránh được việc dừng khi run kết thúc. Chưa kiểm chứng ở nhiều ranh giới hơn.

**Web pilot `13720`: không phản hồi, không khôi phục được trong phiên này.** Các lần khởi động lại:
- 14:18:24, `systemctl --user restart s8-web`. Đến 90 giây sau vẫn không phản hồi (HTTP 000).
- Khoảng 14:50, lần thứ hai, cũng không phản hồi sau 90 giây.
- Trước đó, lúc 13:20, `make up C=web` đã khôi phục được (HTTP 200).
- Bộ nhớ lúc kiểm tra: 17 đến 19 GB dùng trên 23 GB, swap 11 GB. Load trung bình 31 đến 106 trong các lần đo. Đây là khả năng nguyên nhân, nhưng chưa xác nhận.

**Đã dùng hết hai lần thử cho web** theo giới hạn phiên. Dừng ở đây.

**Điểm cần sửa (chỉ đề xuất, chưa áp dụng):**
1. Skill Lead: không được giao thẳng request cho Developer. Bước 2 và Bước 4 phải chạy kể cả khi chỉ có một epic. Nêu rõ điều này trong skill, vì Lead đã tự suy ra đường tắt "một epic, giao thẳng".
2. Workflow: giải thích lý do `in_review` không đánh thức handler khi issue giao cho squad. Có thể cần đổi người giao sang handler khi chuyển bước, giống DEV-18.
3. Hạ tầng pilot: giảm tải hoặc tăng bộ nhớ, rồi kiểm tra web lại. Lỗi web và lỗi `task preparation timed out` ở 5.12 có thể cùng nguyên nhân.

**Chưa xác nhận:** chuỗi đầy đủ (a), (b), (c) vì Lead không đi đến đó. Nguyên nhân chính xác của việc `in_review` không đánh thức Reviewer. Web chưa khôi phục.

### 5.14 Round 5: luồng đóng request chạy đến plan_ready (DEV-22)

Sau khi sửa skill Lead (cấm giao việc cho chính request, và mọi việc phải là issue con), tôi tạo DEV-22 cùng nội dung `sumOpen` như round 4, trong project Build, giao cho squad lúc 15:09. Lượt squad đã dùng trước round này: 64, trần 120. Round này dùng khoảng 15 lượt.

**Kết quả ba điều kiện**

- **(a) Issue con có `parent` = DEV-22: PASS.** DEV-24 (QA), DEV-25 (security), DEV-26 (DevOps) đều có `parent` = DEV-22 và xếp hàng sau epic DEV-23.
- **(b) Wakeup `--until-children-done` trên DEV-22: PASS.** Lead (agent `8ba2f81e`) đăng ký lúc 15:10:47. Wakeup đã kích hoạt và tự tắt sau khi chuỗi xong.
- **(c) Lead viết báo cáo cuối và tự chuyển `plan_ready`: PASS.** Báo cáo cuối được đăng lúc 16:14:07, request chuyển `plan_ready` lúc 16:15:01, cả hai do agent Lead thực hiện. Operator không đặt trạng thái thủ công. `sumOpen` đã vào `main` ở commit `a831612` (QA fast-forward sau GO, 18/18 test, Node v24.22.0).

**Can thiệp thủ công (phải ghi rõ):** epic DEV-23 chuyển sang `in_review` lúc 15:32 và giao cho Code Reviewer, nhưng không có lượt Reviewer nào chạy đến 15:47. Lead không được đánh thức trong lúc chuỗi còn chạy, nên không có cơ chế nào tự phục hồi. Operator chạy lại DEV-23 một lần lúc 15:47, lượt Reviewer `01a1cef0` dispatch ngay sau đó. Đây là lỗi trùng với round 4 (DEV-21), chưa sửa ở workflow.

**Điểm chưa khớp trong báo cáo cuối của Lead:**
- Báo cáo ghi "every sub-issue has closed", nhưng DEV-25 và DEV-26 đang ở `stage_done`, chưa `done`. Lead vẫn chuyển request sang `plan_ready`.
- DEV-21 (round 4) vẫn `in_review` và nhắm cùng `sumOpen`. Đây là nguy cơ trùng lặp, cần dọn.
- Ba finding Low của security (S-1, S-2 và một finding khác) để mở, không chặn.

**Kết luận:** skill Lead đã đủ để chuỗi đi đến `plan_ready` mà không cần Lead bị đánh thức thủ công. Gap còn lại nằm ở workflow: issue giao cho squad không được đánh thức Reviewer ở `in_review`. Cần sửa ở code workflow hoặc chuyển người giao sang handler của bước, giống DEV-18. Chưa áp dụng.

**Dịch vụ sau round 5:** daemon `s8-daemon` (PID 3730715) và web `s8-web` đều chạy khi kiểm tra cuối.
