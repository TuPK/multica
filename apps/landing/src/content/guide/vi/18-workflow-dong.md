---
title: Workflow động, từ con số không đến khi chạy thật
order: 18
---

Chương này dành cho người chưa từng dựng workflow. Nó đưa bạn từ một squad trống đến một workflow chạy được việc thật, và giải thích từng nút bạn sẽ gặp trên đường đi.

## 1. Bạn đang dựng cái gì

**Squad** là một đội nhỏ gồm các agent, có một trưởng nhóm (lead). **Workflow** của squad là đường đi của một việc qua đội đó: ai làm từng bước, người kiểm tra nhìn vào điều gì, khi nào việc bị trả về để làm lại, và việc được phép kết thúc ở đâu.

Bạn không cần viết mã để dựng workflow. Bạn vẽ nó trong trình soạn workflow, từng bước một, và Delegora kiểm tra trước khi ai đó được chạy nó.

Các từ sau xuất hiện khắp trình soạn:

| Từ | Nghĩa đơn giản |
| --- | --- |
| **Squad** | Đội: một agent trưởng nhóm cộng các agent thành viên. |
| **Workflow** | Cả đường đi. Gồm các bước, vai, cạnh, vòng và điểm dừng. |
| **Bản nháp (draft)** | Workflow bạn đang sửa. Chưa có gì chạy theo bản nháp. |
| **Phiên bản (version)** | Bản đã xuất bản. Bản này bị khóa. Việc đang chạy giữ phiên bản mà nó đã bắt đầu. |
| **Bước (step)** | Một chỗ trên đường đi, nơi việc đang đứng. |
| **Vai (role)** | Ai làm việc ở bước: một agent, một thành viên, quản lý squad, hoặc người tạo yêu cầu. |
| **Task** | Một việc cụ thể bên trong bước, ví dụ "viết bản nháp". |
| **Verdict** | Kết quả task báo lại: đạt (pass), không đạt (fail) hoặc trung tính (neutral). Bước đi tiếp phụ thuộc vào kết quả này. |
| **Cổng (gate)** | Bước có nhiệm vụ kiểm tra việc của người khác. |
| **Cạnh (edge)** | Mũi tên từ bước này sang bước khác, có nhãn cho biết khi nào đi theo nó. |
| **Vòng (loop)** | Cạnh quay lại một bước trước, có giới hạn số lần. |
| **Điểm dừng (stop)** | Lý do có tên cho việc một yêu cầu kết thúc mà chưa xong, ví dụ "thiếu căn cứ". |
| **Đích (terminal)** | Chỗ yêu cầu kết thúc bình thường. Một đích có thể đưa việc ra khỏi workspace. |
| **Yêu cầu (request)** | Một việc đang đi qua workflow. |

## 2. Trước khi bắt đầu

Bạn cần:

- **Quyền admin hoặc owner của workspace**, hoặc một vai trò được quản lý squad. Chỉ quản lý squad mới tạo và xuất bản workflow.
- **Một squad** có trưởng nhóm và ít nhất một agent thành viên. Mỗi agent cần một runtime đang online, nếu không task của nó sẽ đợi.
- **Đã bật tính năng soạn workflow.** Nếu trình soạn hiện "Workflow authoring is off", tính năng chưa được bật cho cài đặt này. Hãy nhờ người vận hành máy chủ bật.

Ai làm được gì:

| Việc | Ai làm được |
| --- | --- |
| Xem danh sách workflow và đọc một phiên bản | Thành viên squad |
| Tạo hoặc sửa bản nháp, kiểm tra, mô phỏng | Quản lý squad |
| Xuất bản một phiên bản | Quản lý squad |
| Bật **Auto** cho một workflow | Chỉ admin |
| Xác nhận một workflow có thể rời workspace mà không có người xem | Chỉ admin |
| Đóng task thay cho agent | Quản lý squad (thao tác được ghi nhật ký) |

## 3. Bước 1: Dựng squad

1. Mở **Squads** ở thanh bên và bấm **New squad**. Đặt tên cho squad.
2. Chọn **trưởng nhóm (leader)**. Trưởng nhóm lập kế hoạch công việc và không tự nhận task làm việc.
3. Thêm **agent thành viên** từ trang squad. Mỗi agent cần các **skill** mà task của nó sẽ dùng. Bạn có thể thêm skill sau ở trang của agent.
4. Đảm bảo runtime của từng agent đang online. Nếu agent hiện offline, mở trang runtime để kiểm tra.

