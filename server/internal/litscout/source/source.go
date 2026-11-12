// Package source reads scholarly APIs and returns paper.Metadata. Each source normalises its own
// response at this boundary, so nothing above it sees a source-specific shape.
package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/multica-ai/multica/server/internal/litscout/cache"
	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// Query is one search against one source.
type Query struct {
	Text     string
	FromYear int
	Limit    int
}

// Source is a paper search.
type Source interface {
	Name() string
	Search(ctx context.Context, q Query) ([]paper.Metadata, error)
}

// Allowed lists the metadata sources provenance accepts.
var Allowed = map[string]bool{"OpenAlex": true, "Crossref": true, "arXiv": true, "SemanticScholar": true}

var (
	// ErrQuotaExhausted means the source refused the request because the key's budget is spent
	// (OpenAlex answers HTTP 409 then). Retrying cannot help until the budget resets.
	ErrQuotaExhausted = errors.New("source quota exhausted")
	// ErrRateLimited means the source still answered HTTP 429 after the allowed retries.
	ErrRateLimited = errors.New("source rate limited")
)

// HTTPError is a non-retryable or exhausted HTTP failure. It never carries the request URL,
// which can hold a credential.
type HTTPError struct {
	Source string
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Source, e.Status, e.Body)
}

// Request is one GET. Params are part of the cache key; Secret and Headers are not.
type Request struct {
	Source  string
	Query   string
	URL     string
	Params  url.Values
	Secret  url.Values
	Headers map[string]string
}

// Client sends GETs with a finite retry, a minimum gap between requests, a body cap and a cache.
type Client struct {
	HTTP        *http.Client
	UserAgent   string
	MaxRetries  int           // retries after the first try; default 3
	RetryMin    time.Duration // first backoff; default 5s
	MaxBytes    int64         // body cap; default 8 MiB
	MinInterval time.Duration // gap between two requests of this client
	Cache       *cache.Cache
	Sleep       func(ctx context.Context, d time.Duration) error
	Rand        func() float64
	Now         func() time.Time

	mu   sync.Mutex
	last time.Time
}

// UserAgent identifies the tool to the APIs.
const UserAgent = "MulticaLitScout/1.0 (+https://multica.ai)"

func (c *Client) retries() int {
	if c.MaxRetries < 0 {
		return 0
	}
	if c.MaxRetries == 0 {
		return 3
	}
	return c.MaxRetries
}

func (c *Client) retryMin() time.Duration {
	if c.RetryMin <= 0 {
		return 5 * time.Second
	}
	return c.RetryMin
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) rnd() float64 {
	if c.Rand != nil {
		return c.Rand()
	}
	return rand.Float64()
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// ParseRetryAfter reads the header as seconds or as an HTTP date. It returns 0 when absent or invalid.
func ParseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
		return time.Duration(f * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

func flatten(v url.Values) map[string]string {
	out := make(map[string]string, len(v)+1)
	for k := range v {
		out[k] = v.Get(k)
	}
	return out
}

func (c *Client) wait(ctx context.Context) error {
	if c.MinInterval <= 0 {
		return nil
	}
	c.mu.Lock()
	var d time.Duration
	if !c.last.IsZero() {
		if gap := c.MinInterval - c.now().Sub(c.last); gap > 0 {
			d = gap
		}
	}
	c.last = c.now().Add(d)
	c.mu.Unlock()
	if d > 0 {
		return c.sleep(ctx, d)
	}
	return nil
}

// Get returns the response body of the request.
func (c *Client) Get(ctx context.Context, r Request) ([]byte, error) {
	params := flatten(r.Params)
	params["_url"] = r.URL
	if len(r.Secret) > 0 || len(r.Headers) > 0 {
		params["authenticated"] = "true"
	}
	key := cache.Key(r.Source, r.Query, params)
	if b, ok := c.Cache.Get(key); ok {
		return b, nil
	}
	full := url.Values{}
	for k, vs := range r.Params {
		full[k] = vs
	}
	for k, vs := range r.Secret {
		full[k] = vs
	}
	target := r.URL
	if len(full) > 0 {
		target += "?" + full.Encode()
	}
	maxBytes := c.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	var lastErr error
	for attempt := 0; attempt <= c.retries(); attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: build request", r.Source)
		}
		ua := c.UserAgent
		if ua == "" {
			ua = UserAgent
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "application/json, application/atom+xml")
		for k, v := range r.Headers {
			req.Header.Set(k, v)
		}
		resp, err := c.httpClient().Do(req)
		var delay time.Duration
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// A transport error can embed the URL; keep only its kind.
			lastErr = fmt.Errorf("%s: network error", r.Source)
			delay = c.retryMin() * time.Duration(1<<attempt)
		} else {
			body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
			resp.Body.Close()
			if rerr != nil {
				lastErr = fmt.Errorf("%s: read body", r.Source)
				delay = c.retryMin() * time.Duration(1<<attempt)
			} else if resp.StatusCode == http.StatusConflict && r.Source == "openalex" {
				return nil, fmt.Errorf("%w: %s", ErrQuotaExhausted, snippet(body))
			} else if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				lastErr = &HTTPError{Source: r.Source, Status: resp.StatusCode, Body: snippet(body)}
				if resp.StatusCode == http.StatusTooManyRequests {
					lastErr = fmt.Errorf("%w: %v", ErrRateLimited, lastErr)
				}
				delay = c.retryMin() * time.Duration(1<<attempt)
				if ra := ParseRetryAfter(resp.Header.Get("Retry-After"), c.now()); ra > delay {
					delay = ra
				}
			} else if resp.StatusCode >= 400 {
				return nil, &HTTPError{Source: r.Source, Status: resp.StatusCode, Body: snippet(body)}
			} else {
				_ = c.Cache.Set(key, body)
				return body, nil
			}
		}
		if attempt == c.retries() {
			break
		}
		delay += time.Duration(c.rnd() * float64(time.Second))
		if err := c.sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
