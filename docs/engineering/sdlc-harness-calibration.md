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
