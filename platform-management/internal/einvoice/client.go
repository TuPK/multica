// Package einvoice is the SePay e-invoice provider (spec 7.3). It implements
// invoice.Provider over SePay's REST API.
package einvoice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/invoice"
)

const (
	maxBody       = 1 << 20
	tokenSkew     = 60 * time.Second
	codeExpired   = "TOKEN_EXPIRED"
	pathToken     = "/v1/token"
	badResponse   = "bad_response"
	errNetwork    = "network"
	errTimeout    = "timeout"
	errCanceled   = "canceled"
	httpCodeStart = "http_"
)

// apiError is a ProviderError that also keeps the HTTP status, for callers that
// branch on 404 or 409. Its text is the ProviderError's: code and kind only.
type apiError struct {
	*invoice.ProviderError
	status int
}

func (e *apiError) Unwrap() error { return e.ProviderError }

func providerErr(kind invoice.Kind, code string) *invoice.ProviderError {
	return &invoice.ProviderError{Kind: kind, Code: code}
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code string `json:"code"`
	} `json:"error"`
}

// do sends an authorized JSON request and decodes the data of the answer into
// out (when not nil). An expired token is dropped and the request is sent once more.
func (s *SePay) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return providerErr(invoice.KindPermanent, "encode_failed")
		}
		payload = b
	}
	for attempt := 0; ; attempt++ {
		tok, err := s.accessToken(ctx)
		if err != nil {
			return err
		}
		status, raw, err := s.send(ctx, method, path, "Bearer "+tok, payload)
		if err != nil {
			return err
		}
		env, derr := decodeEnvelope(raw)
		if attempt == 0 && (status == http.StatusUnauthorized || (derr == nil && env.Error != nil && env.Error.Code == codeExpired)) {
			s.dropToken(tok)
			continue
		}
		return finish(status, env, derr, out)
	}
}

func decodeEnvelope(raw []byte) (envelope, error) {
	var env envelope
	err := json.Unmarshal(raw, &env)
	return env, err
}

// finish turns a decoded answer into an error or the decoded data. It never
// puts the response body or message in an error.
func finish(status int, env envelope, derr error, out any) error {
	if status < 200 || status > 299 {
		code := httpCodeStart + strconv.Itoa(status)
		if derr == nil && env.Error != nil && env.Error.Code != "" {
			code = env.Error.Code
		}
		kind := invoice.KindPermanent
		if status == http.StatusTooManyRequests || status >= 500 {
			kind = invoice.KindTransient
		}
		return &apiError{providerErr(kind, code), status}
	}
	if derr != nil || !env.Success {
		return &apiError{providerErr(invoice.KindTransient, badResponse), status}
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return &apiError{providerErr(invoice.KindTransient, badResponse), status}
		}
	}
	return nil
}

func (s *SePay) send(ctx context.Context, method, path, auth string, payload []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.cfg.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, providerErr(invoice.KindPermanent, "bad_request")
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return 0, nil, transportErr(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return 0, nil, transportErr(err)
	}
	return resp.StatusCode, raw, nil
}

// transportErr drops the underlying error: its text can carry the URL.
func transportErr(err error) error {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return providerErr(invoice.KindTransient, errTimeout)
	}
	if errors.Is(err, context.Canceled) {
		return providerErr(invoice.KindTransient, errCanceled)
	}
	return providerErr(invoice.KindTransient, errNetwork)
}

// accessToken returns the cached token, fetching a new one when it is missing
// or within a minute of expiry.
func (s *SePay) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && s.cfg.Now().Before(s.expires) {
		return s.token, nil
	}
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(s.cfg.APIKey))
	status, raw, err := s.send(ctx, http.MethodPost, pathToken, auth, nil)
	if err != nil {
		return "", err
	}
	env, derr := decodeEnvelope(raw)
	var data struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := finish(status, env, derr, &data); err != nil {
		return "", err
	}
	if data.AccessToken == "" {
		return "", &apiError{providerErr(invoice.KindTransient, badResponse), status}
	}
	s.token = data.AccessToken
	s.expires = s.cfg.Now().Add(time.Duration(data.ExpiresIn)*time.Second - tokenSkew)
	return s.token, nil
}

// dropToken forgets tok unless another request has already replaced it.
func (s *SePay) dropToken(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == tok {
		s.token = ""
	}
}

func isStatus(err error, status int) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.status == status
}

func errorCode(err error) string {
	var pe *invoice.ProviderError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}
