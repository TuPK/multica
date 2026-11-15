// Package emailhash names an email address in keys and logs without writing
// the address itself.
package emailhash

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Of returns the hex SHA-256 of the trimmed, lower-cased address.
func Of(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}
