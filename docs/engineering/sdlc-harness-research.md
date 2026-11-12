# SDLC squad: nghiên cứu harness và chọn phương án

Tài liệu nghiên cứu. Đi kèm [sdlc-harness-spec.md](sdlc-harness-spec.md) (thiết kế của phương án được chọn) và
[sdlc-harness-plan.md](sdlc-harness-plan.md) (kế hoạch triển khai). Thiết kế hiện tại nằm ở [sdlc-harness.md](sdlc-harness.md).

Ngôn ngữ: viết bằng tiếng Việt để người dùng đọc; tên lệnh, file, tên kiểm tra và thuật ngữ kỹ thuật giữ nguyên tiếng Anh.

Ngày truy cập tất cả nguồn: **2026-11-12**. Nội dung nguồn ngoài được tóm tắt bằng lời của tôi, không chép dài.

## 1. Câu hỏi và cách nghiên cứu

Câu hỏi: harness nào giúp một AI Agent trong SDLC Squad code hiệu quả trên **mọi** dự án được giao, không chỉ một dự án?
Hiệu quả nghĩa là ít vòng prompt hơn cho phần việc cơ bản, ít lần báo "xong" sai, chi phí token không phình.

Cách làm: đọc tài liệu chính thức của Anthropic và của các công cụ khác, các bài đo định lượng, các bài thực hành cộng đồng,
rồi đối chiếu với thiết kế hiện tại và với kết quả chạy `check-harness` trên chính repo multica.

Các truy vấn web chỉ dùng từ khóa chung (không có tên dự án, tên người dùng, đường dẫn, hay chi tiết nội bộ):

- `evaluating AGENTS.md: are repository-level context files helpful for coding agents`
- `harness engineering agent-first world Codex AGENTS.md table of contents repository knowledge system of record`
- `Writing a good CLAUDE.md instruction budget how many instructions LLM can follow`
- `SWE-bench Verified contamination flawed tests agent scaffold harness matters same model different scaffold score difference`
- `spec-driven development GitHub Spec Kit Martin Fowler tools Kiro spec-kit Tessl`
- `Lulla impact of AGENTS.md files on efficiency of AI coding agents wall-clock runtime output tokens`
- `Cursor rules AGENTS.md support GitHub Copilot custom instructions AGENTS.md Codex discovery 32 KiB`
- `hesreallyhim awesome-claude-code resources hooks CLAUDE.md`

Thang tin cậy dùng trong tài liệu này:

| Nhãn | Nghĩa |
| --- | --- |
| **Chính thức** | Tài liệu hoặc blog kỹ thuật do chính nhà cung cấp công cụ viết. Đúng về cách công cụ hoạt động; không chứng minh hiệu quả. |
| **Dữ liệu** | Nghiên cứu có số đo, có phương pháp. Ghi cỡ mẫu và hạn chế. |
| **Thực hành** | Báo cáo của một đội về kinh nghiệm riêng, không có đối chứng. Có giá trị gợi ý, không phải bằng chứng. |
| **Ý kiến** | Bài viết cộng đồng, thường trích lại nhau. |
| **Chỉ đọc đoạn trích** | Tôi chỉ thấy đoạn trích trong kết quả tìm kiếm, không mở được toàn văn. Không dùng làm căn cứ chính. |

## 2. Nguồn đã đọc

### 2.1 Anthropic, tài liệu Claude Code (Chính thức)

**A1. How Claude remembers your project** (CLAUDE.md và memory). https://code.claude.com/docs/en/memory. Anthropic, tài liệu sống.
- CLAUDE.md là ngữ cảnh, không phải cấu hình được cưỡng chế. Muốn chặn một hành động thì dùng hook `PreToolUse`.
- Nên dưới 200 dòng mỗi file; file dài hơn tiêu thụ ngữ cảnh và làm giảm mức tuân thủ. Hướng dẫn chuyển phần chỉ liên
  quan một vùng sang rule có `paths` hoặc skill.
- `@path` import chỉ giúp tổ chức file, **không giảm chi phí ngữ cảnh**, vì file được import nạp cùng file gốc.
- Chú thích HTML cấp khối trong CLAUDE.md bị gỡ trước khi đưa vào ngữ cảnh (dùng cho ghi chú của người bảo trì).
- Nếu có cả `AGENTS.md` lẫn `CLAUDE.md`, Claude Code chỉ đọc `CLAUDE.md`; muốn đọc cả hai thì `CLAUDE.md` import `@AGENTS.md`.
  Đọc `AGENTS.md` trực tiếp cần Claude Code từ v2.1.277.
- Hai lệnh hỗ trợ: `/doctor` đề xuất cắt thứ Claude tự suy ra được từ code; `/doctor prompt-audit` bắt các chỉ dẫn viết cho
  model cũ, trỏ tới file không còn, hoặc mâu thuẫn nhau.
- Áp dụng: giữ `CLAUDE.md` = `@AGENTS.md`; ngưỡng 200 dòng là **trần**, không phải tối ưu; kiểm tra "đường dẫn còn tồn tại"
  và "không mâu thuẫn" là kiểm tra có căn cứ chính thức.

**A2. Best practices for Claude Code.** https://code.claude.com/docs/en/best-practices. Anthropic, tài liệu sống.
- Chỉ ghi vào CLAUDE.md thứ Claude không tự suy ra được từ code: lệnh xác nhận công việc (test, typecheck, lint, cách chạy
  một file test), chỗ code gây hiểu nhầm, quyết định không có trong repo, và điều đã phải sửa hai lần.
- "Cho Claude một phép kiểm tra để chạy" là đòn bẩy lớn nhất: nếu repo đã có test hoặc typecheck, dòng hữu ích nhất trong
  CLAUDE.md là lệnh chạy nó. Hướng dẫn nêu mức cưỡng chế từ yếu đến mạnh: lời nhắc, `/goal`, hook `Stop`, reviewer là
  subagent không thấy cuộc hội thoại.
- Hook `Stop` thoát bằng mã 2 thì đẩy agent quay lại làm; có giới hạn tám lần chặn liên tiếp. Luật không có ngoại lệ phải
  nằm ở hook, deny rule hoặc managed setting, không phải CLAUDE.md. Giữ tập cưỡng chế nhỏ vì mỗi hook thỉnh thoảng bắn nhầm.
- Hook loại `command` mới tất định; hook `prompt`/`agent` nhờ một model khác quyết định nên không tất định.
- Nên đòi **bằng chứng** (lệnh đã chạy và đầu ra), không chấp nhận câu "tất cả test đều qua".
- Áp dụng: Done command phải chạy được và có đầu ra kiểm chứng; cưỡng chế nằm ở code/CI/hook; prose chỉ cho phần còn lại.

**A3. Extend Claude with skills** (gồm slash command, đã gộp vào skills). https://code.claude.com/docs/en/skills.md. Anthropic.
- Skill nạp theo ba mức: tên và mô tả luôn có trong ngữ cảnh; thân `SKILL.md` nạp khi cần; file kèm nạp khi được gọi.
  `description` cộng `when_to_use` bị cắt ở 1.536 ký tự trong danh sách skill. Giữ `SKILL.md` dưới 500 dòng.
- Nội dung skill đã nạp **ở lại ngữ cảnh** qua các lượt, nên mỗi dòng là chi phí lặp. Quy tắc "chỉ ghi điều Claude không tự
  tìm được" áp dụng cho skill như cho CLAUDE.md.
