package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultAyrshareBaseURL is the production Ayrshare API.
const DefaultAyrshareBaseURL = "https://api.ayrshare.com"

// ayrshareCodeDuplicate is Ayrshare's "Status is a duplicate" code. The docs use it
// for a repeated status on X, a content duplicate. They do not say which code a
// repeated idempotencyKey returns, so code 110 is never read as "already posted".
const ayrshareCodeDuplicate = 110

// duplicateMessage replaces Ayrshare's text for code 110. The post may already be
// on the network, so the attempt is failed with a message that says so.
const duplicateMessage = "Ayrshare refused this post as a duplicate (code 110). An earlier post with the same content or key may already be on the network; check the channel before approving again."

// ErrNotConfigured means the workspace has no Ayrshare key. Nothing is sent.
var ErrNotConfigured = errors.New("ayrshare not configured")

// RequestError is a failed Ayrshare call. Retryable is true only for a network
// error or a 5xx answer; a 4xx answer is final. Status is zero when there was no
// HTTP status (a network error, or a 2xx answer that refused the post). Code is
// Ayrshare's error code when it gave one. Message is Ayrshare's own text, or a
// fixed description, with every key of the call removed.
type RequestError struct {
	Status    int
	Code      int
	Message   string
	Retryable bool
}

func (e *RequestError) Error() string {
	if e.Status == 0 {
		return "Ayrshare: " + e.Message
	}
	return fmt.Sprintf("Ayrshare answered %d: %s", e.Status, e.Message)
}

// PostResult is the answer to one successful post.
type PostResult struct {
	// ExternalID is the post id on the network, or Ayrshare's post id when the
	// post is scheduled.
	ExternalID string
	// ExternalURL is the post's public URL. Empty while the post is scheduled.
	ExternalURL string
	// Scheduled reports that Ayrshare accepted the post for a later time.
	Scheduled bool
}

// Credentials are one workspace's Ayrshare keys, decrypted for one call.
type Credentials struct {
	AyrshareKey string
	XAPIKey     string
	XAPISecret  string
}

// Client calls the Ayrshare API for one set of credentials. Its methods take the
// workspace's Profile-Key as an argument.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// XAPIKey and XAPISecret are the workspace's own X developer keys. Ayrshare
	// requires them for X posts from March 31, 2026. Empty means not set.
	XAPIKey    string
	XAPISecret string
}

// CreateProfile creates a User Profile titled title and returns its Profile-Key.
// Ayrshare returns the key only at creation.
func (c *Client) CreateProfile(ctx context.Context, title string) (profileKey, refID string, err error) {
	var out struct {
		Status     string `json:"status"`
		ProfileKey string `json:"profileKey"`
		RefID      string `json:"refId"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/profiles", "", map[string]any{"title": title}, nil, &out); err != nil {
		return "", "", err
	}
	if out.ProfileKey == "" {
		return "", "", &RequestError{Message: "profile created without a Profile-Key"}
	}
	return out.ProfileKey, out.RefID, nil
}

// LinkSession returns a URL the user opens to link social accounts to the
// profile. The body is empty, so the link keeps Ayrshare's default lifetime
// of five minutes; longer lifetimes need the Max Pack.
func (c *Client) LinkSession(ctx context.Context, profileKey string) (string, error) {
	var out struct {
		Status string `json:"status"`
		URL    string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/profiles/link-sessions", profileKey, map[string]any{}, nil, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", &RequestError{Message: "link session without a url"}
	}
	return out.URL, nil
}

// ActiveAccounts lists the networks the profile has linked, by Ayrshare name
// (for example "twitter", "linkedin", "facebook").
func (c *Client) ActiveAccounts(ctx context.Context, profileKey string) ([]string, error) {
	var out struct {
		ActiveSocialAccounts []string `json:"activeSocialAccounts"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/user", profileKey, nil, nil, &out); err != nil {
		return nil, err
	}
	return out.ActiveSocialAccounts, nil
}

