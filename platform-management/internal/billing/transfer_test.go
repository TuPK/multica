package billing

import (
	"regexp"
	"strings"
	"testing"
)

var transferCodePattern = regexp.MustCompile(`^MUL[A-HJ-NP-Z2-9]{8}$`)

func TestNewTransferCodeFormat(t *testing.T) {
	for range 500 {
		code, err := NewTransferCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 11 || !transferCodePattern.MatchString(code) {
			t.Fatalf("code %q does not match MUL + 8 of A-Z (no I, O) and 2-9", code)
		}
		if strings.ContainsAny(code[3:], "01IO") {
			t.Fatalf("code %q has an ambiguous character", code)
		}
	}
}

func TestTransferAlphabet(t *testing.T) {
	if len(transferAlphabet) != 32 {
		t.Fatalf("alphabet has %d characters; 32 keeps the byte mapping unbiased", len(transferAlphabet))
	}
	seen := map[rune]bool{}
	for _, c := range transferAlphabet {
		if seen[c] {
			t.Fatalf("duplicate %q", c)
		}
		seen[c] = true
	}
}

func TestNewTransferCodeDiffers(t *testing.T) {
	a, _ := NewTransferCode()
	b, _ := NewTransferCode()
	if a == b {
		t.Fatalf("two calls returned %q", a)
	}
	seen := map[string]bool{}
	for range 2000 {
		c, _ := NewTransferCode()
		if seen[c] {
			t.Fatalf("collision in 2000 codes: %q", c)
		}
		seen[c] = true
	}
}

func TestValidTransferCode(t *testing.T) {
	for _, ok := range []string{"MULABCDEFGH", "MUL23456789"} {
		if !ValidTransferCode(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "MUL", "MULABCDEFG", "MULABCDEFGHJ", "mulabcdefgh", "MULABCDEFG0", "MULABCDEFGI", "XYZABCDEFGH"} {
		if ValidTransferCode(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
