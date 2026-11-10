package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	// RegistrySchemaVersion is the only index schema this package reads.
	RegistrySchemaVersion = 1
	// MaxRegistryItems caps how many valid items one index contributes. Items
	// past the cap are dropped and Registry.Truncated is set.
	MaxRegistryItems = 500
	// MaxRegistryBytes bounds the downloaded index.
	MaxRegistryBytes = 2 << 20

	registryIndexFile = "hub-index.json"
	maxRegistryText   = 1000
	maxRegistryName   = 200
	maxRegistryTags   = 20
	maxRegistryTag    = 50
)

var registryNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
var sha256HexPattern = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// RegistryItem is one validated entry of a registry index.
type RegistryItem struct {
	Kind        Kind
	Name        string
	Description string
	Version     string
	URL         string
	SHA256      string
	Tags        []string
}

// Ref returns the SourceRef for downloading this item's archive.
func (it RegistryItem) Ref() SourceRef {
	return SourceRef{Type: SourceRegistry, URL: it.URL, SHA256: it.SHA256}
}

// Registry is a parsed hub-index.json.
type Registry struct {
	Name  string
	Items []RegistryItem
	// Skipped counts items that failed validation or duplicated an earlier
	// kind+name. They are dropped, not fatal, so one bad entry does not block the
	// rest of the index.
	Skipped int
	// Truncated is set when the index had more valid items than MaxRegistryItems.
	Truncated bool
}

type registryDoc struct {
	Schema *int              `json:"schema"`
	Name   string            `json:"name"`
	Items  []json.RawMessage `json:"items"`
}

type registryItemDoc struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	URL         string   `json:"url"`
	SHA256      string   `json:"sha256"`
	Tags        []string `json:"tags"`
}

// ParseRegistry decodes and validates an index document. A wrong or missing
// schema, or a missing items array, fails the whole index. Individual items are
// validated one at a time and invalid ones are counted in Skipped.
func ParseRegistry(data []byte) (*Registry, error) {
	if len(data) > MaxRegistryBytes {
		return nil, fmt.Errorf("%w: index is %d bytes, limit %d", ErrTooLarge, len(data), MaxRegistryBytes)
	}
	var doc registryDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRegistry, err)
	}
	if doc.Schema == nil || *doc.Schema != RegistrySchemaVersion {
		return nil, fmt.Errorf("%w: schema must be %d", ErrInvalidRegistry, RegistrySchemaVersion)
	}
	if doc.Items == nil {
		return nil, fmt.Errorf("%w: items array is required", ErrInvalidRegistry)
	}

	reg := &Registry{Name: clipText(doc.Name, maxRegistryName)}
	seen := make(map[string]bool, len(doc.Items))
	for _, raw := range doc.Items {
		item, ok := validateRegistryItem(raw)
		if !ok {
			reg.Skipped++
			continue
		}
		key := string(item.Kind) + "/" + item.Name
		if seen[key] {
			reg.Skipped++
			continue
		}
		if len(reg.Items) == MaxRegistryItems {
			reg.Truncated = true
			break
		}
		seen[key] = true
		reg.Items = append(reg.Items, item)
	}
	return reg, nil
}

func validateRegistryItem(raw json.RawMessage) (RegistryItem, bool) {
	var doc registryItemDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return RegistryItem{}, false
	}
	kind := Kind(doc.Kind)
	if kind != KindSkill && kind != KindPlugin {
		return RegistryItem{}, false
	}
	name := strings.TrimSpace(doc.Name)
	if !registryNamePattern.MatchString(name) {
		return RegistryItem{}, false
	}
	if !validHTTPURL(doc.URL) {
		return RegistryItem{}, false
	}
	sum := strings.TrimSpace(doc.SHA256)
	if sum != "" {
		if !sha256HexPattern.MatchString(sum) {
			return RegistryItem{}, false
		}
		sum = strings.ToLower(sum)
	}
	return RegistryItem{
		Kind:        kind,
		Name:        name,
		Description: clipText(doc.Description, maxRegistryText),
		Version:     clipText(doc.Version, maxRegistryTag),
		URL:         doc.URL,
		SHA256:      sum,
		Tags:        cleanTags(doc.Tags),
	}, true
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != "" && u.User == nil
}

func cleanTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = clipText(strings.TrimSpace(t), maxRegistryTag)
		if t == "" {
			continue
		}
		out = append(out, t)
		if len(out) == maxRegistryTags {
			break
		}
	}
	return out
}

// clipText trims s and cuts it to at most n runes.
func clipText(s string, n int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}

// RegistryIndexURL returns the index location for a registry base. A value
// already ending in .json is used as is; anything else gets hub-index.json
// appended.
func RegistryIndexURL(base string) string {
	base = strings.TrimSpace(base)
	if strings.HasSuffix(strings.ToLower(base), ".json") {
		return base
	}
	return strings.TrimSuffix(base, "/") + "/" + registryIndexFile
}

// FetchRegistry downloads and parses an index through the SafeFetcher.
func FetchRegistry(ctx context.Context, f *SafeFetcher, indexURL string) (*Registry, error) {
	data, _, err := f.Get(ctx, indexURL, MaxRegistryBytes)
	if err != nil {
		return nil, err
	}
	return ParseRegistry(data)
}
