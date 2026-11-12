# Research Lab: runbook

Cách dùng squad Research Lab và lệnh `multica research`. Thiết kế: [litscout-research-spec.md](litscout-research-spec.md). Kế hoạch và trạng thái triển khai: [litscout-research-plan.md](litscout-research-plan.md).

Ngôn ngữ: tiếng Việt. Lệnh, tên tệp, mã cảnh báo giữ nguyên tiếng Anh.

## 1. Chuẩn bị một lần

1. **Khóa niêm phong cho API key.** Đặt `MULTICA_RESEARCH_SECRET_KEY` (base64, 32 byte: `openssl rand -base64 32`) trong môi trường của server. **Nếu không đặt**, server vẫn chạy nhưng dẫn xuất khóa từ `JWT_SECRET` và ghi một dòng **WARN** lúc khởi động (`research: MULTICA_RESEARCH_SECRET_KEY is not set; sealing stored API keys with a key derived from JWT_SECRET...`). Hệ quả: đổi `JWT_SECRET` thì mọi key đã lưu không đọc được nữa và phải nhập lại; lúc đó `multica research` chạy với hạn mức không-key, không báo lỗi. Môi trường production nên đặt khóa riêng. Nếu `MULTICA_RESEARCH_SECRET_KEY` đặt sai định dạng, server ghi lỗi và không lưu được key (không âm thầm quay về `JWT_SECRET`).
2. **Nhập key trên web.** Settings > Research sources (chỉ owner và admin thấy tab):
   - OpenAlex API key. Từ 2026-02-13 OpenAlex tính hạn mức theo chi phí; không có key thì hạn mức ngày rất nhỏ. Xem [help.openalex.org/access/pricing](https://help.openalex.org/access/pricing/).
   - Semantic Scholar API key (tùy chọn; không có key thì dùng hạn mức chung thấp).
   - Contact email: gửi cho Crossref (polite pool) và Unpaywall (bắt buộc cho tra cứu toàn văn mở).
   Key chỉ ghi được, không bao giờ hiện lại. Ô nhập luôn trống; dán key mới để thay, bấm Remove để xóa.
3. **Ai đọc được key.** `GET /api/research/credentials` (nơi `multica research` lấy key) chỉ trả key cho: token tác vụ của agent (`mat_`, do daemon cấp, gắn với một workspace và một agent) và owner/admin của workspace. Thành viên thường nhận 403; `multica research` khi đó in một dòng cảnh báo ra stderr và dùng biến môi trường trên máy mình. Endpoint `GET /api/research/settings` (chỉ cho biết đã có key hay chưa) mở cho mọi thành viên; `PUT` chỉ cho owner/admin và không bao giờ cho agent.
   **Máy chạy agent (daemon).** Không cần cấu hình thêm: `multica research` tự lấy key của workspace từ server bằng token của agent. Biến môi trường `OPENALEX_API_KEY`, `SEMANTIC_SCHOLAR_API_KEY`, `MULTICA_RESEARCH_EMAIL` (hoặc `CROSSREF_MAILTO`, `UNPAYWALL_EMAIL`) trên máy đó được ưu tiên hơn cài đặt workspace. `--no-remote-credentials` tắt việc hỏi server.
4. **Tạo squad.** Squads > Quickstart > Research Lab. Tạo 5 agent (Research Lab Lead, Literature Scout, Relevance Screener, Evidence Synthesizer, Citation Auditor) và nhập 6 skill `research-lab-*`. Squad này tách khỏi Market Research, không dùng chung agent hay skill với nó.

## 2. Chạy một nghiên cứu bằng squad

Tạo một issue giao cho squad Research Lab. Nội dung: câu hỏi nghiên cứu, và nếu cần: năm bắt đầu, ngôn ngữ báo cáo, số bài tối thiểu/tối đa, số vòng tối đa, nguồn (`openalex`, `crossref`, `arxiv`, `semanticscholar`), có cho phép toàn văn mở hay không. Lead viết `protocol.md`, chạy `multica research init`, rồi giao việc theo sub-issue:

```text
Vòng r (r = 1..max_rounds):  Scout (search, dedup, prefilter)  ->  Screener (ingest-scores)  ->  Lead (gate)
   gate = refine_query        -> vòng r+1
   gate = summarize / summarize_available / stop_no_new_queries
Synthesizer (ingest-summaries)  ->  Auditor (doi-check, verify)  ->  Lead (export)
```

`state.json` đi giữa các agent dưới dạng tệp đính kèm trên comment của sub-issue (`multica issue comment add ... --attachment` và `multica attachment download`). Kết quả cuối: `papers.csv`, `research_digest.md`, `state.json` đính kèm vào comment báo cáo cuối của Lead.

Chi phí ước tính: `1 + 3 × số vòng + 3` lượt chạy agent (3 vòng là khoảng 13).

## 3. Chạy tay (không cần squad)

Dùng để thử, gỡ lỗi, hoặc đánh giá. Mọi lệnh in JSON; `--dir` mặc định là `research-run`.

```bash
multica research init --query "Vietnamese legal GraphRAG" --from-year 2020 \
  --min-papers 10 --max-papers 20 --max-rounds 3 --threshold 6 --language en \
  --sources openalex,crossref
multica research search --query "graph retrieval augmented generation legal question answering"
multica research dedup
multica research prefilter                       # ghi research-run/candidates.json
# ... một người hoặc agent chấm điểm, ghi scores.json:
multica research ingest-scores --file research-run/scores.json    # hoặc --heuristic
multica research gate                            # decision: refine_query | summarize | ...
# ... summaries.json (tóm tắt + claims có quote):
multica research ingest-summaries --file research-run/summaries.json
multica research doi-check
multica research verify --require-doi-check      # thoát != 0 nếu cổng provenance fail
multica research export --bom                    # papers.csv, research_digest.md
multica research eval --gold gold.json           # nếu có tập vàng do người gắn nhãn
```

Định dạng `scores.json`: `{"scores":[{"paper_index":0,"score":8,"reason":"..."}]}` (mỗi ứng viên đúng một dòng). `summaries.json`: `{"summaries":[{"paper_id":"p-...","summary":"..."}],"claims":[{"text":"...","paper_ids":["p-..."],"quote":"..."}]}`. Cả hai bị từ chối nguyên tệp nếu có bất kỳ lỗi nào, và thông báo liệt kê mọi lỗi.

### Lệnh tùy chọn

- `chase --top 5 --per-paper 10`: theo trích dẫn từ các bài top (OpenAlex `cites:` và `referenced_works`), tính một vòng.
- `init --allow-fulltext` rồi `fulltext`: tải PDF truy cập mở qua Unpaywall và trích văn bản vào `research-run/fulltext/<id>.txt`. Nếu máy có `pdftotext` (gói `poppler-utils`) thì dùng nó, vì đọc cột và khoảng cách từ tốt hơn nhiều; nếu không, dùng bộ đọc PDF thuần Go và từ chối kết quả không có khoảng cách giữa các từ (thường gặp với PDF dàn bằng TeX, `ErrWordSpacingLost`, cảnh báo `FULLTEXT_FAILED`). Cả hai đều mất cột đôi, bảng và công thức ở mức nào đó, nên văn bản này **không** được dùng làm bằng chứng cho tóm tắt và claim, và cảnh báo `FULLTEXT_LOW_CONFIDENCE` đi vào digest. Địa chỉ PDF đến từ API bên thứ ba nên kết nối tới địa chỉ riêng, loopback hoặc metadata bị chặn.

## 4. Quy tắc cứng của công cụ

- Metadata chỉ đến từ nguồn. Tệp của agent bị từ chối nếu có trường ngoài lược đồ, nên agent không có chỗ ghi đè tiêu đề, tác giả, DOI, URL, ngày hay abstract.
- Không có tóm tắt cho bài thiếu abstract; claim phải nêu `paper_ids` thuộc tập đã chọn và quote phải có trong abstract của một bài được trích.
- `search` từ chối vòng vượt `max_rounds` (tối đa 5). Query đã dùng bị bỏ qua và không tốn vòng.
- `verify` fail khi: nguồn metadata ngoài danh sách cho phép, thiếu tiêu đề hoặc tác giả, không có định danh truy được, tóm tắt cho bài không abstract, claim trích bài không được chọn, còn ứng viên chưa chấm, hoặc chưa chạy `gate`. `export` chạy `verify` và không ghi gì nếu fail.
- Nguồn hết hạn mức (OpenAlex trả HTTP 409) hoặc bị giới hạn tốc độ (429 sau khi thử lại) bị tắt cho phần còn lại của run (`SOURCE_DISABLED`); các nguồn khác vẫn chạy.
- `papers.csv` vô hiệu hóa ô bắt đầu bằng `= + - @` bằng dấu `'` ở đầu; `--bom` thêm BOM UTF-8 cho bảng tính.

## 5. Mã cảnh báo thường gặp

Xem danh sách đầy đủ trong skill `research-lab-delivery-core`. Hay gặp nhất:

| Cảnh báo | Ý nghĩa | Việc cần làm |
| --- | --- | --- |
| `MISSING_ABSTRACT: <title>` | Nguồn không có abstract | Không tóm tắt; chấm từ tiêu đề nếu cần |
| `SCORING_CANDIDATES_CAPPED: n -> m` | Số ứng viên vượt trần mỗi vòng | Phần còn lại được xếp hạng lại vòng sau; tăng `--max-scoring-candidates` nếu muốn |
| `SOURCE_DISABLED[openalex]` | Hết hạn mức hoặc bị giới hạn | Nhập/đổi key; chạy lại sau khi hạn mức đặt lại (00:00 UTC) |
| `INSUFFICIENT_RESULTS_AFTER_MAX_ROUNDS` | Hết vòng mà chưa đủ `min_papers` | Báo cáo nêu rõ; mọi kết luận chung yếu hơn |
| `LANGUAGE_MISMATCH` | Lý do hoặc tóm tắt khác ngôn ngữ báo cáo (ví dụ chữ Hán) | Screener/Synthesizer viết lại |
| `DOI_UNRESOLVED: <title>` | Cả Crossref lẫn doi.org đều không biết DOI này (DOI của arXiv thuộc DataCite nên được xác nhận qua doi.org, trạng thái `ok_other_registry`) | Auditor kiểm tra và ghi nhận |

## 6. Chạy lại và dọn dẹp

- Cache phản hồi: `$XDG_CACHE_HOME/multica/litscout` (hoặc thư mục cache của hệ điều hành), TTL 24 giờ; `--no-cache` bỏ qua, `--cache-dir` đổi chỗ. Khóa cache không chứa API key.
- `init --force` ghi đè `state.json`. Không có lệnh nào xóa tệp ngoài `research-run`.

## 7. Trạng thái xác minh

Xem mục "Báo cáo triển khai" trong [litscout-research-plan.md](litscout-research-plan.md) để biết lệnh kiểm tra đã chạy và những gì chưa xác minh (chưa chạy squad thật đầu cuối, chưa có tập vàng do người gắn nhãn).
