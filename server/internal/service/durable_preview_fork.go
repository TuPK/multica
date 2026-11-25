package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/multica-ai/multica/server/internal/connectedapps"
)

// Preview is what a person reads before approving a rehearsed action (durable spec
// 5.2). It is built from the call's arguments, never from the agent's words, so it
// says what the call will do.
type Preview struct {
	Kind         string         `json:"kind"`
	Title        string         `json:"title"`
	BodyMarkdown string         `json:"body_markdown"`
	Fields       []PreviewField `json:"fields"`
	Recipients   []string       `json:"recipients,omitempty"`
	AmountMicros *int64         `json:"amount_micros,omitempty"`
}

// PreviewField is one labelled value. Before is empty when the platform has not read
// the object, which is the case for a call that creates.
type PreviewField struct {
	Label  string `json:"label"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// previewValueMax bounds one shown value, so a preview stays readable.
const previewValueMax = 200

// BuildPreview renders the preview of one call. Keys starting with an underscore are
// the platform's own parameters (the rationale among them) and are never shown.
func BuildPreview(toolkit, tool string, args map[string]any) Preview {
	shown := map[string]any{}
	for k, v := range args {
		if !strings.HasPrefix(k, "_") {
			shown[k] = v
		}
	}
	upper := strings.ToUpper(tool)
	switch {
	case strings.Contains(upper, "EMAIL") || strings.Contains(upper, "MAIL") || connectedapps.NormalizeSlug(toolkit) == "gmail" || connectedapps.NormalizeSlug(toolkit) == "outlook":
		if p, ok := emailPreview(shown); ok {
			return p
		}
	case strings.Contains(upper, "POST") || strings.Contains(upper, "TWEET") || strings.Contains(upper, "PUBLISH"):
		if p, ok := postPreview(shown); ok {
			return p
		}
	case strings.Contains(upper, "CREATE") || strings.Contains(upper, "UPDATE") || strings.Contains(upper, "PATCH"):
		if p, ok := recordPreview(shown); ok {
			return p
		}
	}
	return genericPreview(tool, shown)
}

func emailPreview(args map[string]any) (Preview, bool) {
	to := stringList(firstArg(args, "to", "recipient", "recipients", "recipient_email"))
	body := firstString(args, "body", "body_markdown", "html_body", "text", "message")
	subject := firstString(args, "subject", "title")
	if len(to) == 0 && body == "" && subject == "" {
		return Preview{}, false
	}
	p := Preview{
		Kind:         "email",
		Title:        subject,
		BodyMarkdown: body,
		Recipients:   to,
		Fields: []PreviewField{
			{Label: "To", After: strings.Join(to, ", ")},
			{Label: "Subject", After: subject},
		},
	}
	return p, true
}

func postPreview(args map[string]any) (Preview, bool) {
	text := firstString(args, "text", "content", "message", "body")
	if text == "" {
		return Preview{}, false
	}
	p := Preview{
		Kind:         "post",
		Title:        firstString(args, "title", "name"),
		BodyMarkdown: text,
	}
	if ch := firstString(args, "channel", "page_id", "account", "channel_id"); ch != "" {
		p.Fields = append(p.Fields, PreviewField{Label: "Channel", After: ch})
	}
	return p, true
}

func recordPreview(args map[string]any) (Preview, bool) {
	var fields map[string]any
	for _, key := range []string{"properties", "fields", "data", "values"} {
		if m, ok := args[key].(map[string]any); ok {
			fields = m
			break
		}
	}
	if fields == nil {
		return Preview{}, false
	}
	p := Preview{Kind: "record"}
	for _, k := range sortedKeys(fields) {
		p.Fields = append(p.Fields, PreviewField{Label: k, After: previewValue(fields[k])})
	}
	return p, true
}

func genericPreview(tool string, args map[string]any) Preview {
	p := Preview{Kind: "generic", Title: tool}
	for _, k := range sortedKeys(args) {
		p.Fields = append(p.Fields, PreviewField{Label: k, After: previewValue(args[k])})
	}
	return p
}

func firstArg(args map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := args[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func firstString(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := args[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// stringList reads a recipient value that is one address or a list of them.
func stringList(v any) []string {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{strings.TrimSpace(t)}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return t
	}
	return nil
}

func previewValue(v any) string {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case nil:
		s = ""
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			s = fmt.Sprint(t)
		} else {
			s = string(raw)
		}
	}
	if r := []rune(s); len(r) > previewValueMax {
		return string(r[:previewValueMax]) + "…"
	}
	return s
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// CallSHA256 is the hash an approval binds to: the SHA-256 of the call's JSON, whose
// object keys are sorted, so the same call always hashes the same (durable spec 5.2).
func CallSHA256(c connectedapps.CanonicalCall) string {
	raw, err := json.Marshal(c)
	if err != nil {
		// A call the platform cannot encode has no hash; an empty hash never matches,
		// so such an intent cannot be approved.
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
