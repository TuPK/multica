package service

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// Vectors below come from RFC 8785 (JSON Canonicalization Scheme), fetched
// from https://www.rfc-editor.org/rfc/rfc8785.txt. Sections are cited per test.

// jcsDecode decodes JSON the way a caller of Canonicalize would: numbers stay
// as json.Number so the exact source digits reach the serializer.
func jcsDecode(t *testing.T, src string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(src))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", src, err)
	}
	return v
}

func jcsCanonical(t *testing.T, v any) string {
	t.Helper()
	b, err := Canonicalize(v)
	if err != nil {
		t.Fatalf("Canonicalize(%#v): unexpected error: %v", v, err)
	}
	return string(b)
}

// RFC 8785 section 3.2.3: the sorting test data. The RFC gives only the expected
// order of the values, not a full output line, so the test checks that order.
func TestCanonicalizeSortsKeysByUTF16(t *testing.T) {
	src := `{
  "\u20ac": "Euro Sign",
  "\r": "Carriage Return",
  "\ufb33": "Hebrew Letter Dalet With Dagesh",
  "1": "One",
  "\ud83d\ude00": "Emoji: Grinning Face",
  "\u0080": "Control",
  "\u00f6": "Latin Small Letter O With Diaeresis"
}`
	got := jcsCanonical(t, jcsDecode(t, src))

	// Expected value order from RFC 8785 section 3.2.3. U+1F600 (surrogates
	// D83D DE00) sorts before U+FB33 in UTF-16, though its code point is larger.
	ordered := []string{
		"Carriage Return",
		"One",
		"Control",
		"Latin Small Letter O With Diaeresis",
		"Euro Sign",
		"Emoji: Grinning Face",
		"Hebrew Letter Dalet With Dagesh",
	}
	last := -1
	for _, val := range ordered {
		idx := strings.Index(got, val)
		if idx < 0 {
			t.Fatalf("value %q missing from %s", val, got)
		}
		if idx <= last {
			t.Fatalf("value %q out of UTF-16 order in %s", val, got)
		}
		last = idx
	}
}

