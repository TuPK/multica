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

// crawlHTMLReply is the body the engine returns for /html.
type crawlHTMLReply struct {
	Success    bool   `json:"success"`
	HTML       string `json:"html"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error_message"`
}

// HTML asks the engine to render one page and returns its HTML. The engine's answer is
// read with the same size cap as plain HTTP.
func (c *Crawl4AI) HTML(ctx context.Context, rawURL string) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, engineHTMLTimeout)
	defer cancel()
	payload, err := json.Marshal(map[string]string{"url": rawURL})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/html", bytes.NewReader(payload))
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
		return nil, fmt.Errorf("web engine: page is larger than %d bytes", defaultMaxBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web engine answered %d", resp.StatusCode)
	}
	var reply crawlHTMLReply
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, fmt.Errorf("web engine: unreadable reply: %w", err)
	}
	if !reply.Success {
		return nil, fmt.Errorf("web engine could not render the page: %s", reply.Error)
	}
	return &Response{Status: reply.StatusCode, ContentType: "text/html", Body: []byte(reply.HTML), FinalURL: rawURL}, nil
}

// Status reports whether the engine answers its health endpoint. A missing token or a
// non-loopback address is "not_running" with the reason in the error, not a crash.
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
	return WebEngineStatus{Status: "running"}
}