- `disable-model-invocation: true` cho quy trình có tác dụng phụ.
- Áp dụng: skill `sdlc-harness` hiện 131 dòng, nằm trong giới hạn; kiến thức theo tình huống đặt ở `references/` hoặc docs,
  không đặt ở AGENTS.md.

**A4. Hooks guide.** https://code.claude.com/docs/en/hooks-guide.md (đọc có chọn lọc). Anthropic.
- Mã thoát 2 chặn hành động và đưa stderr cho agent; mã khác 0 và 2 thường không chặn. `Stop` hook cần kiểm
  `stop_hook_active` để tránh vòng lặp.
- Áp dụng: hook `Stop` chạy `Done --quick` là một **bộ chuyển (adapter) tùy chọn** chỉ cho runtime Claude Code; không thể là
  cơ chế chung vì squad dùng nhiều runtime.

**A5. Configure permissions.** https://code.claude.com/docs/en/permissions.md (đọc có chọn lọc). Anthropic.
- Quy tắc quyền do Claude Code cưỡng chế, không phải do model; thứ tự deny, ask, allow.
- Áp dụng: xác nhận nguyên tắc "luật không ngoại lệ thì không viết bằng văn xuôi".

**A6. Create custom subagents.** https://code.claude.com/docs/en/sub-agents.md (đọc có chọn lọc). Anthropic.
- Mỗi subagent có ngữ cảnh riêng, danh sách công cụ riêng, giới hạn công cụ do Claude Code cưỡng chế. Tổng mô tả subagent
  trên 15.000 token sẽ bị cảnh báo. Subagent `Explore` và `Plan` khởi động **không** nạp CLAUDE.md.
- Áp dụng: reviewer chỉ đọc (`tools: Read, Grep, Glob`) là cơ chế kiểm tra độc lập rẻ; SDLC Squad đã có vai Reviewer/QA nên
  không cần thêm cấu hình ở harness.

**A7. Use Claude Code features in the SDK.** https://code.claude.com/docs/en/agent-sdk/claude-code-features.md (đọc có chọn lọc). Anthropic.
- Agent SDK nạp CLAUDE.md, rules, skills, hooks qua `settingSources`; bỏ qua tùy chọn này thì nạp user, project, local.
  CLAUDE.md ở thư mục con nạp theo nhu cầu; các cấp cộng dồn, không có thứ tự ưu tiên cứng nếu hai chỉ dẫn mâu thuẫn.
- Áp dụng: nội dung harness commit vào repo được các agent chạy bằng Claude Code/SDK đọc; chỉ dẫn mâu thuẫn giữa các cấp
  là rủi ro thật, nên H-check về mâu thuẫn và trùng lặp có giá trị.

### 2.2 Anthropic, blog kỹ thuật (Chính thức)

**A8. Effective context engineering for AI agents.** https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents. Anthropic, 2025 (ngày không hiện trong bản tải).
- Ngữ cảnh là tài nguyên hữu hạn, hiệu suất giảm dần khi số token tăng ("context rot"). Mục tiêu: tập token **nhỏ nhất** mà
  tín hiệu cao. Nhỏ nhất không có nghĩa ngắn; vẫn phải đủ để agent tuân thủ.
- Ưu tiên ví dụ chuẩn, đa dạng thay vì một danh sách ngoại lệ dài. Truy xuất "đúng lúc" bằng định danh nhẹ (đường dẫn, truy vấn)
  và khám phá dần dần (progressive disclosure). Claude Code là mô hình lai: CLAUDE.md nạp trước, glob/grep truy xuất sau.
- Với việc dài hơi: ghi chú có cấu trúc ngoài cửa sổ ngữ cảnh, compaction, subagent.
- Áp dụng: AGENTS.md là phần "nạp trước" nhỏ nhất; mọi thứ khác là con trỏ có điều kiện tới tài liệu agent tự đọc khi cần.

**A9. Effective harnesses for long-running agents.** https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents. Anthropic, cuối 2025 (ngày không hiện trong bản tải).
- Hai kiểu hỏng khi chạy nhiều cửa sổ ngữ cảnh: cố làm quá nhiều một lúc và để lại việc dở dang không ghi lại; về sau một phiên
  nhìn thấy tiến độ rồi tuyên bố xong.
- Giải pháp thử nghiệm: agent khởi tạo dựng môi trường (script `init.sh`, file tiến độ, commit đầu), agent code mỗi phiên làm
  **một** tính năng rồi để lại trạng thái sạch. Danh sách tính năng dạng **JSON** với trường `passes`, vì model ít sửa nhầm JSON
  hơn Markdown. Mỗi phiên bắt đầu bằng: xem thư mục, đọc git log và file tiến độ, chọn tính năng, **chạy kiểm tra cơ bản
  end-to-end trước khi viết tính năng mới**.
- Agent hay đánh dấu xong mà chưa kiểm tra đầu-cuối; khi được yêu cầu dùng công cụ tự động hóa trình duyệt như người dùng thật,
  chất lượng tăng rõ rệt. Hạn chế: công cụ trình duyệt không thấy hộp thoại alert gốc.
- Áp dụng: Ensure command là `init.sh` của ta; Done command cộng bước smoke đầu phiên là "kiểm tra cơ bản trước khi làm";
  trạng thái tiến độ đã có ở issue, comment và git, nên **không** thêm file tiến độ mới.

**A10. Demystifying evals for AI agents.** https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents. Anthropic, 2026 (ngày không hiện trong bản tải).
- Bắt đầu sớm với **20 đến 50 task đơn giản lấy từ lỗi thật**; chấm bằng grader tất định khi có thể (test qua hay không);
  tách eval khả năng (tỉ lệ qua thấp, để leo dốc) và eval hồi quy (gần 100%).
- Agent không tất định: dùng `pass@k` và `pass^k`. Đọc transcript để biết grader có chấm đúng không. Không chấm cứng theo
  chuỗi bước đi của agent.
- Áp dụng: khung đo hiệu quả harness (mục 10 của spec), gồm "eval pack" nhỏ lấy từ lỗi thật.

**A11. Equipping agents for the real world with Agent Skills.** https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills. Anthropic, cập nhật 2025-12-18 theo trang.
- Progressive disclosure là nguyên tắc cốt lõi; mã chạy được cho việc cần tính tất định; **bắt đầu bằng đánh giá**: tìm chỗ
  agent vấp rồi mới viết skill; nhờ agent tự rút ví dụ thành công và lỗi thường gặp vào skill.
- Áp dụng: nội dung Gotchas trong AGENTS.md sinh từ chỗ agent thật sự vấp, không sinh từ suy đoán.

**A12. Using CLAUDE.md files.** https://claude.com/blog/using-claude-md-files. Anthropic, không ghi ngày.
- Khuyên đưa vào CLAUDE.md một tóm tắt dự án và cây thư mục chính, dùng `/init` làm điểm khởi đầu, rồi lặp. Khuyên giữ gọn
  và chỉ thêm khi gặp vấn đề thật.
- **Mâu thuẫn với dữ liệu mới**: khuyến nghị bản đồ thư mục này bị C1 phản bác (xem mục 5, điểm 2). Tài liệu A2 mới hơn đã đổi sang nguyên tắc
  "chỉ điều không tự suy ra được". Trong thiết kế ta theo A2 và C1.

### 2.3 Chuẩn mở và công cụ khác (Chính thức)

**B1. AGENTS.md.** https://agents.md. Cộng đồng do Agentic AI Foundation (Linux Foundation) bảo trợ; khởi xướng có OpenAI Codex, Amp, Jules, Cursor, Factory.
- Markdown thường, không trường bắt buộc; file gần nhất với file đang sửa thắng; prompt của người dùng thắng tất cả. Agent sẽ
  chạy các lệnh test nếu bạn liệt kê. Monorepo dùng AGENTS.md lồng nhau. Không nêu giới hạn kích thước.