// RFC 8785 section 3.2.2 sample, and section 3.2.2.3 / Appendix B Table 1 for
// the number serialization. The expected sample output is copied from the RFC.
func TestCanonicalizeRFC8785Vectors(t *testing.T) {
	t.Run("section 3.2.2 sample", func(t *testing.T) {
		src := `{
  "numbers": [333333333.33333329, 1E30, 4.50,
              2e-3, 0.000000000000000000000000001],
  "string": "\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/",
  "literals": [null, true, false]
}`
		want := `{"literals":[null,true,false],"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27],"string":"€$\u000f\nA'B\"\\\\\"/"}`
		if got := jcsCanonical(t, jcsDecode(t, src)); got != want {
			t.Fatalf("got  %s\nwant %s", got, want)
		}
	})

	// Appendix B, Table 1: IEEE 754 bits and expected JSON serialization.
	// NaN and Infinity rows are not serializable and are covered by
	// TestCanonicalizeRejectsNonFinite.
	table := []struct {
		name string
		bits uint64
		want string
	}{
		{"zero", 0x0000000000000000, "0"},
		{"minus zero", 0x8000000000000000, "0"},
		{"min pos number", 0x0000000000000001, "5e-324"},
		{"min neg number", 0x8000000000000001, "-5e-324"},
		{"max pos number", 0x7fefffffffffffff, "1.7976931348623157e+308"},
		{"max neg number", 0xffefffffffffffff, "-1.7976931348623157e+308"},
		{"max pos int", 0x4340000000000000, "9007199254740992"},
		{"max neg int", 0xc340000000000000, "-9007199254740992"},
		{"~2**68", 0x4430000000000000, "295147905179352830000"},
		{"9.999999999999997e+22", 0x44b52d02c7e14af5, "9.999999999999997e+22"},
		{"1e+23", 0x44b52d02c7e14af6, "1e+23"},
		{"1.0000000000000001e+23", 0x44b52d02c7e14af7, "1.0000000000000001e+23"},
		{"999999999999999700000", 0x444b1ae4d6e2ef4e, "999999999999999700000"},
		{"999999999999999900000", 0x444b1ae4d6e2ef4f, "999999999999999900000"},
		{"1e+21", 0x444b1ae4d6e2ef50, "1e+21"},
		{"9.999999999999997e-7", 0x3eb0c6f7a0b5ed8c, "9.999999999999997e-7"},
		{"0.000001", 0x3eb0c6f7a0b5ed8d, "0.000001"},
		{"333333333.3333332", 0x41b3de4355555553, "333333333.3333332"},
		{"333333333.33333325", 0x41b3de4355555554, "333333333.33333325"},
		{"333333333.3333333", 0x41b3de4355555555, "333333333.3333333"},
		{"333333333.3333334", 0x41b3de4355555556, "333333333.3333334"},
		{"333333333.33333343", 0x41b3de4355555557, "333333333.33333343"},
		{"-0.0000033333333333333333", 0xbecbf647612f3696, "-0.0000033333333333333333"},
		{"1424953923781206.2", 0x43143ff3c1cb0959, "1424953923781206.2"},
	}
	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			if got := jcsCanonical(t, math.Float64frombits(tc.bits)); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCanonicalizeNumbers(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		// ES6 Number::toString: exponent form is "1e+21", not "1e21".
		{"1e21", float64(1e21), "1e+21"},
		{"1e-7", float64(1e-7), "1e-7"},
		{"0.000001", float64(0.000001), "0.000001"},
		{"100 has no .0", float64(100), "100"},
		{"1.5", float64(1.5), "1.5"},
		{"-0 becomes 0", math.Copysign(0, -1), "0"},
		{"4.5e-10", float64(4.5e-10), "4.5e-10"},
		{"json.Number 100.0", json.Number("100.0"), "100"},
		{"json.Number -0", json.Number("-0"), "0"},
		{"json.Number 1E+21", json.Number("1E+21"), "1e+21"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jcsCanonical(t, tc.in); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCanonicalizeRejectsNonFinite(t *testing.T) {
	cases := map[string]any{
		"NaN":                  math.NaN(),
		"+Inf":                 math.Inf(1),
		"-Inf":                 math.Inf(-1),
		"json.Number overflow": json.Number("1e400"),
	}
	for name, in := range cases {
		if _, err := Canonicalize(in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCanonicalizeStringEscapes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Only U+0000..U+001F, quote and backslash are escaped.
		{"named controls", "\b\t\n\f\r", `"\b\t\n\f\r"`},
		{"other controls use lowercase \\u00xx", "\x00\x01\x0b\x1f", `"\u0000\u0001\u000b\u001f"`},
		{"quote and backslash", `"\`, `"\"\\"`},
		{"slash is not escaped", "/", `"/"`},
		{"DEL is not escaped", "\x7f", "\"\x7f\""},
		{"U+0080 is not escaped", "\u0080", "\"\u0080\""},
		{"non-ASCII stays raw", "é€😀", `"é€😀"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jcsCanonical(t, tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCanonicalizeRejectsInvalidUTF8(t *testing.T) {
	// Encoded lone surrogate U+D800: not valid UTF-8, so RFC 8785 requires an error.
	if _, err := Canonicalize("\xed\xa0\x80"); err == nil {
		t.Fatal("expected an error for a lone surrogate")
	}
}

func TestCanonicalizeRejectsUnsupportedType(t *testing.T) {
	if _, err := Canonicalize(1); err == nil {
		t.Fatal("expected an error for a Go int; numbers must be float64 or json.Number")
	}
}

func TestCanonicalizeNoWhitespace(t *testing.T) {
	src := " { \"b c\" : [ 1 , 2 ] ,\n\t\"t\" : true , \"n\" : null , \"f\" : false } "
	want := `{"b c":[1,2],"f":false,"n":null,"t":true}`
	if got := jcsCanonical(t, jcsDecode(t, src)); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestCanonicalizeSortsNestedObjectsKeepsArrayOrder(t *testing.T) {
	src := `[{"b":1,"a":2},{"d":1,"c":2}]`
	want := `[{"a":2,"b":1},{"c":2,"d":1}]`
	if got := jcsCanonical(t, jcsDecode(t, src)); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestCanonicalizeRoundTripStable(t *testing.T) {
	docs := []string{
		`{"b":[3,{"z":1,"a":"é"}],"a":1e30}`,
		`{"numbers":[333333333.33333329,1E30,4.50,2e-3,0.000000000000000000000000001],"literals":[null,true,false]}`,
		`{"k":"line\nbreak \"quoted\" \\ back","empty":{},"list":[]}`,
	}
	for _, doc := range docs {
		first, err := Canonicalize(jcsDecode(t, doc))
		if err != nil {
			t.Fatalf("first pass of %s: %v", doc, err)
		}
		second, err := Canonicalize(jcsDecode(t, string(first)))
		if err != nil {
			t.Fatalf("second pass of %s: %v", first, err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("not stable for %s:\nfirst  %s\nsecond %s", doc, first, second)
		}
	}
}
