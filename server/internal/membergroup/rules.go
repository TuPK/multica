package membergroup

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// MaxNameLength and MaxDescriptionLength count characters, not bytes.
	MaxNameLength        = 64
	MaxDescriptionLength = 280
	// MaxMembers is the most people one group holds.
	MaxMembers = 1000
	// MaxAddPerRequest is the most people one add-members request names.
	MaxAddPerRequest = 100
)

var (
	ErrNameInvalid        = errors.New("group name must be 1 to 64 characters")
	ErrDescriptionInvalid = errors.New("group description must be at most 280 characters")
)

// UnknownPermissionError reports a key that is not in the catalogue.
type UnknownPermissionError struct{ Key string }

func (e *UnknownPermissionError) Error() string {
	return fmt.Sprintf("unknown permission %q", e.Key)
}

// NormalizeName trims name and checks its length. Whether it is free in the
// workspace is the store's question: the database index decides, ignoring case.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxNameLength {
		return "", ErrNameInvalid
	}
	return name, nil
}

// NormalizeDescription trims description and checks its length. It may be empty.
func NormalizeDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(description) > MaxDescriptionLength {
		return "", ErrDescriptionInvalid
	}
	return description, nil
}

// NormalizePermissions checks every key against the catalogue and returns the
// set without repeats, in catalogue order. The result is never nil, so it is
// stored as [] and not as null.
func NormalizePermissions(keys []string) ([]string, error) {
	have := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !IsKnown(key) {
			return nil, &UnknownPermissionError{Key: key}
		}
		have[key] = true
	}
	out := make([]string, 0, len(have))
	for _, key := range catalogue {
		if have[key] {
			out = append(out, key)
		}
	}
	return out, nil
}
