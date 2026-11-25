package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Canonicalize serializes a value decoded from JSON in the RFC 8785 (JCS) form:
// no insignificant whitespace, object keys sorted by UTF-16 code unit, numbers
// in ECMAScript Number::toString form, and strings escaped per section 3.2.2.2.
// Accepted values: map[string]any, []any, string, bool, nil, float64, json.Number.
// Signatures over attestations depend on this exact byte output.
func Canonicalize(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		return writeCanonicalString(buf, x)
	case float64:
		s, err := canonicalNumber(x)
		if err != nil {
			return err
		}
		buf.WriteString(s)
	case json.Number:
		f, err := strconv.ParseFloat(string(x), 64)
		if err != nil {
			return fmt.Errorf("canonicalize: number %q: %w", string(x), err)
		}
		s, err := canonicalNumber(f)
		if err != nil {
			return err
		}
		buf.WriteString(s)
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := writeCanonical(buf, x[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("canonicalize: unsupported type %T", v)
	}
	return nil
}

// utf16Less compares two strings by UTF-16 code units, as RFC 8785 requires.
// It differs from code point order for characters above U+FFFF.
func utf16Less(a, b string) bool {
	ua := utf16.Encode([]rune(a))
	ub := utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

// writeCanonicalString escapes per RFC 8785 section 3.2.2.2. Scanning bytes is
// safe: every byte that is escaped is ASCII, and UTF-8 continuation bytes are >= 0x80.
func writeCanonicalString(buf *bytes.Buffer, s string) error {
	if !utf8.ValidString(s) {
		return errors.New("canonicalize: string is not valid UTF-8 (lone surrogate)")
	}
	const hexDigits = "0123456789abcdef"
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\t':
			buf.WriteString(`\t`)
		case '\n':
			buf.WriteString(`\n`)
		case '\f':
			buf.WriteString(`\f`)
		case '\r':
			buf.WriteString(`\r`)
		default:
			if c < 0x20 {
				buf.WriteString(`\u00`)
				buf.WriteByte(hexDigits[c>>4])
				buf.WriteByte(hexDigits[c&0xf])
			} else {
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
	return nil
}

// canonicalNumber follows ECMAScript Number::toString (ECMA-262 7.1.12.1), which
// RFC 8785 section 3.2.2.3 mandates. Go's shortest round-trip digits match the
// ECMAScript digits; the layout rules below pick the ES6 exponent and decimal form.
func canonicalNumber(f float64) (string, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", fmt.Errorf("canonicalize: %v is not representable in JSON", f)
	}
	if f == 0 {
		return "0", nil // also covers -0
	}
	sign := ""
	e := strconv.FormatFloat(f, 'e', -1, 64) // e.g. "-4.5e-10"
	if e[0] == '-' {
		sign = "-"
		e = e[1:]
	}
	mant, expPart, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, err := strconv.Atoi(expPart)
	if err != nil {
		return "", fmt.Errorf("canonicalize: number %v: %w", f, err)
	}
	k := len(digits)
	n := exp + 1 // value = 0.digits × 10^n
	switch {
	case k <= n && n <= 21:
		return sign + digits + strings.Repeat("0", n-k), nil
	case 0 < n && n <= 21:
		return sign + digits[:n] + "." + digits[n:], nil
	case -6 < n && n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + digits, nil
	}
	ex := n - 1
	expSign := "+"
	if ex < 0 {
		expSign = "-"
		ex = -ex
	}
	m := digits[:1]
	if k > 1 {
		m += "." + digits[1:]
	}
	return fmt.Sprintf("%s%se%s%d", sign, m, expSign, ex), nil
}
