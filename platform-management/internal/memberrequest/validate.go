package memberrequest

import (
	"net/mail"
	"slices"
	"strings"
)

const maxEmailBytes = 254

// validEmail reports whether s is one plain address: no display name, no
// comment, and a domain with a dot.
func validEmail(s string) bool {
	if s == "" || len(s) > maxEmailBytes {
		return false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	return at > 0 && strings.Contains(s[at+1:], ".")
}

// ValidateInput checks the input of spec R6 section 4.2, rule 2 and returns it
// cleaned: the email trimmed and lower-cased, the role trimmed. A bad email is a
// 400 member_request_invalid naming the field; a role outside the member roles,
// owner included, is a 400 member_role_invalid.
func ValidateInput(in Input) (Input, *Error) {
	out := Input{Email: strings.ToLower(strings.TrimSpace(in.Email)), Role: strings.TrimSpace(in.Role)}
	if !validEmail(out.Email) {
		return Input{}, &Error{Status: 400, Code: CodeInvalid, Message: "the request has invalid fields", Fields: []string{"email"}}
	}
	if !slices.Contains(Roles, out.Role) {
		return Input{}, &Error{Status: 400, Code: CodeRoleInvalid, Message: "role must be admin, member or viewer"}
	}
	return out, nil
}
