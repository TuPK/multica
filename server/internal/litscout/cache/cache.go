// Package cache is a small TTL cache of raw API responses, one file per key. It is request
// data, never run state. A key and a value never contain an API key: callers pass the
// request parameters without credentials.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Cache stores responses under Dir. A nil *Cache is a cache that never hits.
type Cache struct {
	Dir string
	TTL time.Duration
	Now func() time.Time
}

// DefaultDir is <user cache dir>/multica/litscout.
func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "multica", "litscout"), nil
}

// Key hashes the source, the query and the request parameters (without credentials).
func Key(source, query string, params map[string]string) string {
	canonical, _ := json.Marshal([]any{source, query, params}) // map keys marshal in sorted order
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func (c *Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Cache) path(key string) string { return filepath.Join(c.Dir, key+".json") }

// Get returns the stored value, or false when it is missing or older than the TTL. An expired
// entry is removed.
func (c *Cache) Get(key string) ([]byte, bool) {
	if c == nil || c.Dir == "" || c.TTL <= 0 {
		return nil, false
	}
	p := c.path(key)
	info, err := os.Stat(p)
	if err != nil {
		return nil, false
	}
	if c.now().Sub(info.ModTime()) > c.TTL {
		_ = os.Remove(p)
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	return b, true
}

// Set writes the value atomically (temp file, then rename).
func (c *Cache) Set(key string, value []byte) error {
	if c == nil || c.Dir == "" || c.TTL <= 0 {
		return nil
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.Dir, "tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(value); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, c.path(key)); err != nil {
		os.Remove(name)
		return errors.Join(err)
	}
	return nil
}
