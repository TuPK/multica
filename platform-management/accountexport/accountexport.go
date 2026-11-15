// Package accountexport holds the pure parts of the account export: the rate
// limit constants and the scan that keeps secrets out of the file.
package accountexport

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Limit requests are allowed per Window for each user.
const (
	Limit  = 1
	Window = time.Hour
)

// minSecretLen is the shortest forbidden value the scan looks for. Shorter
// literals would match ordinary text.
const minSecretLen = 8

// redacted is the value the audit writer stores for a sensitive detail key.
const redacted = "[redacted]"

// forbiddenKeys are JSON object keys (compared in lower case) that must not
// appear with a real value anywhere in the export.
var forbiddenKeys = map[string]bool{
	"token_hash":             true,
	"token":                  true,
	"access_token":           true,
	"refresh_token":          true,
	"password":               true,
	"password_hash":          true,
	"secret":                 true,
	"client_secret":          true,
	"api_key":                true,
	"credentials":            true,
	"authorization":          true,
	"config_encrypted":       true,
	"webhook_secret":         true,
	"access_token_encrypted": true,
}

// RetryAfterSeconds rounds a wait up to whole seconds for the Retry-After
// header. The result is at least 1.
func RetryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	s := int((d + time.Second - 1) / time.Second)
	if s < 1 {
		return 1
	}
	return s
}

// FindSecrets walks a decoded JSON value and returns the paths where a
// forbidden key holds a value other than the audit redaction marker, or where
// a string contains one of the forbidden literals (token hashes and token
// values that belong to the user). An empty result means the value is clean.
func FindSecrets(v any, forbiddenValues []string) []string {
	var lits []string
	for _, s := range forbiddenValues {
		if len(s) >= minSecretLen {
			lits = append(lits, s)
		}
	}
	var found []string
	walk(v, "$", lits, &found)
	sort.Strings(found)
	return found
}

func walk(v any, path string, lits []string, found *[]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			p := path + "." + k
			if forbiddenKeys[strings.ToLower(k)] && child != redacted {
				*found = append(*found, p)
				continue
			}
			walk(child, p, lits, found)
		}
	case []any:
		for i, child := range t {
			walk(child, fmt.Sprintf("%s[%d]", path, i), lits, found)
		}
	case string:
		for _, l := range lits {
			if strings.Contains(t, l) {
				*found = append(*found, path)
				return
			}
		}
	}
}
