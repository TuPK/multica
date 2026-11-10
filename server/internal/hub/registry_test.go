package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func indexJSON(t *testing.T, schema any, items ...any) []byte {
	t.Helper()
	doc := map[string]any{"schema": schema, "name": "Acme Hub", "items": items}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func goodItem(name string) map[string]any {
	return map[string]any{
		"kind":        "skill",
		"name":        name,
		"description": "does things",
		"version":     "1.0.0",
		"url":         "https://example.com/" + name + ".zip",
		"sha256":      strings.Repeat("a", 64),
		"tags":        []string{"ops", "  ", "deploy"},
	}
}

func TestParseRegistry(t *testing.T) {
	t.Run("valid index", func(t *testing.T) {
		reg, err := ParseRegistry(indexJSON(t, 1, goodItem("deploy"), map[string]any{
			"kind": "plugin", "name": "hello", "url": "http://example.com/h.zip",
		}))
		if err != nil {
			t.Fatalf("ParseRegistry: %v", err)
		}
		if reg.Name != "Acme Hub" || len(reg.Items) != 2 || reg.Skipped != 0 || reg.Truncated {
			t.Fatalf("registry = %+v", reg)
		}
		first := reg.Items[0]
		if first.Kind != KindSkill || first.Name != "deploy" || len(first.Tags) != 2 || first.Tags[0] != "ops" {
			t.Fatalf("item = %+v (blank tags must be dropped)", first)
		}
		if ref := first.Ref(); ref.Type != SourceRegistry || ref.URL != first.URL || ref.SHA256 != first.SHA256 {
			t.Fatalf("Ref() = %+v", ref)
		}
	})

	fatal := []struct {
		name string
		data []byte
	}{
		{"not JSON", []byte("<html>")},
		{"schema 2", indexJSON(t, 2, goodItem("a"))},
		{"schema missing", []byte(`{"items":[]}`)},
		{"schema as string", []byte(`{"schema":"1","items":[]}`)},
		{"items missing", []byte(`{"schema":1}`)},
	}
	for _, tc := range fatal {
		t.Run("rejected: "+tc.name, func(t *testing.T) {
			if _, err := ParseRegistry(tc.data); !errors.Is(err, ErrInvalidRegistry) {
				t.Fatalf("err = %v, want ErrInvalidRegistry", err)
			}
		})
	}

	skipped := []struct {
		name string
		item any
	}{
		{"unknown kind", map[string]any{"kind": "app", "name": "x", "url": "https://e.com/x.zip"}},
		{"name with slash", map[string]any{"kind": "skill", "name": "a/b", "url": "https://e.com/x.zip"}},
		{"name starting with dot", map[string]any{"kind": "skill", "name": ".hidden", "url": "https://e.com/x.zip"}},
		{"javascript URL", map[string]any{"kind": "skill", "name": "x", "url": "javascript:alert(1)"}},
		{"file URL", map[string]any{"kind": "skill", "name": "x", "url": "file:///etc/passwd"}},
		{"URL with userinfo", map[string]any{"kind": "skill", "name": "x", "url": "https://u:p@e.com/x.zip"}},
		{"short sha", map[string]any{"kind": "skill", "name": "x", "url": "https://e.com/x.zip", "sha256": "abc"}},
		{"non-hex sha", map[string]any{"kind": "skill", "name": "x", "url": "https://e.com/x.zip", "sha256": strings.Repeat("g", 64)}},
		{"not an object", "just a string"},
	}
	for _, tc := range skipped {
		t.Run("skipped: "+tc.name, func(t *testing.T) {
			reg, err := ParseRegistry(indexJSON(t, 1, goodItem("keep"), tc.item))
			if err != nil {
				t.Fatalf("ParseRegistry: %v", err)
			}
			if len(reg.Items) != 1 || reg.Skipped != 1 || reg.Items[0].Name != "keep" {
				t.Fatalf("items=%d skipped=%d", len(reg.Items), reg.Skipped)
			}
		})
	}

	t.Run("duplicate kind+name skipped, same name different kind kept", func(t *testing.T) {
		plugin := goodItem("deploy")
		plugin["kind"] = "plugin"
		reg, err := ParseRegistry(indexJSON(t, 1, goodItem("deploy"), goodItem("deploy"), plugin))
		if err != nil {
			t.Fatalf("ParseRegistry: %v", err)
		}
		if len(reg.Items) != 2 || reg.Skipped != 1 {
			t.Fatalf("items=%d skipped=%d", len(reg.Items), reg.Skipped)
		}
	})

	t.Run("sha256 normalized to lowercase", func(t *testing.T) {
		item := goodItem("upper")
		item["sha256"] = strings.Repeat("AB", 32)
		reg, err := ParseRegistry(indexJSON(t, 1, item))
		if err != nil {
			t.Fatalf("ParseRegistry: %v", err)
		}
		if reg.Items[0].SHA256 != strings.Repeat("ab", 32) {
			t.Fatalf("sha = %q", reg.Items[0].SHA256)
		}
	})

	t.Run("items capped at 500 and flagged", func(t *testing.T) {
		var items []any
		for i := 0; i < MaxRegistryItems+25; i++ {
			items = append(items, goodItem(fmt.Sprintf("item-%04d", i)))
		}
		reg, err := ParseRegistry(indexJSON(t, 1, items...))
		if err != nil {
			t.Fatalf("ParseRegistry: %v", err)
		}
		if len(reg.Items) != MaxRegistryItems || !reg.Truncated {
			t.Fatalf("items=%d truncated=%v", len(reg.Items), reg.Truncated)
		}
	})

	t.Run("oversized index refused", func(t *testing.T) {
		big := []byte(`{"schema":1,"items":[],"name":"` + strings.Repeat("x", MaxRegistryBytes) + `"}`)
		if _, err := ParseRegistry(big); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("err = %v, want ErrTooLarge", err)
		}
	})

	t.Run("long fields clipped", func(t *testing.T) {
		item := goodItem("clip")
		item["description"] = strings.Repeat("d", maxRegistryText+50)
		reg, err := ParseRegistry(indexJSON(t, 1, item))
		if err != nil {
			t.Fatalf("ParseRegistry: %v", err)
		}
		if got := len([]rune(reg.Items[0].Description)); got != maxRegistryText {
			t.Fatalf("description runes = %d, want %d", got, maxRegistryText)
		}
	})
}

func TestRegistryIndexURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com/hub":            "https://example.com/hub/hub-index.json",
		"https://example.com/hub/":           "https://example.com/hub/hub-index.json",
		"https://example.com/hub/index.json": "https://example.com/hub/index.json",
		"https://example.com/HUB.JSON":       "https://example.com/HUB.JSON",
		"  https://example.com/hub  ":        "https://example.com/hub/hub-index.json",
	}
	for in, want := range cases {
		if got := RegistryIndexURL(in); got != want {
			t.Errorf("RegistryIndexURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFetchRegistry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hub/hub-index.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(indexJSON(t, 1, goodItem("fetched")))
	}))
	defer srv.Close()

	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
	reg, err := FetchRegistry(context.Background(), f, RegistryIndexURL(srv.URL+"/hub"))
	if err != nil {
		t.Fatalf("FetchRegistry: %v", err)
	}
	if len(reg.Items) != 1 || reg.Items[0].Name != "fetched" {
		t.Fatalf("registry = %+v", reg)
	}

	if _, err := FetchRegistry(context.Background(), f, srv.URL+"/nope/hub-index.json"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing index err = %v, want ErrNotFound", err)
	}
	blocked := NewSafeFetcher(FetcherOptions{})
	if _, err := FetchRegistry(context.Background(), blocked, srv.URL+"/hub/hub-index.json"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("loopback index err = %v, want ErrBlockedAddress", err)
	}
}
