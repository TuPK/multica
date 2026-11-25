package service

// Chi phí phút chú ý ước tính của câu hỏi gửi người quyết định (durable-platform spec 7.2).
// Bảng ở đây là giá trị mặc định; job hằng tháng hiệu chỉnh bằng số đo thật, không tự đổi bảng.

const (
	// Dự đoán F5 đủ chắc thì câu hỏi tốn ít thời gian hơn một nửa.
	attentionPredictedConfidence = 0.8
	attentionPredictedDiscount   = 0.5
	// Mỗi câu thêm trong lô cộng thêm chừng này (spec 7.2).
	attentionExtraAskMinutes = 0.5
	// Proposal: cứ mỗi bấy nhiêu từ thân thì cộng thêm một phút.
	attentionWordsPerMinute = 400
)

// EstimateMinutes trả phút ước tính cho một câu hỏi và cho một lô gồm batchSize câu
// cùng loại, cùng đầu vào (batchSize ≥ 1). Lô gồm câu khác loại: dùng BatchMinutes
// với phút của từng câu từ askMinutes.
//
// Giả định:
//   - Số từ của proposal làm tròn xuống: 399 từ không cộng phút, 400 từ cộng một.
//   - words, irreversible bị bỏ qua với loại không liên quan.
//   - kind không biết trả 0 kể cả khi batchSize > 1.
func EstimateMinutes(kind string, words int, irreversible bool, predictedConf float64, batchSize int) float64 {
	per, known := askMinutes(kind, words, irreversible, predictedConf)
	if !known {
		return 0
	}
	return per + attentionExtraAskMinutes*float64(batchSize-1)
}

// BatchMinutes trả phút của cả lô: câu tốn nhiều nhất, cộng 0.5 cho mỗi câu còn lại.
func BatchMinutes(perAsk []float64) float64 {
	if len(perAsk) == 0 {
		return 0
	}
	largest := perAsk[0]
	for _, m := range perAsk[1:] {
		if m > largest {
			largest = m
		}
	}
	return largest + attentionExtraAskMinutes*float64(len(perAsk)-1)
}

// askMinutes trả phút của một câu và false khi kind không có trong bảng.
func askMinutes(kind string, words int, irreversible bool, predictedConf float64) (float64, bool) {
	var m float64
	switch kind {
	case "proposal":
		m = 3 + float64(words/attentionWordsPerMinute)
	case "org_change":
		m = 6
	case "effect_intent":
		m = 1
		if irreversible {
			m++
		}
	case "outcome_check":
		m = 4
	case "app_action_request":
		m = 1
	case "inquiry":
		m = 2
	default:
		return 0, false
	}
	if predictedConf >= attentionPredictedConfidence {
		m *= attentionPredictedDiscount
	}
	return m, true
}
