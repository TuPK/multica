package litscout

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// RE2's \w is ASCII only, so word characters are spelled out as Unicode classes.
var wordRE = regexp.MustCompile(`[\p{L}\p{N}_]+`)

var foldMarks = transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// Normalize folds case, removes accents and punctuation, and collapses whitespace. Two titles that
// differ only in those ways normalise to the same string.
func Normalize(s string) string {
	out, _, err := transform.String(foldMarks, s)
	if err != nil {
		out = s
	}
	out = cases.Fold().String(out)
	return strings.Join(wordRE.FindAllString(out, -1), " ")
}

// Tokens returns the word tokens of a text after case folding. Accents are kept, so a Vietnamese
// word is matched as written.
func Tokens(s string) []string {
	return wordRE.FindAllString(cases.Fold().String(s), -1)
}

// IndelRatio is the normalised Indel similarity on a 0-100 scale: 100 * 2 * LCS / (len(a)+len(b)).
// It counts only insertions and deletions, which is what rapidfuzz's fuzz.ratio computes; it is
// not Levenshtein similarity.
func IndelRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 100
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			switch {
			case ra[i-1] == rb[j-1]:
				cur[j] = prev[j-1] + 1
			case prev[j] >= cur[j-1]:
				cur[j] = prev[j]
			default:
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
	}
	lcs := prev[len(rb)]
	return 100 * float64(2*lcs) / float64(len(ra)+len(rb))
}
