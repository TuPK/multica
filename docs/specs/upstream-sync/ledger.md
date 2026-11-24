# Sổ các lần sync upstream

Mỗi lần sync thêm một dòng. "Conflict gốc" là số file git báo chưa merge khi không có driver; "còn lại" là số file người phải giải sau khi dùng driver (`pnpm sync:simulate`, hoặc số thật nếu đã sync bằng quy trình mới). Chỉ ghi số đo được; để trống khi chưa biết.

| Ngày | Merge | Commit upstream | Conflict gốc | Còn lại | Giờ người | Mất tính năng sau sync | Ghi chú |
|---|---|---|---|---|---|---|---|
| 2026-11-22 | `e4f7bf2ad` | 588 | 301 | | | | sync #1, chưa mô phỏng với driver |
| 2026-11-22 | `7b6a1ab8f` | 53 | 97 | 29 (mô phỏng) | | 14 khóa `composio_mcp` ở locale bị bỏ, phát hiện và khôi phục sau | merge thật bỏ khóa này ở cả `en`; driver giữ |
| 2026-11-23 | `ce85c3b61` | 27 | 75 | 37 (mô phỏng) | | | |
| 2026-11-24 | `0ecf290b7` | 114 | 85 | 30 (mô phỏng) | | | sync #4 |
| 2026-11-24 | `25b62a720` | 36 | 27 (thật, sau driver) | 27 | | không | sync #5, lần đầu theo runbook; `squad_handoff.go` trùng đường dẫn nên bản fork đổi sang `*_fork.go`; fork đã gỡ telemetry nên bỏ `RecordHumanDelegation` khỏi `doc_task.go` |

Số "còn lại" của ba dòng cuối là mô phỏng bằng driver trong repo (`pnpm sync:simulate --merge <commit>`), không phải số lúc đó người giải.
