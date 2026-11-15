// Package consent holds the pure rules for legal-document consent: which
// documents are required, which the user still owes, and whether an accept
// request names the current versions. It has no database or HTTP code.
package consent

import (
	"errors"
	"strings"
)

// Document names, in the order they are listed to the user.
const (
	DocTerms   = "terms"
	DocPrivacy = "privacy"
)

var (
	// ErrInvalidDocument means an item names a document other than terms or privacy.
	ErrInvalidDocument = errors.New("consent: invalid document")
	// ErrVersionMismatch means an item's version is not the current one.
	ErrVersionMismatch = errors.New("consent: version is not current")
)

// Document is one required document at its current version.
type Document struct {
	Name    string
	Version string
	URL     string
}

// Required is the wire form of a document the user still has to accept.
type Required struct {
	Document string `json:"document"`
	Version  string `json:"version"`
	URL      string `json:"url"`
}

// Acceptance is a recorded (document, version) pair.
type Acceptance struct {
	Document string
	Version  string
}

// Item is one entry of an accept request.
type Item struct {
	Document string
	Version  string
}

// Config is the set of documents currently required.
type Config struct {
	docs []Document
}

// FromEnv reads LEGAL_TERMS_VERSION, LEGAL_TERMS_URL, LEGAL_PRIVACY_VERSION and
// LEGAL_PRIVACY_URL. A document with an empty version is not required.
func FromEnv(getenv func(string) string) Config {
	var c Config
	for _, d := range []struct{ name, prefix string }{
		{DocTerms, "LEGAL_TERMS_"},
		{DocPrivacy, "LEGAL_PRIVACY_"},
	} {
		version := strings.TrimSpace(getenv(d.prefix + "VERSION"))
		if version == "" {
			continue
		}
		c.docs = append(c.docs, Document{
			Name:    d.name,
			Version: version,
			URL:     strings.TrimSpace(getenv(d.prefix + "URL")),
		})
	}
	return c
}

// Documents returns the required documents.
func (c Config) Documents() []Document { return append([]Document(nil), c.docs...) }

// Pending returns the required documents the user has not accepted at the
// current version. The result is empty, never nil.
func (c Config) Pending(accepted []Acceptance) []Required {
	done := make(map[Acceptance]bool, len(accepted))
	for _, a := range accepted {
		done[a] = true
	}
	out := []Required{}
	for _, d := range c.docs {
		if !done[Acceptance{Document: d.Name, Version: d.Version}] {
			out = append(out, Required{Document: d.Name, Version: d.Version, URL: d.URL})
		}
	}
	return out
}

func validName(name string) bool { return name == DocTerms || name == DocPrivacy }

// Validate checks an accept request. An unknown document name wins over a
// version mismatch, so the caller can answer 400 before 409.
func (c Config) Validate(items []Item) error {
	for _, it := range items {
		if !validName(it.Document) {
			return ErrInvalidDocument
		}
	}
	for _, it := range items {
		current := false
		for _, d := range c.docs {
			if d.Name == it.Document && d.Version == it.Version {
				current = true
				break
			}
		}
		if !current {
			return ErrVersionMismatch
		}
	}
	return nil
}