**B2. Codex: AGENTS.md.** https://learn.chatgpt.com/docs/agent-configuration/agents-md (chuyển hướng từ developers.openai.com/codex/guides/agents-md). OpenAI.
- Nối các file từ gốc xuống thư mục hiện tại, tối đa một file mỗi thư mục, dừng khi tổng đạt `project_doc_max_bytes`, mặc định
  **32 KiB**. Có `AGENTS.override.md`.
- Áp dụng: trần cứng liên-công-cụ gần nhất mà ta thấy có số cụ thể. Dùng làm cơ sở cho trần của chuỗi AGENTS.md.

**B3. Cursor Rules.** https://cursor.com/docs/context/rules. Cursor.
- Hỗ trợ AGENTS.md ở gốc và thư mục con (lồng nhau, file gần hơn ưu tiên). Rule `.mdc` có bốn kiểu áp dụng
  (luôn, thông minh theo mô tả, theo glob, thủ công). Rule tham chiếu file bằng đường dẫn thì agent đọc khi cần, không nhúng sẵn.

**B4. GitHub Copilot, repository custom instructions.** https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions. GitHub.
- Ba loại: toàn repo, theo đường dẫn, và agent instructions (một hoặc nhiều `AGENTS.md`, file gần nhất ưu tiên); hoặc một
  `CLAUDE.md`/`GEMINI.md` ở gốc.

**B5. Kiro steering.** https://kiro.dev/docs/steering/. Kiro/AWS.
- Hỗ trợ AGENTS.md ở gốc, thư mục con và `~/.kiro/steering/`; AGENTS.md **luôn được nạp**, không có chế độ nạp có điều kiện
  như file steering riêng (`always`, `fileMatch`, `manual`, `auto`). Có cú pháp `#[[file:...]]` để tham chiếu file sống.
- Áp dụng cho B1 đến B5: một `AGENTS.md` ở gốc là mẫu số chung; nạp có điều kiện không đồng nhất giữa các công cụ, nên
  **con trỏ có điều kiện bằng văn bản** (agent tự đọc file khi gặp điều kiện) là cách duy nhất chạy giống nhau ở mọi nơi.

### 2.4 Nghiên cứu có dữ liệu (Dữ liệu)

**C1. Evaluating AGENTS.md: Are Repository-Level Context Files Helpful for Coding Agents?** Gloaguen và cộng sự (ETH Zurich, LogicStar.ai). arXiv:2602.11988; bản v1 2026-02-12, v3 2026-09-29; trình bày ở ICLR 2026. https://arxiv.org/abs/2602.11988 (số liệu lấy từ HTML v3: https://arxiv.org/html/2602.11988v3).
- Thiết lập: tác vụ SWE-bench Lite (có file ngữ cảnh do LLM sinh) và **CTXbench** tự dựng từ issue mới của repo có sẵn file ngữ
  cảnh do lập trình viên viết; bốn agent/model (gồm Claude Code với Sonnet 4.5, GPT-5.2, GPT-5.1 mini, Qwen3-30b-coder). Python.
- Kết quả: file do LLM sinh làm tỉ lệ giải giảm trung bình 0,5% (SWE-bench) và 2% (CTXbench), **không có ý nghĩa thống kê**
  (p = 87% và 37%); số bước tăng 2,45 và 3,92; **chi phí tăng 20% và 23%**, có ý nghĩa thống kê. File do lập trình viên viết cải
  thiện trung bình 2,4% (p = 21%, không có ý nghĩa) và hơn file LLM sinh một khoảng có ý nghĩa; chi phí tăng tới 19%.
- Cơ chế: agent **tuân thủ** chỉ dẫn trong file, nên chạy test và khám phá nhiều hơn (suy luận tăng 10% đến 22% ở một số
  model). Phần "tổng quan repo" **không** giúp agent chạm vào file cần sửa sớm hơn. 100% file do Sonnet 4.5 sinh có phần tổng
  quan; 8 trên 12 file của lập trình viên cũng có.
- Khi xóa hết tài liệu khác khỏi repo, file LLM sinh lại giúp +2,7% và vượt file của người: file LLM sinh phần lớn **trùng**
  tài liệu sẵn có.
- Kết luận của tác giả: bỏ file ngữ cảnh do LLM sinh; file ngữ cảnh hữu ích để nêu **thực hành không chuẩn**; mọi cải tiến
  phải được đo trước khi triển khai. Lưu ý: abstract v1 viết "có xu hướng giảm tỉ lệ thành công", v3 viết "nhìn chung không
  cải thiện"; tôi dùng v3 vì thận trọng hơn.
- Hạn chế: chỉ Python, tác vụ sửa lỗi/tính năng trong repo đã có tài liệu; chưa kiểm cho dự án dựng từ đầu.
- Áp dụng: **đây là bằng chứng mạnh nhất chống lại thiết kế hiện tại** (mục 5.1), đồng thời là căn cứ cho luật "không tổng quan".

**C2. On the Impact of AGENTS.md Files on the Efficiency of AI Coding Agents.** Lulla và cộng sự. arXiv:2601.20404; v1 2026-01-28, v2 2026-03-30. https://arxiv.org/abs/2601.20404 (chỉ đọc abstract).
- 10 repo, 124 pull request, chạy agent có và không có AGENTS.md. Có file thì thời gian chạy trung vị thấp hơn 28,64% và
  token đầu ra thấp hơn 16,58%, mức hoàn thành tương đương. Bài ngắn (5 trang); là liên hệ, không phải thí nghiệm có đối chứng
  chặt về loại nội dung.
- Áp dụng: bằng chứng ngược chiều với C1 về chi phí. Hai bài khác phương pháp và nhóm agent (theo C3). Hòa giải ở mục 4.

**C3. Do Context Files Help Coding Agents? A Two-Agent Ablation Study on Real Repositories.** arXiv:2607.27250v1 (tháng 7/2026 theo mã). https://arxiv.org/html/2607.27250v1. Preprint, chưa thấy dấu hiệu bình duyệt.
- Hai agent (Claude Code, Codex), 17 tác vụ từ 3 repo, 288 lượt chạy, chấm bằng test ẩn. So ba chiến lược: không có, AGENTS.md
  luôn nạp, tài liệu theo chủ đề truy xuất khi cần. Tác động lên tỉ lệ đúng bị chặn trong ±10 điểm phần trăm (Claude) và ±15
  (Codex) bằng kiểm định tương đương: **không thấy khác biệt đo được**.
