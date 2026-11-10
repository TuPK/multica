package salescollect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Engine renders a page in a browser and returns its HTML. The Web engine (Crawl4AI, spec 4.13)
// is the only implementation; a recipe that needs a browser render is refused when no engine is
// configured.
type Engine interface {
	HTML(ctx context.Context, rawURL string) (*Response, error)
	Status(ctx context.Context) WebEngineStatus
}

// DefaultEngineURL is where the runbook starts the Crawl4AI container.
const DefaultEngineURL = "http://127.0.0.1:11235"

// Crawl4AI is the Web engine client. It talks to a container that listens on loopback only and
// requires the token the runbook generates; the token is never written to a recipe.
type Crawl4AI struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// NewCrawl4AI validates the engine's address: it must be loopback, because an engine that other
// machines can reach could be used to read internal services through a squad's URLs.
func NewCrawl4AI(baseURL, token string) (*Crawl4AI, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("web engine url %q is not an http address", baseURL)
	}
	ip := net.ParseIP(u.Hostname())
	if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("web engine must listen on loopback, got host %q", u.Hostname())
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("web engine token is empty")
	}
	return &Crawl4AI{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Client:  &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

// engineHTMLTimeout bounds one page, the runbook's per-page limit.
const engineHTMLTimeout = 45 * time.Second

// crawlReply is the body the engine returns for /crawl. The raw html keeps the attributes a
// recipe reads, such as a link's href; /html returns a cleaned page without them.
type crawlReply struct {
	Success bool `json:"success"`
	Results []struct {
		Success      bool   `json:"success"`
		HTML         string `json:"html"`
		StatusCode   int    `json:"status_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"results"`
}

// HTML asks the engine to render one page and returns its raw HTML. The reply is read with the
// same size cap as plain HTTP.
func (c *Crawl4AI) HTML(ctx context.Context, rawURL string) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, engineHTMLTimeout)
	defer cancel()
	payload, err := json.Marshal(map[string][]string{"urls": {rawURL}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/crawl", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web engine: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, defaultMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("web engine read: %w", err)
	}
	if int64(len(body)) > defaultMaxBytes {
		return nil, fmt.Errorf("web engine: reply is larger than %d bytes", defaultMaxBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web engine answered %d: %s", resp.StatusCode, engineDetail(body))
	}
	var reply crawlReply
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, fmt.Errorf("web engine: unreadable reply: %w", err)
	}
	if !reply.Success || len(reply.Results) != 1 {
		return nil, errors.New("web engine returned no result for the page")
	}
	got := reply.Results[0]
	if !got.Success {
		return nil, fmt.Errorf("web engine could not render the page: %s", got.ErrorMessage)
	}
	return &Response{Status: got.StatusCode, ContentType: "text/html", Body: []byte(got.HTML), FinalURL: rawURL}, nil
}

// engineDetail reads the message of an error reply, which the engine sends as {"detail": "..."}.
func engineDetail(body []byte) string {
	var e struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(body, &e) == nil && e.Detail != "" {
		return e.Detail
	}
	return "no detail"
}

// Status reports whether the engine answers its health endpoint, with the version it reports. An
// unreachable engine is "not_running", not an error.
func (c *Crawl4AI) Status(ctx context.Context) WebEngineStatus {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return WebEngineStatus{Status: "not_running"}
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.Client.Do(req)
	if err != nil {
		return WebEngineStatus{Status: "not_running"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return WebEngineStatus{Status: "not_running"}
	}
	var health struct {
		Version string `json:"version"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&health)
	return WebEngineStatus{Status: "running", Version: health.Version}
}