Nếu bạn đã có sẵn một bộ agent và skill đóng gói, bạn có thể bỏ qua bước thiết lập thủ công và nhập cả bộ. Phần 12 nói về cách này.

## 4. Bước 2: Tạo workflow mới

Mở squad và cuộn tới mục **Workflows**. Bấm **New workflow**. Một hộp thoại mở ra với bốn cách bắt đầu.

**Blank (trống).** Bạn có một bản nháp rỗng. Gõ tên rồi bấm **Create**. Trình soạn mở ra và bạn tự dựng từng bước. Dùng cách này khi quy trình của bạn khác các mẫu có sẵn.

**Copy a Kit Việt workflow (sao chép workflow Kit Việt).** Bạn chọn một mẫu Kit Việt có sẵn và nhận một bản sao. Bản sao tách biệt với bản gốc: thay đổi ở bản gốc không tới bản sao của bạn, và thay đổi ở bản sao không tới bản gốc. Tên được lấy từ mẫu, trừ khi bạn gõ tên khác.

**Import a file (nhập tệp).** Chọn tệp `flow.json`, hoặc dán nội dung của nó. Tệp phải là manifest của workflow và nhỏ hơn 512 KiB. Nếu tệp không hợp lệ, hộp thoại cho biết phần nào sai trước khi tạo bất cứ thứ gì.

**Ask an agent to draft (nhờ agent soạn).** Dùng cách này khi bạn đã có một quy trình viết sẵn và muốn một agent chuyển nó thành các bước:

1. Chọn **agent** sẽ soạn. Chọn agent đọc tài liệu tốt.
2. Ở ô **What the workflow should do**, mô tả quy trình bằng lời của bạn: ai làm gì, cần kiểm tra điều gì, và điều gì xảy ra khi kiểm tra không đạt.
3. Ở mục **Reference files**, đính kèm tài liệu agent cần đọc, hoặc tick hướng dẫn và skill có sẵn của squad.
4. Bấm **Ask the agent**. Delegora mở một issue thông thường giao cho agent, kèm ghi chú của bạn và danh sách thành viên squad. Bản nháp mới rỗng cho tới khi agent làm xong.
5. Khi agent đính một tệp `flow.json` vào issue đó, trang bản nháp hiện nút **Import proposal**. Bấm nút này. Bản nháp được điền bằng đề xuất.
6. Đọc đề xuất trong trình soạn, sửa những gì chưa đúng, rồi chạy **Check** và **Simulate** như với mọi bản nháp khác. Xuất bản cần thêm một bước xác nhận, xem phần 9.

Agent chỉ đề xuất. Nó không thể xuất bản, không thể gắn vai, và không thể bật một workflow.

## 5. Bước 3: Làm quen màn hình trình soạn

Mở một bản nháp để vào trình soạn. Trình soạn có ba khu vực.

**Thanh trên cùng** gồm:

- **Tên workflow**. Bấm vào để đổi tên.
- **Trạng thái lưu**: "Saving…", "Saved" hoặc "Unsaved changes". Trình soạn tự lưu khoảng 1,5 giây sau thay đổi cuối cùng của bạn. Nếu không lưu được, bạn thấy "Could not save" và thay đổi vẫn nằm trên màn hình. Hãy sao chép những gì quan trọng trước khi rời trang.
- **Check**: liệt kê các lỗi của bản nháp.
- **Simulate**: chạy đường đi mà không cần agent nào.
- **Export**: tải bản nháp về thành tệp `flow.json`.
- **Publish**: biến bản nháp thành một phiên bản bị khóa. Nút mờ đi khi còn lỗi.

**Cột bên trái, Steps (danh sách bước),** liệt kê mọi bước theo thứ tự việc chạy qua. Dùng ô tìm kiếm để tìm bước theo tên. Bấm một bước để mở nó. Menu **Add step** ở đầu cột này dùng để thêm bước mới.

**Cột giữa, Inspector,** chỉnh thứ bạn vừa chọn: một bước, một task hoặc một cạnh.

**Cột bên phải** chứa các bảng phụ. Chỉ mở từng bảng khi cần:

- **Roles**: ai được làm từng bước, và agent nào thuộc từng vai.
- **Variables**: các giá trị có tên mà workflow có thể đọc, như một con số hoặc một lựa chọn có/không.
- **Verdicts**: các bộ kết quả mà task có thể báo, và mỗi kết quả tính là đạt hay không đạt.
- **Loops**: mỗi vòng sửa được chạy tối đa bao nhiêu lần.
- **Stops**: các lý do có tên mà yêu cầu có thể dừng.
- **Terminals**: các chỗ yêu cầu có thể kết thúc.
- **Reference**: hướng dẫn agent và skill của squad, chỉ để đọc, để bạn đối chiếu quy trình gốc khi đang dựng.

**Tab Graph** vẽ các bước thành một bản đồ. Đây là để xem, không phải để sửa:

- Bấm một bước trên bản đồ để chọn nó trong Inspector.
- **Fit** co bản đồ để mọi bước vừa màn hình. Bản đồ không thu nhỏ dưới một nửa kích thước, nên với workflow lớn bản đồ sẽ cuộn.
- **Find a step** nhảy tới một bước theo tên.
- **Show loops only** ẩn mọi thứ không thuộc vòng nào.
- Không có nút phóng to hay thu nhỏ. Hãy cuộn bản đồ bằng thanh cuộn hoặc touchpad.

**Trên bản đồ này không có kéo thả.** Bạn không kéo bước hay vẽ mũi tên bằng cách kéo. Mọi thay đổi làm trong Inspector và các bảng phụ. Chỗ duy nhất đổi thứ tự bằng nút là cạnh: mỗi cạnh trong danh sách cạnh của một bước có mũi tên lên và xuống. (Kéo thả chỉ có ở trình soạn workflow trạng thái cũ, trong Settings → Workflows.)

Trên điện thoại, danh sách bước hiện dạng danh sách, và chạm vào một bước sẽ mở Inspector thành một khung toàn màn hình. Bản đồ đổi thành danh sách bước theo chiều dọc.

## 6. Bước 4: Dựng bước đầu tiên

Phần này dựng một workflow nội dung nhỏ. Có bốn bước:

1. **Draft (soạn)**: một agent viết bài.
2. **Review (duyệt)**: một agent biên tập kiểm tra. Nó có thể cho qua, trả bài về Draft, hoặc chặn.
3. **Approve (phê duyệt)**: một người thuộc quản lý squad phê duyệt.
4. **Done (xong)**: đích nơi việc kết thúc.

Bắt đầu với bước đầu tiên.

1. Ở cột **Steps**, bấm **Add step** và chọn **Work**. Một bước mới hiện ra ở cuối danh sách và mở trong Inspector.
2. Trong Inspector, đặt **tên bước**, ví dụ "Draft". Giữ loại là **Tasks**. Giữ **Wait for** là **All tasks**: bước xong khi mọi task của nó xong.
3. Ở mục **Tasks**, bấm **Add task**. Đặt tên cho task, ví dụ "Viết bản nháp". Chọn **loại** của nó: **Work**. Ô **Role** còn trống; bạn sẽ điền ở bước 5.
4. Viết **Instructions** cho task: agent làm gì, kết quả phải có gì, và đặt kết quả ở đâu. Agent đọc phần này, nên hãy viết cụ thể.
5. Nếu task cần một tệp, tick **Needs an attachment** và ghi rõ loại tệp. Một task yêu cầu tệp chỉ xong khi agent đính kèm tệp đó.

## 7. Bước 5: Tạo vai và gán người

Vai là một "ghế" trong workflow. Task được giao cho vai, không giao trực tiếp cho agent. Nhờ vậy bạn đổi agent sau này mà không phải sửa các bước.

1. Mở bảng **Roles** ở bên phải và bấm **Add role**.
2. Đặt **key** (tên ngắn như `writer`) và **label** (tên hiển thị, như "Writer"). Chọn loại:
   - **Agent**: một hoặc nhiều agent của squad.
   - **Member**: một thành viên người của workspace.
   - **Squad managers**: bất kỳ ai quản lý squad.
   - **Requester**: người tạo yêu cầu.
3. Với vai loại Agent, dùng **Choose an agent** để chọn các agent được nhận vai này. Cột **Pool** cho biết số lượng. Nếu thiếu agent bạn cần, dùng **Add agent** để thêm nó vào squad trước.
4. Quay lại Inspector, đặt **Role** của task là `writer`.