- Phân tích lỗi: agent hỏng vì kỹ năng cài đặt (thiết kế, chọn mẫu, nối dây), **không phải thiếu tri thức repo** mà file ngữ cảnh
  có thể cung cấp. Tín hiệu duy nhất sống sót: ở repo mà AGENTS.md có cảnh báo cụ thể về thời gian chạy ("bộ test đầy đủ mất
  hơn 20 phút"), Claude giảm khoảng 24% thời gian và số lần chạy cả bộ test "mù" giảm theo liều (3,67 → 2,44 → 1,67).
- Về sức mạnh thống kê: phát hiện chênh 10 điểm với độ tin 80% cần khoảng **120 đến 200 tác vụ**; thêm lần lặp gần như không giúp.
  Độ khó "biên" phụ thuộc agent (tương quan Spearman 0,75), có thể giải thích vì sao các nghiên cứu trước mâu thuẫn.
- Áp dụng: giá trị thật của file ngữ cảnh là **sự kiện vận hành không suy ra được** (thời gian, thứ tự, môi trường). Cỡ mẫu
  của chính bài này nhỏ nên không dùng làm bằng chứng tuyệt đối; dùng làm hướng và làm cảnh báo về cách đo.

**C4. Agent READMEs: An Empirical Study of Context Files for Agentic Coding.** Chatlatanagulchai và cộng sự. arXiv:2511.12884; v1 2025-11-17, v2 2026-08-09. https://arxiv.org/abs/2511.12884 (chỉ đọc abstract).
- 2.303 file từ 1.925 repo: file tiến hóa như mã cấu hình, qua nhiều lần thêm nhỏ, khó đọc. Nội dung phổ biến: quy trình test
  75,9%, chi tiết cài đặt 70,8%, kiến trúc 68,1%; ràng buộc phi chức năng hiếm: bảo mật 14,8%, hiệu năng 14,5%.
- Áp dụng: file ngữ cảnh thật thường **phình dần** và nghiêng về mô tả; bảo mật và hiệu năng gần như bị bỏ trống. Gợi ý một
  luật tăng trưởng có kiểm soát (mục 9 spec) và mục "Rules" có `Enforced by` cho bảo mật.

**C5. How Many Instructions Can LLMs Follow at Once? (IFScale).** Jaroslawicz và cộng sự. arXiv:2507.11538, 2025-07-15. https://arxiv.org/abs/2507.11538 (chỉ đọc abstract).
- Benchmark 500 chỉ dẫn "chèn từ khóa" cho một tác vụ viết báo cáo, 20 model: model tốt nhất chỉ đạt 68% ở mật độ 500 chỉ dẫn;
  có ba kiểu suy giảm và thiên lệch về các chỉ dẫn đứng trước.
- **Lưu ý**: con số "150 đến 200 chỉ dẫn" được nhiều bài cộng đồng (D2, D5) trích như kết quả nghiên cứu. Tôi **không tìm
  thấy con số đó trong abstract** này, và bài này đo tác vụ viết từ khóa, không phải lập trình. Coi đó là ước lượng thực hành,
  không phải số liệu.

**C6. Context Rot: How Increasing Input Tokens Impacts LLM Performance.** Chroma (công ty cơ sở dữ liệu vector). https://www.trychroma.com/research/context-rot (đọc phần mở đầu; truy cập qua chuyển hướng từ research.trychroma.com).
- 18 model; giữ độ khó tác vụ không đổi, chỉ tăng độ dài đầu vào: hiệu suất giảm không đều khi đầu vào dài hơn, kể cả với
  tác vụ đơn giản. Nghiên cứu của nhà cung cấp, dữ liệu mở; không phải tác vụ lập trình.
- Áp dụng: cùng hướng với A8; hỗ trợ việc giữ phần nạp trước nhỏ.

**C7. SWE-agent: Agent-Computer Interfaces Enable Automated Software Engineering.** Yang và cộng sự. arXiv:2405.15793, 2024-05. https://arxiv.org/abs/2405.15793 (chỉ đọc abstract).
- Thiết kế **giao diện agent-máy tính** (cách sửa file, điều hướng repo, chạy test) làm tăng đáng kể hiệu suất so với shell trần.
  Số đo cụ thể thuộc thế hệ model 2024 nên chỉ dùng định tính.
- Áp dụng: phản hồi của lệnh kiểm tra (thông báo lỗi có hướng dẫn sửa) là một phần của harness, cùng ý với D1.

**C8. On Randomness in Agentic Evals.** Bjarnason, Silva, Monperrus (KTH). arXiv:2602.07150, workshop ICLR 2026. https://arxiv.org/pdf/2602.07150v2 (**chỉ đọc đoạn trích** trong kết quả tìm kiếm).
- 60.000 quỹ đạo trên SWE-Bench-Verified: pass@1 của một lượt chạy đơn dao động 2,2 đến 6,0 điểm tùy lượt, độ lệch chuẩn
  trên 1,5 điểm kể cả ở nhiệt độ 0; cải thiện 2 đến 3 điểm có thể chỉ là nhiễu.
- Áp dụng: một lần chạy smoke trên một dự án không thể "chứng minh" gì thống kê; dùng nhiều lượt và cỡ mẫu theo tính toán.

**C9. Coding Agents Have Converged: Why the SWE-bench Leaderboard Can No Longer Order Its Top Entries.** arXiv:2609.17394, 2026-09-15. https://arxiv.org/abs/2609.17394 (chỉ đọc abstract).
- Phân tích 254 bản nộp: với cùng một model, khoảng dao động theo scaffold lên tới 29,8 điểm (quan sát, không kết luận nhân quả),
  trong khi top 30 bảng xếp hạng chỉ chênh 8,8 điểm.
- Áp dụng: phần "ngoài model" (scaffold, harness) có thể lớn hơn chênh lệch giữa các model; nhưng "scaffold" ở đây là phần mềm
  agent, không phải riêng file AGENTS.md. Không suy ra rằng AGENTS.md đáng giá 30 điểm.

### 2.5 Thực hành và ý kiến (Thực hành / Ý kiến)

**D1. Harness engineering: leveraging Codex in an agent-first world.** Ryan Lopopolo (OpenAI), 2026-02-11. Bản gốc https://openai.com/index/harness-engineering/ **trả HTTP 403**; tôi đọc toàn văn qua bản sao trong kho GitHub https://github.com/celesteanders/harness/blob/main/docs/research/260211_openai_harness_engineering_codex.md. Thực hành của một đội, không có đối chứng.
- Cách "một AGENTS.md khổng lồ" đã thất bại: chiếm ngữ cảnh, "quá nhiều hướng dẫn thành không có hướng dẫn", mục nát ngay,
  khó kiểm bằng máy. Thay bằng AGENTS.md khoảng 100 dòng làm **mục lục** trỏ tới `docs/` coi là hệ thống ghi nhận (system of record).
- Cưỡng chế bằng **linter tùy biến và test cấu trúc**, viết thông báo lỗi sao cho **đưa chỉ dẫn sửa vào ngữ cảnh agent**;
  "khi tài liệu chưa đủ, nâng quy tắc lên thành mã". Lớp kiến trúc có hướng phụ thuộc kiểm bằng máy.
- Agent "đọc được" ứng dụng: khởi chạy theo từng git worktree, điều khiển trình duyệt, đọc log và metric. Agent "dọn rác" định
  kỳ (doc-gardening) tìm tài liệu lỗi thời và mở PR sửa.
- Tác giả tự nêu: hành vi này phụ thuộc cấu trúc và công cụ riêng của repo, không nên coi là khái quát hóa.
- Áp dụng: lấy (a) mục lục thay vì bách khoa, (b) thông báo lỗi của kiểm tra có hướng sửa, (c) nâng luật văn xuôi lên mã,
  (d) làm mới theo dõi kiểu doc-gardening. Không lấy: cây `docs/` đầy đủ (design-docs, exec-plans, quality score), vì quá nặng
  cho mọi dự án, và không có đo đối chứng.

**D2. Writing a good CLAUDE.md.** HumanLayer (blog), không ghi ngày. https://www.humanlayer.dev/blog/writing-a-good-claude-md. Thực hành/ý kiến.
- Ít chỉ dẫn hơn thì tốt hơn; file gốc của họ dưới 60 dòng. Progressive disclosure bằng thư mục tài liệu theo chủ đề. **Ưu tiên con
  trỏ hơn bản sao**; tránh đoạn mã dễ lỗi thời, dùng tham chiếu file:dòng. Không dùng LLM làm linter; dùng linter/formatter và
  hook `Stop`. Khuyên không tự sinh CLAUDE.md bằng `/init` vì đây là điểm đòn bẩy cao.
- Khẳng định (chưa tôi kiểm chứng) rằng Claude Code bọc nội dung CLAUDE.md trong lời nhắc "có thể không liên quan"; vì vậy chỉ dẫn không áp
  dụng chung dễ bị bỏ qua. Dựa trên việc nghe lưu lượng của tác giả ở một phiên bản cụ thể; có thể đã đổi.
- Con số "150 đến 200 chỉ dẫn": xem C5 (không xác thực).

**D3. Understanding Spec-Driven-Development: Kiro, spec-kit, and Tessl.** Trang martinfowler.com (tác giả không hiện trong bản tải), khoảng 2025-10. https://martinfowler.com/articles/exploring-gen-ai/sdd-3-tools.html. Ý kiến có quan sát thực.
- Phân ba mức: spec-first, spec-anchored, spec-as-source. Phân biệt **spec** (gắn một thay đổi) với **memory bank** (quy tắc xuyên suốt).
  Cảnh báo: spec dài gây quá tải khi review; một quy trình không vừa mọi cỡ việc (sửa lỗi nhỏ ra 4 user story, 16 tiêu chí); "cảm
  giác kiểm soát giả" vì agent vẫn bỏ qua hoặc làm quá chỉ dẫn; MDD trước đây thất bại vì chi phí và ràng buộc.
- Áp dụng: harness là memory bank, nhỏ; spec của dự án nằm ở artifact planning; không thêm vòng file spec cho việc harness.

**D4. awesome-claude-code.** https://github.com/hesreallyhim/awesome-claude-code. Danh mục cộng đồng. Trang chỉ trả danh sách file;
chỉ có đoạn trích tìm kiếm cho các mục (ví dụ công cụ biến luật mềm thành hook cưỡng chế). **Chỉ đọc đoạn trích**; tin cậy thấp,
không dùng làm căn cứ.

**D5. Các bài cộng đồng về "ngân sách chỉ dẫn"** (DataCamp, tianpan.co, các bài dev.to và gist). **Chỉ đọc đoạn trích.** Phần
lớn trích lại nhau về "150 đến 200 chỉ dẫn" và "system prompt của Claude Code dùng khoảng 50". Xem C5: không xác thực.

**D6. GitHub Spec Kit.** https://github.com/github/spec-kit, https://github.github.com/spec-kit/. **Chỉ đọc đoạn trích**; cách mô tả của D3 đã đủ cho thiết kế này.

## 3. Nguồn không tải được hoặc chỉ đọc một phần

| Nguồn | Chuyện gì xảy ra | Cách xử lý |
| --- | --- | --- |
| openai.com/index/harness-engineering | HTTP 403 | Đọc toàn văn qua bản sao GitHub (D1). Ghi rõ là bản sao; không thể xác nhận từng chữ với bản gốc. |
| code.claude.com/docs/en/skills (không có `.md`) | Không khớp cụm tìm (trang trả rỗng) | Dùng URL có đuôi `.md` (A3). |
| arxiv.org/pdf/2602.11988 | Loại nội dung PDF không hỗ trợ | Dùng bản HTML v3 (C1). |
| developers.openai.com/codex/guides/agents-md | Chuyển hướng sang origin khác | Đọc ở learn.chatgpt.com (B2). |
| research.trychroma.com/context-rot | Chuyển hướng sang www.trychroma.com | Đọc ở địa chỉ mới (C6); chỉ đọc phần mở đầu. |
| C2, C4, C5, C7, C9 | Chỉ lấy được trang abstract | Chỉ dùng điều abstract nói; không suy ra phương pháp chi tiết. |
| C8, D4, D5, D6 | Chỉ có đoạn trích tìm kiếm | Nhãn "chỉ đọc đoạn trích", không dùng làm căn cứ chính. |
| Anthropic engineering posts | Bản tải không có ngày xuất bản | Ghi "không hiện ngày". |

Chưa đọc: tài liệu `hooks.md` đầy đủ và `settings.md` (chỉ đọc hooks-guide và permissions có chọn lọc); các bài so sánh benchmark
của bên thứ ba (digitalapplied và tương tự), vì chỉ có đoạn trích và độ tin thấp.

## 4. Số liệu thật và ý kiến

| Điều được nói | Loại | Nguồn | Ghi chú |
| --- | --- | --- | --- |
| File ngữ cảnh không cải thiện tỉ lệ thành công (−0,5% và −2% với file LLM sinh; +2,4% với file người viết, đều không ý nghĩa) | Số liệu | C1 | Python; SWE-bench Lite và CTXbench |
| File ngữ cảnh làm chi phí tăng khoảng 20 đến 23% (LLM sinh), tới 19% (người viết) | Số liệu | C1 | Có ý nghĩa thống kê; nguyên nhân là agent chạy thêm test và khám phá |
| Tổng quan repo không giúp tìm file cần sửa nhanh hơn | Số liệu | C1 | |
| Có AGENTS.md thì thời gian chạy trung vị thấp hơn 28,64% | Số liệu (nhỏ) | C2 | 10 repo, 124 PR, bài 5 trang |
| Khác biệt đúng/sai giữa các chiến lược nạp ngữ cảnh nằm trong ±10 đến 15 điểm | Số liệu (nhỏ) | C3 | 17 tác vụ; công suất thấp |
| Cảnh báo thời gian chạy cụ thể giảm khoảng 24% thời gian ở một repo | Số liệu (rất nhỏ) | C3 | 1 repo; cơ chế hợp lý, chưa tái lập |
| Phát hiện chênh 10 điểm cần 120 đến 200 tác vụ | Số liệu (mô phỏng) | C3 | |
| Pass@1 một lượt dao động 2,2 đến 6,0 điểm | Số liệu | C8 | Chỉ đọc đoạn trích |
| Hiệu suất giảm khi ngữ cảnh dài | Số liệu | C6, A8 | Không phải tác vụ lập trình |
| Model tốt nhất chỉ đạt 68% ở 500 chỉ dẫn | Số liệu | C5 | Tác vụ từ khóa |
| "Frontier model theo được 150 đến 200 chỉ dẫn" | **Ý kiến** | D2, D5 | Không xác thực (C5) |
| "Claude thường bỏ qua CLAUDE.md không áp dụng chung" | **Ý kiến** (quan sát cá nhân) | D2 | Phụ thuộc phiên bản |
| "Một AGENTS.md khổng lồ thất bại, mục lục thì chạy" | **Thực hành** | D1 | Không đối chứng |
| "Dưới 200 dòng mỗi file" | **Chính thức** (khuyến nghị) | A1 | Là trần khuyến nghị, không đo tối ưu |
| "Cho agent phép kiểm tra để chạy là đòn bẩy lớn nhất" | **Chính thức** (khuyến nghị) | A2, A9 | A9 có kết quả định tính |

## 5. Các mâu thuẫn và cách hòa giải

1. **C1 (file ngữ cảnh không giúp, tốn thêm 20%) và C2 (giảm 28,64% thời gian).** Khác phương pháp: C2 so có/không file, cỡ nhỏ,
   không tách loại nội dung; C1 có đối chứng nhiều model và tách LLM sinh/người viết. C3 gợi ý lời giải: ở chỗ file chứa **sự
   kiện vận hành cụ thể** (thời gian, thứ tự, môi trường) thì giảm chạy mù; file chứa mô tả chung thì tăng khám phá mà không tăng
   độ đúng. Kết luận dùng: nội dung quyết định kết quả, vì vậy chỉ giữ loại nội dung có cơ chế.
2. **A12 (đưa bản đồ thư mục vào CLAUDE.md) và C1 (tổng quan vô ích) và A2 (chỉ ghi điều không suy ra được).** Tài liệu A2 mới
   hơn đã đổi hướng, C1 có số đo. Theo A2 và C1. Ngoại lệ có căn cứ: nếu repo **không có tài liệu nào** thì tổng quan LLM sinh
   hữu ích (+2,7% trong C1 khi xóa hết tài liệu). Vì vậy luật có một ngoại lệ hẹp cho repo trống.
3. **D1 (mục lục ~100 dòng) và D2 (dưới 60 dòng) và A1 (dưới 200 dòng).** Cả ba cùng nguyên tắc, khác con số, không nguồn nào đo
   điểm tối ưu. Ta chọn mục tiêu thấp (80 dòng), ngưỡng cảnh báo giữa (120), trần chặn theo giới hạn công cụ (200 dòng, 24 KB), và
   coi tất cả là **tạm thời cho tới khi đo** (mục 10 của spec).
4. **A9 (file tiến độ, danh sách tính năng JSON) và việc SDLC Squad đã có issue, comment, git.** Không thêm file tiến độ; chỉ
   lấy phần "chạy kiểm tra cơ bản trước khi làm" và "để lại trạng thái sạch".
5. **D1 (nâng luật lên mã) và thực tế nhiều runtime.** Hook `Stop` chỉ có ở Claude Code. Cơ chế chung duy nhất chạy ở mọi runtime
   là lệnh `Done` + CI + git. Hook là bộ chuyển tùy chọn.

## 6. Rút ra

| # | Điều rút ra | Căn cứ chính |
| --- | --- | --- |
| F1 | Không có bằng chứng mạnh rằng file ngữ cảnh làm tăng tỉ lệ thành công; có bằng chứng rằng nó làm tăng chi phí. Coi harness là giả thuyết cần đo, giữ nhỏ. | C1, C3 |
| F2 | Nội dung có cơ chế: lệnh xác nhận, sự kiện vận hành không suy ra được, chỗ code gây hiểu nhầm, thực hành không chuẩn. Nội dung vô ích: tổng quan, thứ trùng tài liệu. | C1, C3, A2 |
| F3 | Chỉ dẫn là ngữ cảnh, không phải cưỡng chế. Luật không ngoại lệ phải nằm ở mã, CI hoặc hook. Linter chứ không phải LLM làm việc của linter. | A1, A2, A5, D2, D1 |
| F4 | Vòng kiểm chứng (lệnh chạy được, đòi bằng chứng) là đòn bẩy lớn nhất; agent hay báo xong mà chưa kiểm tra đầu-cuối. | A2, A9 |
| F5 | Ngữ cảnh nạp trước phải nhỏ; import không giảm chi phí; ngưỡng cụ thể chưa có số đo. | A1, A8, C6 |
| F6 | Progressive disclosure bằng con trỏ có điều kiện chạy ở mọi công cụ; nạp có điều kiện bằng cấu hình thì không đồng nhất. | A3, A8, A11, B1 đến B5, D2 |
| F7 | Chạy dài cần: script dựng môi trường, kiểm tra cơ bản đầu phiên, một việc mỗi phiên, trạng thái sạch. SDLC đã có phần lớn bằng issue và git. | A9 |
| F8 | File ngữ cảnh mục nát và phình dần; cần kiểm tra tồn tại/mâu thuẫn bằng máy và luật tăng trưởng/cắt tỉa. | A1, C4, D1 |
| F9 | Đo hiệu quả rất nhiễu: một lượt dao động vài điểm, phát hiện 10 điểm cần hàng trăm tác vụ. Dùng tín hiệu theo nhóm dự án và một bộ eval nhỏ lấy từ lỗi thật, nhiều lượt. | C3, C8, A10 |
| F10 | Một AGENTS.md ở gốc là mẫu số chung của Codex, Cursor, Copilot, Kiro; Claude Code cần `CLAUDE.md` import. Trần cứng đã biết: 32 KiB ở Codex. | B1 đến B5, A1 |
| F11 | Quy trình spec nặng gây quá tải review và không vừa mọi cỡ việc. Harness không được thêm vòng spec. | D3 |
| F12 | File do người viết ổn hơn file do LLM sinh. Agent Architect là LLM, nên nội dung của nó phải dựa trên **thứ nó đã chạy và đã vấp**, không phải thứ nó mô tả lại. | C1, D2 |

## 7. Các phương án

Năm phương án. A là thiết kế hiện tại. B là phương án chọn.

### 7.1 Mô tả

**A. Hiện tại** ([sdlc-harness.md](sdlc-harness.md)). Stage `Harness` sau `Environment`. Architect (LLM) viết `AGENTS.md` từ mẫu
tối đa 150 dòng: định hướng 3 đến 4 câu, mục Policy, bảng "Where things are" có cột `Forbidden` và `Enforced by`, mục Running
and verifying (Ensure, Quick, `Done = ...` kèm mức bằng chứng), Conventions, Known pitfalls. 13 kiểm tra văn bản H1 đến H13, trong đó 8
chặn; smoke do Developer; làm mới theo trigger.

**B. Hợp đồng trước, nội dung tối thiểu, tăng theo thất bại** (chọn). Ba ý chính:
1. *Hợp đồng bằng mã*: hai lệnh chạy được và được chứng minh bằng thực thi (Ensure, Done) là phần cốt lõi; AGENTS.md chỉ trỏ tới.
2. *AGENTS.md tối thiểu*: chỉ Rules (có `Enforced by`), Gotchas (có bằng chứng), con trỏ có điều kiện; không tổng quan, không trạng thái,
   không chép lệnh. Mục tiêu 80 dòng.
3. *Tăng theo thất bại và đo được*: dòng mới chỉ vào khi agent vấp hai lần hoặc Architect vấp lúc dựng; luật vi phạm hai lần được
   nâng thành lint, test hoặc CI; chỉ số lấy từ dữ liệu có sẵn của nền tảng.
Kiểm tra chia ba mức: `gate` (chỉ khi harness vắng hoặc nói dối thì chặn Developer), `cert` (điều kiện chứng nhận stage), `warning`.

**C. Kho tri thức trong repo** (kiểu D1). AGENTS.md khoảng 100 dòng làm mục lục; thư mục `docs/` có cấu trúc (design-docs, exec-plans,
product-specs, references, QUALITY_SCORE); linter tùy biến và test cấu trúc; agent doc-gardening định kỳ.

**D. Khởi tạo và tiến độ cho chạy dài** (kiểu A9). Agent khởi tạo dựng `init.sh`, file tiến độ, danh sách tính năng JSON `passes`,
commit đầu; mỗi phiên làm một tính năng, chạy kiểm tra cơ bản trước khi làm, để lại trạng thái sạch.

**E. Không có file ngữ cảnh** (đường cơ sở). Agent tự khám phá; CI là cổng. Dùng như nhánh đối chứng của đo đạc.

### 7.2 So sánh

| Tiêu chí | A. Hiện tại | B. Hợp đồng trước (chọn) | C. Kho tri thức | D. Khởi tạo và tiến độ | E. Không file |
| --- | --- | --- | --- | --- | --- |
| Khớp bằng chứng định lượng (C1, C3) | **Thấp**: có tổng quan (C1 nói vô ích), do LLM sinh, tới 150 dòng | **Cao**: chỉ sự kiện vận hành và lệnh kiểm chứng, gọn | Trung bình: mục lục tốt, cây `docs/` chưa có đo | Trung bình: không bàn tới file ngữ cảnh | Cao về chi phí, thấp về sự kiện vận hành (C3) |
| Khớp hướng dẫn chính thức (A1 đến A3, A8, A9) | Trung bình: trong 200 dòng nhưng chứa phần suy ra được | **Cao** | Cao | **Cao** (A9 là nguồn) | Thấp: bỏ qua lệnh kiểm chứng (A2) |
| Chi phí ngữ cảnh mỗi lượt chạy (ước tính 4 ký tự/token) | Tới ~150 dòng, khoảng 2,5k đến 4k token; ngân sách "must-read" 60 KB cho phép khoảng 15k | 80 dòng, khoảng 1,2k đến 2k token | ~100 dòng + đọc tài liệu theo nhu cầu (5k đến 20k nếu đọc) | Mỗi phiên đọc file tiến độ và danh sách tính năng, **tăng theo số tính năng** | 0 |
| Kiểm chứng bằng máy | Văn bản (heuristic): bắt được đường dẫn/lệnh sai, không bắt được "Done vô nghĩa" | **Văn bản cộng thực thi** Ensure và Done | Cao (linter, test cấu trúc), nhưng chi phí dựng lớn | Trung bình (`passes` do agent tự đặt) | Chỉ CI |
| Chống "báo xong khi chưa xong" (A9) | Thấp: chỉ kiểm dòng Done có tồn tại | **Cao**: Done chạy thật; smoke có bước làm hỏng cố ý | Trung bình | Cao (kiểm tra cơ bản mỗi phiên) | Thấp |
| Chi phí dựng (lượt chạy agent mỗi dự án, ước tính) | 3 đến 6 (Harness, smoke) | 3 đến 6; cộng 1 lần chạy `--run` | 15 đến 40 (cấu trúc, linter, tài liệu) | 3 đến 8; file tiến độ phải duy trì | 0 |
| Chạy giống nhau ở mọi runtime | Có (AGENTS.md) | **Có**: Done, CI, git là cơ chế chung; hook là tùy chọn | Có | Có | Có |
| Rủi ro chính | Chặn Developer vì kiểm tra sai (xem 8.1); mục nát; Architect LLM viết tổng quan | Kiểm tra thực thi chạy lệnh của repo; ngưỡng tạm thời; chưa có đo đối chứng | Quá nặng cho dự án nhỏ; không có đo đối chứng; trùng lặp tài liệu | File tiến độ trùng issue/comment; JSON phình | Mất sự kiện vận hành (ví dụ môi trường, thứ tự lệnh) |
| Phù hợp "mọi dự án được giao" | Trung bình | **Cao** (nhỏ, chạy được ở mọi cỡ) | Thấp ở dự án nhỏ | Trung bình | Thấp |

### 7.3 Chọn phương án B, vì sao

1. **Bằng chứng mạnh nhất** (C1) nói file ngữ cảnh nhìn chung không tăng tỉ lệ thành công và tăng chi phí; điểm sáng duy nhất là chỉ dẫn
   cụ thể và thực hành không chuẩn (C1, C3). B là phương án duy nhất bỏ phần vô ích (tổng quan) và giữ phần có cơ chế.
2. **Vấn đề của dự án có agent code nhiều vòng prompt** thường là "agent không biết chạy gì để kiểm tra" và "báo xong sai" (A2, A9),
   không phải "agent không biết cấu trúc repo" (C3: lỗi do kỹ năng cài đặt, không do thiếu tri thức repo). B nhắm đúng chỗ đó bằng
   Ensure, Done và bằng chứng thực thi.
3. **Chạy được ở mọi runtime** nhờ chỉ dùng AGENTS.md + lệnh + CI + git; hook chỉ là bộ chuyển (F10, D1).
4. **Đo được.** B kèm chỉ số từ dữ liệu có sẵn và quy tắc quyết định đặt trước; A thì chỉ có ngưỡng smoke tự đặt.
5. **Mượn có chọn lọc**: từ D lấy Ensure/Done làm `init.sh` và "kiểm tra cơ bản trước khi làm"; từ C lấy mục lục, thông báo kiểm tra có
   hướng sửa, nâng luật lên mã, làm mới kiểu doc-gardening. Không lấy file tiến độ (đã có issue, comment, git) và cây `docs/` nặng.

Phương án bị loại:
- **A nguyên trạng**: mục 8.1.
- **C đầy đủ**: chi phí dựng cao cho từng dự án, không có đo đối chứng, trùng lặp với artifact planning (architecture.md và epics).
- **D đầy đủ**: file tiến độ và danh sách tính năng trùng với issue và comment của nền tảng; JSON tăng theo số tính năng làm tăng
  ngữ cảnh mỗi phiên.
- **E**: giữ làm nhánh đối chứng đo đạc, không phải mặc định, vì C3 cho thấy sự kiện vận hành cụ thể có tác dụng thật và vì A2 coi lệnh
  kiểm chứng là đòn bẩy lớn nhất.

### 7.4 Những điểm còn bất định

1. **Hiệu quả của chính harness chưa được chứng minh** cho dự án dựng từ đầu. C1 đo trên Python, repo đã có tài liệu; C3 nhỏ. Có thể
   harness tối thiểu cũng không tăng tỉ lệ thành công; khi đó giá trị chỉ còn ở chi phí và ở việc chặn "báo xong sai".
2. **Ngưỡng kích thước** (80, 120, 200 dòng; 8, 24 KB) là tạm thời; không nguồn nào đo điểm tối ưu.
3. **Hook `Stop` và hook git trên runtime của daemon** chưa kiểm chứng; chưa rõ daemon chạy agent qua CLI nào ở từng dự án.
4. **Kiểm tra thực thi** (`--run`) chạy lệnh có trong repo; cần giới hạn thời gian và chỉ Architect dùng. Tác dụng phụ của lệnh Ensure
   ở môi trường dùng chung chưa kiểm.
5. **Đo theo nhóm dự án** không cho kết luận nhân quả khi cỡ mẫu nhỏ (C3, C8); cần bộ eval ngoại tuyến (spec mục 10.4) mới trả lời được.
6. Một số nguồn chỉ đọc đoạn trích hoặc abstract (mục 3).

## 8. Đánh giá thẳng thắn thiết kế hiện tại

### 8.1 Bằng chứng nội bộ: kiểm tra của ta không chấp nhận harness của multica

Tôi chạy `node packages/core/squads/sdlc-skills/sdlc-harness/scripts/check-harness.mjs --root .` (chỉ đọc) trên repo multica ngày
2026-11-12, đúng repo mà mục tiêu nói là "đạt mức như". Kết quả: **5 trong 8 kiểm tra chặn fail**.

| Kiểm tra | Kết quả trên multica | Vì sao |
| --- | --- | --- |
| H1 | fail | `AGENTS.md` 13.814 byte, giới hạn 12.288 |
| H2 | fail | không có dòng `Verified <date> against <sha>` |
| H3, H4, H7 | pass | đường dẫn, lệnh, và không có từ ngữ trạng thái |
| H6 | fail | không có dòng `Done = ...` |
| H8 | fail | không có bảng ranh giới với cột `Enforced by` |
| H10 | fail | không có `docs/sdlc/environment.md` |

Điểm quan trọng: `AGENTS.md` của multica dài 130 dòng, có bản đồ phạm vi, bảng ranh giới gói, bảng lệnh xác minh, và nhiều luật có
lý do. Nó là harness tốt và chỉ thiếu những thứ **do mẫu của ta đặt ra**. Hệ quả: các kiểm tra chặn đo **mức tuân thủ mẫu của ta**,
chưa phải **hiệu quả**. Vì Developer chạy `check-harness --quick` trước mỗi issue đổi mã và `block tooling` khi có kiểm tra chặn fail
(sdlc-development, sdlc-delivery-core), một kiểm tra chặn sai nằm trên đường găng của mọi issue. Đây chính là loại cổng tự đặt mà
[sdlc-autonomy.md](sdlc-autonomy.md) mục 1 đã rút ra bài học.

Hai điều cảnh báo hiệu chỉnh: (a) tôi chỉ thử trên một repo; cần thêm vài repo tốt khác để biết tỉ lệ chặn sai (bước S0 của plan);
(b) H11 báo "a real .env file is tracked" vì repo commit `apps/mobile/.env.production` và `.env.staging`; có thể là cấu hình công khai
(Expo) nên đây là cảnh báo nhiễu chứ chưa chắc là lỗi, cần nhìn nội dung trước khi kết luận.

### 8.2 Giữ

| Giữ | Vì sao |
| --- | --- |
| Stage `Harness` sau `Environment`, Architect sở hữu, không đổi server | Vị trí hợp lý; thay đổi nhỏ nhất; khớp với "do the simplest thing" của A8 |
| `CLAUDE.md` chỉ chứa `@AGENTS.md` | Đúng với A1: có cả hai file thì Claude chỉ đọc CLAUDE.md |
| H3, H4 (đường dẫn và lệnh có thật) chặn | Khách quan, rẻ, bắt đúng lỗi "trỏ tới file không còn" mà A1 gọi tên; chỉ dẫn sai tệ hơn không có |
| Con trỏ thay cho bản sao, "ledger rule" khi gỡ chỉ dẫn | Khớp D2, D1, A2 |
| Ensure command và Done command | Chính là `init.sh` và phép kiểm tra của A9, A2 |
| Hạ bậc có ghi chép thay vì dừng | Bài học của sdlc-autonomy; giữ |
| Trigger làm mới (retrospective, manifest đổi, ruling lặp) | Hợp lý, không cần đồng hồ; khớp ý doc-gardening của D1 |
| Chạy thuần Node, không phụ thuộc, in JSON | Dễ nhúng vào skill; test được |

### 8.3 Đổi hoặc bỏ

| # | Hiện tại | Vấn đề (bằng chứng) | Thay bằng |
| --- | --- | --- | --- |
| 1 | Mẫu AGENTS.md có "định hướng 3 đến 4 câu" và bảng "Where things are" | Là tổng quan repo: C1 nói không giúp, có thể trùng tài liệu, tốn thêm bước. | Bỏ tổng quan; chỉ Rules, Gotchas, con trỏ có điều kiện, hai lệnh. Ngoại lệ hẹp cho repo trống. |
| 2 | Architect LLM viết từ mẫu | C1: file LLM sinh kém file người viết và tăng chi phí 20%+. | Nội dung chỉ từ thứ Architect **đã chạy và đã vấp**, mỗi dòng có bằng chứng. |
| 3 | Tám kiểm tra chặn, trong đó có H2, H8, H1 theo byte | Chặn cả harness tốt của multica (8.1); H8 ép bảng không phải repo nào cũng cần. | Ba mức: `gate` (H1, H3, H4), `cert` (H6, H7, H10, H14, E1, E2), `warning` (còn lại). Developer chỉ bị `gate` chặn. |
| 4 | Kiểm tra Done chỉ nhìn văn bản; H4 và H6 chỉ phân giải lệnh `npm`, `pnpm`, `yarn`, `make` (`judgeCommand` trong `check-harness.mjs`) | ``Done = `echo ok` `` qua H6, vì `judgeCommand` không phân giải lệnh khác; với repo Go, Python hay Rust (`go test`, `pytest`, `cargo test`) H4 hầu như không kiểm gì. A2, A9: agent báo xong khi chưa kiểm tra. | Kiểm tra thực thi `--run` (Ensure và Done exit 0 trên cây sạch, không phụ thuộc ngôn ngữ) và bước smoke làm hỏng cố ý. |
| 5 | Ngưỡng 200 dòng và 12 KB là "giới hạn" | 200 là trần khuyến nghị của A1, không phải điểm tối ưu; chính multica vượt 12 KB. | Mục tiêu 80, cảnh báo 120 dòng hoặc 8 KB, chặn theo trần công cụ (200 dòng, 24 KB < 32 KiB của Codex). Gọi rõ là tạm thời. |
| 6 | Dòng `Verified <date> against <sha>` bắt buộc trong AGENTS.md | Thêm chỉ dẫn nhiễu vào mọi lượt chạy ở công cụ không gỡ chú thích; chỉ để phục vụ trigger làm mới. | Chuyển provenance sang `docs/sdlc/harness-cert.json` (đọc bằng máy). AGENTS.md không mang nó. |
| 7 | Bước 7 của `planning-architecture`: squad planning phải làm AGENTS.md qua kiểm tra | Planning squad chưa chạy lệnh nào, chỉ viết từ tài liệu (đúng loại C1 phê phán). | Planning squad chỉ để lại mục "non-default conventions" và "verification strategy" trong architecture.md; stage Harness của SDLC viết AGENTS.md. |
| 8 | Ngưỡng smoke (tối đa 2 run, 0 block...) là giả thuyết, không có đo thật | C8: một lượt không phân biệt nhiễu; không có cách biết harness có giúp không. | Giữ smoke như kiểm tra chức năng (đạt/trượt), thêm bước làm hỏng cố ý; thêm bộ chỉ số theo nhóm dự án và eval pack. |
| 9 | Phân biệt "luật" và "kiến thức repo" chưa rõ: mọi thứ cùng nằm trong AGENTS.md | Luật cần cưỡng chế và kiểm tra; kiến thức cần nạp theo nhu cầu; trộn lại thì file phình và mục nát (C4, D1). | Bốn loại nội dung với nơi cư trú riêng (spec mục 4). |
| 10 | `must-read` tối đa 60 KB (H13) | Khoảng 15k token cho phép đọc trước, ngược A8; không có căn cứ. | Cảnh báo ở 32 KiB (khớp trần Codex, B2). |
| 11 | Không đo | F9 | Chỉ số M1 đến M9 và quy tắc quyết định đặt trước (spec mục 10). |

Ghi chú về chi phí: C1 cho thấy file ngữ cảnh làm agent chạy thêm test và khám phá, tăng chi phí. Với ta, "chạy thêm kiểm tra"
vốn là hành vi mong muốn (A2). Chi phí phải được đo (M4), không giả định là miễn phí.

## 9. Kết luận

Chọn phương án B. Giữ khung của thiết kế hiện tại (stage, Architect, `@AGENTS.md`, H3/H4, Ensure/Done, trigger làm mới, không đổi
server). Đổi: bỏ tổng quan và bảng ranh giới khỏi mẫu, đưa nội dung về những gì có cơ chế, chuyển phần cốt lõi từ văn bản sang
thực thi, tách mức `gate`/`cert`/`warning`, chuyển provenance sang file chứng nhận JSON, thêm đo đạc và quy tắc quyết định đặt trước.
Chi tiết ở [sdlc-harness-spec.md](sdlc-harness-spec.md); các bước triển khai ở [sdlc-harness-plan.md](sdlc-harness-plan.md).