// Post publishes one channel's post. The request body comes from
// PostRequest.Body and carries the idempotency key. A refusal as a duplicate
// (code 110) is a final error, never a success: the answer does not say whether
// this attempt or another post caused it.
func (c *Client) Post(ctx context.Context, profileKey string, body map[string]any, platform string) (PostResult, error) {
	name, err := ayrshareName(platform)
	if err != nil {
		return PostResult{}, err
	}
	var extra http.Header
	if platform == PlatformX && c.XAPIKey != "" && c.XAPISecret != "" {
		extra = http.Header{}
		extra.Set("X-Twitter-OAuth1-Api-Key", c.XAPIKey)
		extra.Set("X-Twitter-OAuth1-Api-Secret", c.XAPISecret)
	}
	var out struct {
		Status  string `json:"status"`
		ID      string `json:"id"`
		PostIDs []struct {
			Status   string `json:"status"`
			ID       string `json:"id"`
			PostURL  string `json:"postUrl"`
			Platform string `json:"platform"`
		} `json:"postIds"`
		Errors []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	err = c.do(ctx, http.MethodPost, "/api/post", profileKey, body, extra, &out)
	var re *RequestError
	if errors.As(err, &re) && re.Code == ayrshareCodeDuplicate {
		re.Message = duplicateMessage
		return PostResult{}, re
	}
	if err != nil {
		return PostResult{}, err
	}
	switch out.Status {
	case "scheduled":
		return PostResult{ExternalID: out.ID, Scheduled: true}, nil
	case "success":
		for _, p := range out.PostIDs {
			if p.Platform == name && p.Status == "success" {
				return PostResult{ExternalID: p.ID, ExternalURL: p.PostURL}, nil
			}
		}
		// The overall post succeeded but the platform's own entry is missing:
		// keep Ayrshare's id so the post can be found in its history.
		return PostResult{ExternalID: out.ID}, nil
	}
	msg := "post was not accepted"
	code := 0
	if len(out.Errors) > 0 {
		code = out.Errors[0].Code
		if out.Errors[0].Message != "" {
			msg = out.Errors[0].Message
		}
	}
	if code == ayrshareCodeDuplicate {
		msg = duplicateMessage
	}
	// Ayrshare answered 2xx but refused the post: final, not a transport fault.
	return PostResult{}, &RequestError{Code: code, Message: c.redact(msg, profileKey)}
}

// do sends one JSON request and decodes the answer into out. A 4xx answer and
// a 2xx answer that is not a success are final; a 5xx answer and a network error
// are retryable.
func (c *Client) do(ctx context.Context, method, path, profileKey string, body any, extra http.Header, out any) error {
	if c.APIKey == "" {
		return ErrNotConfigured
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("publish: encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
	if err != nil {
		return fmt.Errorf("publish: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if profileKey != "" {
		req.Header.Set("Profile-Key", profileKey)
	}
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return &RequestError{Message: "request failed", Retryable: true}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &RequestError{Status: resp.StatusCode, Message: "could not read the answer", Retryable: true}
	}
	if resp.StatusCode >= 500 {
		msg, code := errorText(raw)
		return &RequestError{Status: resp.StatusCode, Code: code, Message: c.redact(msg, profileKey), Retryable: true}
	}
	if resp.StatusCode >= 400 {
		msg, code := errorText(raw)
		return &RequestError{Status: resp.StatusCode, Code: code, Message: c.redact(msg, profileKey)}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return &RequestError{Status: resp.StatusCode, Message: "unreadable answer"}
		}
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// redact removes every key this client holds from an Ayrshare message, so a key
// echoed back in an error never reaches a log or a stored attempt.
func (c *Client) redact(msg string, profileKey string) string {
	for _, secret := range []string{c.APIKey, c.XAPIKey, c.XAPISecret, profileKey} {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[redacted]")
		}
	}
	return msg
}

// errorText takes Ayrshare's message and code from an error body.
func errorText(raw []byte) (string, int) {
	var e struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return "no message from Ayrshare", 0
	}
	if len(e.Errors) > 0 {
		if e.Errors[0].Message != "" {
			return e.Errors[0].Message, e.Errors[0].Code
		}
		return "no message from Ayrshare", e.Errors[0].Code
	}
	if e.Message != "" {
		return e.Message, e.Code
	}
	return "no message from Ayrshare", e.Code
}