Trưởng nhóm không thể gắn vào vai của task. Vai nào không task nào dùng sẽ bị đánh dấu là chưa dùng, và Check báo lỗi này.

## 8. Bước 6: Thêm bước duyệt và nối các bước

Cổng là bước kiểm tra việc của người khác.

1. **Add step** → **Gate**. Cổng đi kèm ba cạnh dựng sẵn và một task kiểm tra:
   - **Passed**: đi tiếp.
   - **Send back for rework**: quay lại bước đã làm việc, khi còn lượt sửa.
   - **Blocked**: dừng yêu cầu ở trạng thái stalled, và phải có người xem.
2. Đổi tên bước cổng thành "Review". Mở task của nó, đổi tên thành "Check the draft", và đặt vai mới là `editor`. Hãy chắc rằng `editor` là agent khác với `writer`. Cổng không được giao cho agent đã làm việc mà cổng đang chấm. Check báo đây là lỗi tách vai.
3. Đổi cạnh **Send back for rework** để trỏ tới "Draft". Mỗi cạnh có một **Destination**: một bước, một đích, hoặc một điểm dừng.
4. Đổi cạnh **Passed** để trỏ tới "Approve", bước người sẽ thêm ở phần sau.

Danh sách cạnh nằm ở đâu, và dùng thế nào:

- Mỗi bước liệt kê các cạnh ra theo thứ tự. Thứ tự quan trọng: cạnh được kiểm từ trên xuống, và cạnh cuối cùng là phương án dự phòng.
- Dùng mũi tên lên và xuống để đổi thứ tự.
- Dùng **Add edge** để thêm cạnh, và biểu tượng thùng rác để xóa.
- Một cạnh có thể có **điều kiện (condition)**, dựng bằng bộ xây điều kiện. Bộ này so sánh một dữ kiện hoặc biến, và ghép các luật bằng AND, OR và NOT. Để trống điều kiện nếu cạnh luôn được đi.

## 9. Bước 7: Thêm phê duyệt của người và giới hạn vòng sửa

**Thêm phê duyệt của người.** Bấm **Add step** → **Human approval**. Đây là bước thủ công: yêu cầu đợi ở đó cho tới khi một người hành động.

- Trong Inspector, đặt **Exit role** là **Squad managers**. Chỉ những người này mới đưa yêu cầu ra khỏi bước này được.
- Đặt **Notice message**. Thông báo này được gửi khi yêu cầu vào bước, để người phê duyệt biết phải làm gì.
- Nối cạnh **Passed** của bước Review tới bước này, và nối cạnh thủ công của bước này tới **Done**.

**Giới hạn vòng sửa.** Cạnh **Send back for rework** của bước Review là một vòng. Mở bảng **Loops**:

- Tìm vòng, ví dụ `review`.
- Đặt **Up to N times**, ví dụ 2. Sau hai lần trả về Draft, lần không đạt tiếp theo đi theo cạnh bạn đã đặt cho "loop used up", hoặc yêu cầu dừng ở điểm dừng **Loop used up**.
- Luôn cho vòng một cạnh cho trường hợp "đã dùng hết lượt". Check báo lỗi khi một bước quay lại mà không có cạnh nào cho trường hợp đó.

**Thêm đích.** Mở bảng **Terminals**. Hầu hết mẫu đã có sẵn một đích tên **Done**. Hãy chắc rằng lối ra của bước Approve dẫn tới đích đó. Nếu một đích được đánh dấu **Leaves the workspace**, việc sẽ rời workspace khi yêu cầu kết thúc tại đó. Đích như vậy cần một người xem việc trước, và workflow có nó có thể hiện cảnh báo phát hành.

**Điểm dừng.** Mở bảng **Stops** để xem các lý do dừng. Điểm dừng có sẵn là **Missing grounds** và **Loop used up**. Bạn có thể thêm điểm dừng riêng kèm thông điệp. Thông điệp có thể dùng giá trị từ biến, và bạn xem trước được trước khi lưu.

## 10. Bước 8: Kiểm tra bản nháp

Bấm **Check**. Một danh sách lỗi mở ra, nhóm theo bước. Bấm một dòng để nhảy tới đúng chỗ.

Các lỗi gặp thường xuyên nhất, và cách xử lý:

| Thông báo | Nghĩa là | Cách làm |
| --- | --- | --- |
| Một bước không thể đi tới từ điểm bắt đầu | Không có cạnh nào dẫn tới bước đó | Thêm một cạnh dẫn vào, hoặc xóa bước |
| Vai cần N người, mới có M | Vai có ít agent hơn số task cần | Thêm agent vào pool của vai |
| Cổng không thể giao cho người đã làm việc mà nó chấm | Cùng một agent vừa làm vừa chấm | Giao cổng cho agent khác |
| Vai là người và không có skill | Vai loại Member không có skill nào | Gán một skill cho vai, hoặc đổi loại vai |
| Bước quay lại nhưng không có cạnh sau cho trường hợp hết lượt | Vòng không có lối thoát | Thêm cạnh cho "loop used up" |
| Bước nằm trên một chu trình không có cạnh vòng | Việc có thể chạy vòng mãi | Đánh dấu cạnh quay lại là vòng |
| Không có đường nào từ điểm bắt đầu tới đích mà không qua điểm dừng | Yêu cầu không bao giờ kết thúc được | Nối một đường tới đích |
| Bước thủ công không có cạnh thủ công | Yêu cầu sẽ đợi mãi | Thêm cạnh thủ công |
| Bước thủ công không có thông báo | Người phê duyệt không biết phải làm gì | Đặt thông điệp thông báo |
| Đích đưa việc ra khỏi workspace mà không có người xem | Cảnh báo phát hành | Thêm phê duyệt của người trước đích, hoặc nhờ admin xác nhận |
| Một vai không được dùng | Không task nào dùng vai đó | Dùng vai đó, hoặc xóa |

Lỗi sẽ chặn **Publish**. Cảnh báo không chặn, nhưng hãy đọc từng cái. Cảnh báo cần xác nhận sẽ yêu cầu bạn tick một ô trước khi xuất bản.

## 11. Bước 9: Mô phỏng đường đi

Bấm **Simulate**. Chọn giá trị cho từng biến, hoặc để biến ở **Default**. Bấm **Run**.

Bạn thấy:

- **Task results**: một dòng mỗi lần một bước được vào.
- **Path**: thứ tự các bước và cạnh đã đi, kèm số lần ghé.
- **Ends at**: chỗ mô phỏng kết thúc.
- **Loops**: mỗi vòng đã chạy bao nhiêu lần.

Đọc đường đi với hai câu hỏi: mỗi bước có đi tới chỗ bạn mong đợi không, và mỗi yêu cầu có kết thúc ở đâu đó không. Simulate chỉ kiểm tra đường đi. Nó không kiểm tra agent làm việc tốt hay không. Việc đó là chức năng của chạy thử.

## 12. Bước 10: Xuất bản một phiên bản

Bấm **Publish**. Hộp thoại hiển thị:

- Một ghi chú: xuất bản tạo ra một phiên bản bị khóa, và các yêu cầu đang chạy giữ phiên bản của chúng.
- Kết quả của một lượt kiểm tra mới. Nếu có lỗi, bạn thấy "Fix the errors before publishing" và nút không bật.
- **Make this the active workflow**. Tick để biến phiên bản này thành phiên bản các yêu cầu mới sẽ dùng. Bỏ tick để giữ phiên bản đang active cũ và thử bản mới bằng chạy thử trước.
- Nếu workflow có thể rời workspace mà không có người xem, cảnh báo phát hành sẽ hiện. Chỉ admin mới tick được xác nhận.
- Nếu bản nháp đến từ một agent, bạn phải tick **I have read this workflow** trước khi nút được bật.

Nếu có người khác xuất bản một phiên bản mới hơn khi bạn đang sửa, bạn thấy "A newer version was published". Bấm **Edit the latest version** để mở phiên bản đó.

Sau khi xuất bản:

- Phiên bản xuất hiện trong danh sách **Workflows** của squad, kèm số phiên bản.
- **Auto** tắt ở phiên bản mới, kể cả khi phiên bản cũ đang bật. Một admin bật lại sau khi xem phiên bản mới.
- Bản nháp vẫn nằm trong danh sách **Drafts** để bạn tiếp tục sửa. Xuất bản lần nữa sẽ tạo một phiên bản mới.

## 13. Bước 11: Thử bằng chạy thử

Đừng chuyển một squad đang chạy sang một workflow chưa thử. Hãy thử trước:

1. Trong mục **Workflows** của squad, tìm dòng phiên bản và bấm **Trial run**.
2. Nhập tiêu đề cho một yêu cầu mẫu, ví dụ "Bài mẫu về cà phê". Bấm **Start**.
3. Yêu cầu chạy với agent thật trên cùng engine. Nó có huy hiệu **Trial**.

Yêu cầu chạy thử tách biệt với các yêu cầu thật. Nó không làm thay đổi cách các yêu cầu khác chạy. Nó cũng không được tính vào số liệu mặc định. Việc từ chạy thử không thể rời workspace khi chưa có người xem, nên bạn có thể thử đường đi thật một cách an toàn.

## 14. Bước 12: Dùng workflow cho một yêu cầu

Để giao việc thật cho một squad, tạo một issue và giao nó cho squad.

- Nếu squad có nhiều hơn một workflow đã xuất bản, form tạo issue hiện bộ chọn **workflow**. Mặc định là phiên bản đang active.
- Trưởng nhóm lập kế hoạch cho yêu cầu, rồi yêu cầu đi theo workflow từng bước.
- Các việc định kỳ và yêu cầu do agent tạo luôn dùng phiên bản đang active.

## 15. Bước 13: Theo dõi một yêu cầu

Mở issue. Panel **Flow** cho biết yêu cầu đang ở đâu:

- **Step**: bước hiện tại.
- **Round**: đã dùng bao nhiêu vòng sửa, hiển thị dạng `↺ n/max`, ví dụ `↺ 1/2`.
- **Outcome**: Pending, Approved, Rejected, Blocked hoặc Done.
- **Tasks**: từng task với vai, trạng thái và kết quả.
- **Path**: các bước và cạnh đã đi, kèm thời gian. Mở một dòng để xem dữ kiện đằng sau lựa chọn đó.
- **Feedback**: nhận xét mà một lần sửa đã trả về, để người viết biết cần sửa gì.

Khi một task đang chờ quyết định, hãy chọn kết quả mà task đó cho phép. Panel chỉ đưa ra các kết quả thuộc về task đó.

Các nút khác:

- **Close on behalf**: quản lý squad có thể đóng một task thay cho agent. Task hiện dòng "Closed by {tên} on behalf of the agent". Chỉ dùng khi agent không thể hoàn thành và bạn đã kiểm tra công việc.
- **Exit**: đi lối thủ công, như **Approve and continue**, khi bước đang đợi một người.
- **Back to drafting** và **Resume**: đưa một yêu cầu bị stalled về bản nháp, hoặc tiếp tục nó. Resume sẽ hỏi bạn xác nhận khi nó bỏ qua một điểm kiểm mà chưa có người xem.
- **Detach**: tách yêu cầu khỏi engine workflow. Các task đang mở trở thành issue con thông thường và workflow ngừng điều khiển chúng. Thao tác này không hoàn tác được, nên chỉ dùng khi workflow hỏng với yêu cầu này.

Các lý do dừng bạn có thể thấy:

| Điểm dừng | Ý nghĩa |
| --- | --- |
| Stop 1: Blocked | Một kiểm tra thấy thiếu căn cứ: yêu cầu thiếu thứ kiểm tra cần. |
| Stop 3: Max returns reached | Vòng sửa đã dùng hết lượt. |
| Stalled | Yêu cầu không tự đi tiếp được. Phải có người hành động: đi lối thoát, tiếp tục, hoặc tách. |

## 16. Bước 14: Đọc số liệu và cải thiện

Mở một phiên bản đã xuất bản và xem **Insights**. Với tối đa 500 yêu cầu gần nhất trên phiên bản đó, bạn thấy:

- có bao nhiêu yêu cầu, và bao nhiêu cái còn mở;
- với mỗi bước, số lần bước được vào, và thời gian trung vị cùng phân vị 90 đã dành ở đó;
- với mỗi cạnh, số lần cạnh được đi;
- với mỗi vòng, bao nhiêu yêu cầu đã quay lại 0, 1, 2 lần, v.v.

Tick **Include trial runs** để tính cả các yêu cầu chạy thử.

Dùng số liệu để quyết định nên đổi gì. Ví dụ:

- Một cổng trả việc về trong phần lớn yêu cầu: hướng dẫn của người viết có thể chưa rõ. Hãy cải thiện hướng dẫn của task.
- Một vòng hết lượt thường xuyên: chỉ nên tăng giới hạn sau khi đã kiểm tra hướng dẫn.
- Một bước mất nhiều ngày: kiểm tra xem có người nào đang đợi ở đó không, hoặc agent có đang offline không.

