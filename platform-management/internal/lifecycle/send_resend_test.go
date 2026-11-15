package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/config"
)

var sendNow = func() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) }

var vietnamese = Message{
	To:      []string{"owner@example.com", "admin@example.com"},
	Subject: "Your plan has ended / Gói đã hết hạn",
	Body:    "Xin chào,\r\nGói Pro của workspace đã hết hạn.\r\n",
}

type fakeHTTP struct {
	req    *http.Request
	body   []byte
	status int
	err    error
}

func (h *fakeHTTP) Do(r *http.Request) (*http.Response, error) {
	h.req = r
	h.body, _ = io.ReadAll(r.Body)
	if h.err != nil {
		return nil, h.err
	}
	return &http.Response{StatusCode: h.status, Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func resendConfig() config.Email {
	return config.Email{Provider: config.ProviderResend, From: "billing@example.com", ResendAPIKey: "re_secret"}
}

func TestSenderResendPayload(t *testing.T) {
	h := &fakeHTTP{status: 200}
	s, err := NewMailSender(resendConfig(), h, nil, sendNow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), vietnamese); err != nil {
		t.Fatal(err)
	}
	if h.req.URL.String() != "https://api.resend.com/emails" || h.req.Header.Get("Authorization") != "Bearer re_secret" {
		t.Errorf("request = %s, auth %q", h.req.URL, h.req.Header.Get("Authorization"))
	}
	var got struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
	}
	if err := json.Unmarshal(h.body, &got); err != nil {
		t.Fatal(err)
	}
	if got.From != "billing@example.com" || len(got.To) != 2 || got.Subject != vietnamese.Subject || got.Text != vietnamese.Body {
		t.Errorf("payload = %+v", got)
	}
}

func TestSenderResendFailureDoesNotLeakTheKeyOrTheRecipients(t *testing.T) {
	for name, h := range map[string]*fakeHTTP{
		"status":    {status: 500},
		"transport": {err: errors.New("dial tcp: connection refused")},
	} {
		s, _ := NewMailSender(resendConfig(), h, nil, sendNow, nil)
		err := s.Send(context.Background(), vietnamese)
		if err == nil {
			t.Fatalf("%s: no error", name)
		}
		for _, secret := range []string{"re_secret", "owner@example.com"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("%s: error %q holds %q", name, err, secret)
			}
		}
	}
}
