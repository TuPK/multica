package billing

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	transferPrefix = "MUL"
	transferLen    = 8
	// transferAlphabet is A-Z and 2-9 without I and O (and the digits 0 and 1):
	// 24 letters and 8 digits, so 32 symbols and one random byte maps to a
	// symbol with the low five bits and no bias.
	transferAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// NewTransferCode returns a code for the customer to write in the transfer
// note: MUL and eight random symbols. The unique index on billing_order catches
// the rare collision.
func NewTransferCode() (string, error) {
	var b [transferLen]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("billing: transfer code: %w", err)
	}
	var sb strings.Builder
	sb.WriteString(transferPrefix)
	for _, v := range b {
		sb.WriteByte(transferAlphabet[v&31])
	}
	return sb.String(), nil
}

// ValidTransferCode reports whether s has the shape NewTransferCode makes.
func ValidTransferCode(s string) bool {
	if len(s) != len(transferPrefix)+transferLen || !strings.HasPrefix(s, transferPrefix) {
		return false
	}
	for _, c := range s[len(transferPrefix):] {
		if !strings.ContainsRune(transferAlphabet, c) {
			return false
		}
	}
	return true
}