Thực hiện thay đổi trong một bản nháp mới, kiểm tra, mô phỏng, xuất bản, rồi chạy thử lại. Số liệu không tự thay đổi workflow.

## 17. Tùy chọn: bật Auto

**Auto** cho phép hệ thống đưa yêu cầu đi tiếp trong workflow mà không đánh thức trưởng nhóm ở mọi bước nhỏ. Chỉ admin mới bật được, và chỉ trên một phiên bản đã xuất bản. Phiên bản mới bắt đầu với Auto tắt.

Chỉ bật sau khi bạn đã đọc Insights của phiên bản đó và đã chạy thử. Auto không thay đổi những bước nào tồn tại; nó chỉ thay đổi ai là người đưa yêu cầu đi tiếp.

## 18. Lối tắt: nhập cả bộ

Nếu bạn đã có sẵn một bộ agent, skill và một flow đóng gói, bạn không cần thiết lập từng thứ bằng tay.

1. Mở **Squads** và bấm **Import a pack**.
2. Tải lên tệp `.zip`, hoặc dán link GitHub hay link tới tệp `.zip`.
3. Bấm **Read the pack**. Bản xem trước hiển thị:
   - **Agents**: tạo mới từng agent, dùng agent đang có, hoặc bỏ qua. Với agent mới, chọn một runtime.
   - **Skills**: skill trùng tên với skill bạn đã có sẽ giữ bản hiện tại, hoặc được nhập thành skill mới. Không có gì bị ghi đè.
   - **Leader**: chọn một agent trong bộ, hoặc dùng trưởng nhóm chung.
   - **Ignored settings**: tools, model, biến môi trường và cấu hình MCP trong bộ không được áp dụng. Bạn tự thiết lập chúng sau đó.
   - **Security scan**: bộ bị chặn thì không cài được. Cảnh báo phải được xác nhận bằng cách tick.
4. Bấm **Install**. Bạn thấy những gì đã được tạo.
5. Nếu bộ có kèm một flow, bấm **Design the workflow**. Một bản nháp mở ra từ flow đó. Tiếp tục từ phần 5.

Các định dạng được nhận: bộ có tệp `squad-bundle.json`, agent pack có tệp `pack.json`, thư mục agent viết dạng `agents/*.md` (kiểu Claude Code), hoặc thư mục skill, mỗi thư mục có tệp `SKILL.md`.

## 19. Sự cố bạn có thể gặp

| Bạn thấy | Vì sao | Cách làm |
| --- | --- | --- |
| "Workflow authoring is off" | Tính năng chưa được bật cho cài đặt này | Nhờ người vận hành máy chủ |
| "Could not save" | Mạng hoặc máy chủ lỗi | Chờ rồi xem trạng thái lưu; sao chép nội dung nếu không tự hồi phục |
| "Someone saved this draft first" | Người khác đã sửa bản nháp | Chọn **Reload**, hoặc **Keep mine as a copy** để giữ bản của bạn riêng |
| "Fix the errors before publishing" | Check vẫn còn lỗi | Bấm dòng lỗi, sửa, kiểm tra lại |
| "A newer version was published" | Phiên bản bạn sửa không còn là mới nhất | Bấm **Edit the latest version** và làm lại thay đổi ở đó |
| "Only workspace admins can create statuses" | Workflow dùng một trạng thái chỉ admin tạo được | Nhờ admin, hoặc dùng trạng thái đã có |
| "The role lacks skills" | Task cần một skill mà agent trong vai không có | Thêm skill cho agent, hoặc bỏ skill khỏi task |
| Một yêu cầu bị stalled | Chưa ai hành động | Mở yêu cầu và làm hành động tiếp theo trong panel Flow |
| Task đợi và không bao giờ bắt đầu | Agent offline | Kiểm tra runtime của agent trên trang của nó |

## 20. Các từ bạn đã thấy

- **Active**: phiên bản mà các yêu cầu mới dùng.
- **Unreadable**: một workflow đã lưu mà phiên bản ứng dụng này không đọc được. Cập nhật ứng dụng, hoặc nhờ admin.
- **Trial**: yêu cầu được bắt đầu từ chạy thử.
- **On behalf**: việc đóng do quản lý làm thay cho agent.
