package workspaceprovision

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// SlugPattern is the shape every workspace slug must have.
var SlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

var diacritics = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// DeriveSlug turns a workspace name into a slug candidate: diacritics removed
// (đ/Đ become d), lowercase, runs of other characters become one hyphen. The
// result may be empty and may be reserved; the caller checks both.
func DeriveSlug(name string) string {
	name = strings.NewReplacer("đ", "d", "Đ", "d").Replace(name)
	if out, _, err := transform.String(diacritics, name); err == nil {
		name = out
	}
	name = nonSlugRun.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(name, "-")
}

// reservedSlugs mirrors handler/reserved_slugs.json (embedded files cannot be
// shared across packages, so this is a copy).
//
//go:embed reserved_slugs.json
var reservedSlugsJSON []byte

var reservedSlugs = loadReservedSlugs()

func loadReservedSlugs() map[string]bool {
	var data struct {
		Groups []struct {
			Slugs []string `json:"slugs"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(reservedSlugsJSON, &data); err != nil {
		panic("workspaceprovision: parse reserved_slugs.json: " + err.Error())
	}
	out := make(map[string]bool)
	for _, g := range data.Groups {
		for _, s := range g.Slugs {
			out[s] = true
		}
	}
	return out
}

// IsReservedSlug reports whether slug collides with a frontend route or
// platform name.
func IsReservedSlug(slug string) bool { return reservedSlugs[slug] }

func uuidString(id pgtype.UUID) string { return uuid.UUID(id.Bytes).String() }

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

// DefaultIssuePrefix derives a new workspace's default issue prefix from its
// slug: alphanumerics only, first 4 chars, uppercased. "front-end" gives
// "FRON". The slug is the source, not the name (MUL-6050), and the rule is kept
// in sync with the preview in packages/views/onboarding/steps/step-workspace.tsx.
func DefaultIssuePrefix(slug string) string {
	head := nonAlphanumeric.ReplaceAllString(slug, "")
	if head == "" {
		return "WS"
	}
	if len(head) > 4 {
		head = head[:4]
	}
	return strings.ToUpper(head)
}
