# SDLC specialist routing: báo cáo Giai đoạn 3

Ngày 2026-11-07. Kèm [spec](sdlc-specialist-routing.md) và [plan](sdlc-specialist-routing-plan.md).
Mọi kết quả đọc từ database và comment thật của agent chạy trên máy này (runtime Codex).

## Câu hỏi mở

| # | Câu hỏi | Kết quả | Bằng chứng |
| --- | --- | --- | --- |
| Q2 | Giao việc cho agent đang `offline` (runtime `online`) có chạy không | Có | TUPK-113: Architect (offline) nhận và hoàn tất |
| Q4 | Wakeup `--until-issue` gắn vào issue của Lead hoạt động không | Có, với điều kiện issue kia ở trạng thái `done`/`closed` | TUPK-115/116: Lead được đánh thức khi 116 sang `done`; ở `in_review` thì chưa đủ. Unblock issue kết thúc ở `Stage done` (category done) nên đúng điều kiện |
| Q5 | Runtime Codex có cho DevOps dùng Docker không | Có | TUPK-114: `docker version` và `docker ps` chạy được, thấy các container Supabase của mua_ngon |

Q3 (parent/child) không cần trả lời: thiết kế dùng issue không parent và nó chạy đúng.

## Nâng cấp squad mua_ngon_ecommerce

Sao lưu trước (workflow, agent, squad, skill) rồi chạy `scripts/upgrade-sdlc-squad.mjs --apply`:

- Workflow `SDLC Auto` có step `environment` (handler SDLC DevOps), brief của 9 step được thay.
- Instruction của Lead, Developer, DevOps được thay; các agent khác đã là bản hiện tại.
- 11 skill SDLC cập nhật, Lead có `reference.md` và `team-playbook.md`.
- Chạy lại lần hai: mọi thứ `current`, không đổi gì (idempotent).
- Lúc áp dụng Epic 8 (TUPK-101) đang chạy; thay đổi chỉ có hiệu lực từ lần giao việc kế tiếp.

## Diễn tập (project thử `zz-drill-routing`, repo thử `~/Documents/zz-drill-env`)

| # | Kịch bản | Kết quả |
| --- | --- | --- |
| S1 | Môi trường: PostgreSQL của project thử không chạy, báo `Blocker-class: environment` | Đạt. Lead phân loại, tạo TUPK-118 cho DevOps ở `environment` không parent, xếp hàng không `--after`, đăng ký wakeup. DevOps probe, viết `scripts/env-ensure.sh`, stack QA riêng cổng khác, dừng rồi dựng lại để chứng minh, `docs/sdlc/environment.md`, commit. Lead được đánh thức, tự chạy lại `env-ensure.sh`, kiểm chứng. Không có proposal nào gửi người dùng. Tổng: Lead 2 lượt, DevOps 1 lượt, khoảng 6 phút |
| S2 | Yêu cầu: hai AC mâu thuẫn | Đạt. Lead giao Product Analyst ở `analysis`. Analyst ra Ruling (AC cụ thể hơn thắng), ghi `rulings.md` R-001 và assumption, commit, nêu tác động. Lead resume về `in_progress` |
| S3 | Thiết kế: chưa có hợp đồng phân trang cho API công khai | Đạt. Lead giao Architect ở `architecture`. Architect ra ADR-002 (keyset), hợp đồng `get-orders.md`, R-002, commit. Lead resume về `in_progress` |
| S4 | Vòng lặp review lần ba | Chưa chạy: cần ba vòng Developer và Reviewer thật, tốn nhiều lượt chạy. Test tự động chỉ kiểm văn bản skill |
| S5 | Finding bảo mật | Chưa chạy |
| S6 | Thiếu vai trò trong roster | Chưa chạy; phía server đã có test (`TestSDLCStepsOmitStagesWithoutAnAgent`) |

Sau khi Lead resume S2 và S3, Developer bắt đầu chạy ngay trên issue thử; các run đã được hủy và các issue thử chuyển `cancelled`.

## Lỗi tìm thấy khi diễn tập, đã sửa

1. **DevOps tự chuyển issue bị block sang `qa`** ngay sau khi xong (S1). Issue bị kẹt ở `qa` giao cho Lead thay vì QA, và Lead tự phát verdict "GO". Sửa: Ruling của chuyên gia không được chuyển, giao hay comment trên issue bị block (delivery-core, DevOps Environment mode); Lead là người resume, chỉ kiểm phán quyết và không đưa verdict review/QA hay đánh dấu done (Step 7.6). Kiểm lại ở S2 và S3: Analyst và Architect không đụng issue bị block, Lead resume đúng.
2. Lead xếp unblock issue của S3 lúc đầu thấy `run_start_status = todo` trong vài giây, rồi thành `architecture` sau bước queue; không ảnh hưởng kết quả.

## Chưa kiểm chứng

- S4, S5, S6 trên agent thật.
- Hành vi trên một block thật của mua_ngon: epic 8 trở đi chưa bị block kể từ khi nâng cấp.
- Chỉ số mục 8 của spec trước/sau: cần các block thật để so; số "trước" nằm ở mục 2 của spec.
- Lead và DevOps chạy bằng cùng runtime Codex; chưa thử với runtime khác.

## Giai đoạn 4: bộ đếm vòng trả lại phía server

Lý do: S4 chưa chạy được và việc Developer tự đếm là một bước LLM có thể quên; dữ liệu thật có issue bị trả lại 5 lần
(Epic 1). Hành vi mới (`reworkLoopFor` trong `issue_workflow_entry.go`): khi agent xử lý bước `in_review` hoặc `qa`
chuyển issue về `in_progress` lần thứ ba kể từ lần block gần nhất, server chuyển issue sang `blocked` thay thế và đăng
báo cáo `Blocker-class: rework-loop` theo đúng khuôn của delivery-core, nên Lead định tuyến như mọi blocker khác. Chỉ áp
dụng khi workflow giao `blocked` cho một agent và bước review/QA có `back` về `in_progress`; thao tác của thành viên người
dùng không bị đổi. Bộ đếm đọc từ `activity_log` (truy vấn `CountWorkflowReturns`) và bắt đầu lại sau mỗi lần block, nên
sau phán quyết của chuyên gia, vòng review mới có đủ lượt trả lại riêng.

Test (Go, cần DB): lần trả lại thứ ba bị chặn, lần thứ hai vẫn về `in_progress`, đếm lại sau block, thành viên không bị ảnh
hưởng. `TestHandoffStopsPreviousSquadRuns` fail cả trước thay đổi này.

Chưa kiểm trên agent thật: cần một issue thật bị Reviewer trả lại ba lần.
