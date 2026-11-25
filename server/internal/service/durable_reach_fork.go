package service

import (
	"net/url"
	"strings"
)

// EffectRef là phần của effect_event mà Reach Map đọc. effect_event (B2) chưa có,
// nên đây là kiểu thuần; khi có bảng, SurfaceOf nhận db.EffectEvent qua một hàm chuyển đổi nhỏ.
type EffectRef struct {
	Source string
	Target string
}

// SurfaceOf trả bề mặt (spec 11.2) của một hệ quả. Giả định phân tích chuỗi:
//   - connected_app: Target dạng TOOLKIT_ACTION thì toolkit là phần trước dấu "_" đầu tiên
//     (không đổi hoa/thường); không tách được (không có "_", hoặc "_" ở đầu) thì dùng cả Target.
//   - channel: Target là loại kênh (ví dụ "slack").
//   - declared: Target là URL (có "://") thì lấy hostname, bỏ cổng; "smtp:<host>[:port]" và
//     "host:<host>[:port]" lấy phần host; còn lại, hoặc rỗng, là "unknown".
//   - nguồn khác: work_class cắt tới dấu ":" thứ hai ("flow:abc:stage" → "flow:abc");
//     không có hai dấu ":" thì giữ nguyên.
func SurfaceOf(e EffectRef) string {
	switch e.Source {
	case "connected_app":
		return "app:" + toolkitOf(e.Target)
	case "channel":
		return "channel:" + e.Target
	case "declared":
		return "external:" + hostOf(e.Target)
	default:
		return "area:" + workClassPrefix(e.Target)
	}
}

func toolkitOf(target string) string {
	if i := strings.Index(target, "_"); i > 0 {
		return target[:i]
	}
	return target
}

func hostOf(target string) string {
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil || u.Hostname() == "" {
			return "unknown"
		}
		return u.Hostname()
	}
	for _, prefix := range []string{"smtp:", "host:"} {
		if rest, ok := strings.CutPrefix(target, prefix); ok {
			host, _, _ := strings.Cut(rest, ":")
			if host != "" {
				return host
			}
			return "unknown"
		}
	}
	return "unknown"
}

func workClassPrefix(workClass string) string {
	parts := strings.SplitN(workClass, ":", 3)
	if len(parts) < 3 {
		return workClass
	}
	return parts[0] + ":" + parts[1]
}

// SurfaceStats là dữ liệu 30 ngày của một bề mặt, đã gom sẵn. ReachState chỉ đọc các trường này.
type SurfaceStats struct {
	EffectsAgent30d    int  // hệ quả do agent làm
	Effects30d         int  // mọi hệ quả trên bề mặt (người và agent)
	DeclaredEffects30d int  // hệ quả nguồn declared
	AgentConfirm30d    int  // hệ quả của agent đi qua confirm hoặc rehearse
	AllowedAgent       bool // có hệ quả của agent được allow trong 30 ngày
	GapAsks30d         int  // câu hỏi missing_access | missing_info
	MissingAccess      bool // có câu hỏi missing_access cho bề mặt này
	// HumanOnly không được ReachState đọc: bảng 11.2 chỉ dựa vào MissingAccess.
	// Giữ trường theo đầu vào đã định; chưa có chỗ dùng.
	HumanOnly bool
}

// ReachState trả trạng thái của bề mặt theo thứ tự ưu tiên của bảng 11.2.
// "declared_only" đòi mọi hệ quả là declared; "human_only" đòi không có hệ quả của agent.
func ReachState(s SurfaceStats) string {
	switch {
	case s.AllowedAgent && s.GapAsks30d == 0:
		return "agent_operable"
	case s.EffectsAgent30d > 0 && 2*s.AgentConfirm30d > s.EffectsAgent30d:
		return "agent_with_confirm"
	case s.EffectsAgent30d == 0 && s.DeclaredEffects30d > 0 && s.DeclaredEffects30d == s.Effects30d:
		return "declared_only"
	case s.MissingAccess && s.EffectsAgent30d == 0:
		return "human_only"
	default:
		return "unknown"
	}
}
