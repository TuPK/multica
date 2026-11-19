package deletionrequest

import (
	"strings"
	"unicode/utf8"
)

// MaxReasonChars is the longest reason a request accepts (spec R6 section 6.2, rule 2).
const MaxReasonChars = 500

// CleanReason trims a reason and checks it: required, at most MaxReasonChars.
func CleanReason(reason string) (string, *Error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", &Error{Status: 400, Code: CodeReasonRequired, Message: "a reason is required"}
	}
	if utf8.RuneCountInString(reason) > MaxReasonChars {
		return "", &Error{Status: 400, Code: CodeReasonTooLong, Message: "reason is longer than 500 characters"}
	}
	return reason, nil
}

// ConfirmMatches reports whether the typed confirmation equals the expected
// slug or tenant name (spec R6 GĐ10): both sides are trimmed at the ends and then
// compared exactly, so the match is case-sensitive.
func ConfirmMatches(typed, expected string) bool {
	return strings.TrimSpace(typed) == strings.TrimSpace(expected)
}
