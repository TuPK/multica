package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// GetBytes performs a GET and returns the body as it was sent. The Open Company
// Format package is a zip, which GetJSON cannot read.
func (c *APIClient) GetBytes(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	err = wrapTransport(req, err)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, newHTTPError(http.MethodGet, path, resp)
	}
	body, err := io.ReadAll(resp.Body)
	return body, wrapBodyRead(req, err)
}

// PostBytes posts body as contentType and decodes the JSON answer into out.
func (c *APIClient) PostBytes(ctx context.Context, path, contentType string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	c.setHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	err = wrapTransport(req, err)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return newHTTPError(http.MethodPost, path, resp)
	}
	if out == nil {
		return nil
	}
	return wrapBodyRead(req, json.NewDecoder(resp.Body).Decode(out))
}
