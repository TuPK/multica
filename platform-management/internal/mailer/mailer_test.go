package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/smtp"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/config"
	"github.com/multica-ai/multica/platform-management/internal/emailhash"
)

const (
	code     = "483920"
	apiKey   = "re_secret_key_value"
	password = "smtp-password-value"
	to       = "staff@example.com"
)

type fakeHTTP struct {
	req    *http.Request
	body   []byte
	status int
	err    error
}

func (f *fakeHTTP) Do(r *http.Request) (*http.Response, error) {
	f.req = r
	if r.Body != nil {
		f.body, _ = io.ReadAll(r.Body)
	}
	if f.err != nil {
		return nil, f.err
	}
	status := f.status
	if status == 0 {
		status = 200
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"id":"x"}`))}, nil
}

type fakeSession struct {
	authErrs   []error // returned by successive Auth calls
	authCalls  int
	authValues []smtp.Auth
	authLine   string
	from       string
	rcpt       []string
	data       bytes.Buffer
	quit       bool
	closed     bool
	failRcpt   error
}

func (s *fakeSession) Auth(a smtp.Auth) error {
	s.authValues = append(s.authValues, a)
	i := s.authCalls
	s.authCalls++
	if i < len(s.authErrs) {
		return s.authErrs[i]
	}
	return nil
}
func (s *fakeSession) Extension(name string) (bool, string) {
	if name == "AUTH" {
		return s.authLine != "", s.authLine
	}
	return false, ""
}
func (s *fakeSession) Mail(from string) error { s.from = from; return nil }
func (s *fakeSession) Rcpt(r string) error {
	if s.failRcpt != nil {
		return s.failRcpt
	}
	s.rcpt = append(s.rcpt, r)
	return nil
}
func (s *fakeSession) Data() (io.WriteCloser, error) { return nopCloser{&s.data}, nil }
func (s *fakeSession) Quit() error                   { s.quit = true; return nil }
func (s *fakeSession) Close() error                  { s.closed = true; return nil }

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

type fakeDialer struct {
	sessions []*fakeSession
	dials    int
	err      error
}

func (d *fakeDialer) Dial(context.Context) (SMTPSession, error) {
	if d.err != nil {
		return nil, d.err
	}
	i := d.dials
	d.dials++
	if i >= len(d.sessions) {
		return nil, errors.New("no more fake sessions")
	}
	return d.sessions[i], nil
}

func resendConfig() config.Email {
	return config.Email{Provider: config.ProviderResend, From: "platform@example.com", ResendAPIKey: apiKey}
}

func smtpConfig() config.Email {
	return config.Email{Provider: config.ProviderSMTP, From: "platform@example.com", SMTPHost: "smtp.example.com", SMTPPort: "587", SMTPUsername: "mailer", SMTPPassword: password}
}

func capture() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestResendRequest(t *testing.T) {
	h := &fakeHTTP{}
	logger, _ := capture()
	m, err := New(resendConfig(), h, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SendCode(context.Background(), to, code); err != nil {
		t.Fatalf("SendCode: %v", err)
	}
	if h.req.Method != http.MethodPost {
		t.Errorf("method = %s", h.req.Method)
	}
	if got := h.req.URL.String(); got != "https://api.resend.com/emails" {
		t.Errorf("url = %s", got)
	}
	if got := h.req.Header.Get("Authorization"); got != "Bearer "+apiKey {
		t.Errorf("Authorization = %q", got)
	}
	if got := h.req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	var body struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
	}
	if err := json.Unmarshal(h.body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body.From != "platform@example.com" || len(body.To) != 1 || body.To[0] != to {
		t.Errorf("from/to = %q %v", body.From, body.To)
	}
	if body.Subject == "" || !strings.Contains(body.HTML, code) {
		t.Errorf("subject %q, html lacks the code", body.Subject)
	}
}

func TestResendFailureIsAnErrorWithoutSecrets(t *testing.T) {
	for name, h := range map[string]*fakeHTTP{
		"status":    {status: 422},
		"transport": {err: errors.New("connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			logger, buf := capture()
			m, _ := New(resendConfig(), h, nil, logger)
			err := m.SendCode(context.Background(), to, code)
			if err == nil {
				t.Fatal("SendCode succeeded")
			}
			for _, secret := range []string{code, apiKey, to} {
				if strings.Contains(err.Error(), secret) || strings.Contains(buf.String(), secret) {
					t.Errorf("error or log contains %q", secret)
				}
			}
		})
	}
}

func TestSMTPSend(t *testing.T) {
	s := &fakeSession{}
	d := &fakeDialer{sessions: []*fakeSession{s}}
	logger, _ := capture()
	m, err := New(smtpConfig(), nil, d, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SendCode(context.Background(), to, code); err != nil {
		t.Fatalf("SendCode: %v", err)
	}
	if s.from != "platform@example.com" || len(s.rcpt) != 1 || s.rcpt[0] != to {
		t.Errorf("from=%q rcpt=%v", s.from, s.rcpt)
	}
	if s.authCalls != 1 {
		t.Errorf("Auth called %d times, want 1", s.authCalls)
	}
	msg := s.data.String()
	if !strings.Contains(msg, "To: "+to) || !strings.Contains(msg, "Subject:") {
		t.Errorf("message headers: %q", msg)
	}
	if !strings.Contains(msg, code) {
		t.Errorf("message lacks the code: %q", msg)
	}
	if !s.quit || !s.closed {
		t.Errorf("quit=%v closed=%v", s.quit, s.closed)
	}
}

func TestSMTPWithoutUsernameSkipsAuth(t *testing.T) {
	cfg := smtpConfig()
	cfg.SMTPUsername, cfg.SMTPPassword = "", ""
	s := &fakeSession{}
	m, _ := New(cfg, nil, &fakeDialer{sessions: []*fakeSession{s}}, nil)
	if err := m.SendCode(context.Background(), to, code); err != nil {
		t.Fatal(err)
	}
	if s.authCalls != 0 {
		t.Errorf("Auth called %d times", s.authCalls)
	}
}

func TestSMTPFallsBackToLoginAuth(t *testing.T) {
	first := &fakeSession{authErrs: []error{errors.New("504 5.7.4 Unrecognized authentication type")}, authLine: "PLAIN LOGIN"}
	second := &fakeSession{}
	d := &fakeDialer{sessions: []*fakeSession{first, second}}
	m, _ := New(smtpConfig(), nil, d, nil)
	if err := m.SendCode(context.Background(), to, code); err != nil {
		t.Fatalf("SendCode: %v", err)
	}
	if d.dials != 2 || !first.closed {
		t.Errorf("dials=%d firstClosed=%v", d.dials, first.closed)
	}
	if second.authCalls != 1 || !strings.Contains(second.data.String(), code) {
		t.Errorf("second session: auth=%d data=%q", second.authCalls, second.data.String())
	}
}

func TestSMTPAuthFailureIsNotRetried(t *testing.T) {
	s := &fakeSession{authErrs: []error{errors.New("535 bad credentials")}, authLine: "PLAIN LOGIN"}
	d := &fakeDialer{sessions: []*fakeSession{s}}
	m, _ := New(smtpConfig(), nil, d, nil)
	if err := m.SendCode(context.Background(), to, code); err == nil {
		t.Fatal("SendCode succeeded")
	}
	if d.dials != 1 {
		t.Errorf("dials = %d", d.dials)
	}
}

func TestSMTPFailuresAreErrorsWithoutSecrets(t *testing.T) {
	cases := map[string]*fakeDialer{
		"dial": {err: errors.New("dial tcp: connection refused")},
		"rcpt": {sessions: []*fakeSession{{failRcpt: errors.New("550 no such user")}}},
		"auth": {sessions: []*fakeSession{{authErrs: []error{errors.New("535 rejected")}}}},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			logger, buf := capture()
			m, _ := New(smtpConfig(), nil, d, logger)
			err := m.SendCode(context.Background(), to, code)
			if err == nil {
				t.Fatal("SendCode succeeded")
			}
			for _, secret := range []string{code, password, to} {
				if strings.Contains(err.Error(), secret) || strings.Contains(buf.String(), secret) {
					t.Errorf("error or log contains %q", secret)
				}
			}
		})
	}
}

func TestSendLogsOnlyTheEmailHash(t *testing.T) {
	t.Run("resend", func(t *testing.T) {
		logger, buf := capture()
		m, _ := New(resendConfig(), &fakeHTTP{}, nil, logger)
		if err := m.SendCode(context.Background(), to, code); err != nil {
			t.Fatal(err)
		}
		assertLogClean(t, buf.String(), apiKey)
	})
	t.Run("smtp", func(t *testing.T) {
		logger, buf := capture()
		m, _ := New(smtpConfig(), nil, &fakeDialer{sessions: []*fakeSession{{}}}, logger)
		if err := m.SendCode(context.Background(), to, code); err != nil {
			t.Fatal(err)
		}
		assertLogClean(t, buf.String(), password)
	})
}

func assertLogClean(t *testing.T, log, secret string) {
	t.Helper()
	if log == "" {
		t.Fatal("nothing was logged; the test would prove nothing")
	}
	for _, s := range []string{code, secret, to} {
		if strings.Contains(log, s) {
			t.Errorf("log contains %q: %s", s, log)
		}
	}
	if !strings.Contains(log, emailhash.Of(to)) {
		t.Errorf("log lacks the email hash: %s", log)
	}
}

func TestSendRefusesAnAddressThatIsNotPlain(t *testing.T) {
	h := &fakeHTTP{}
	m, _ := New(resendConfig(), h, nil, nil)
	for _, bad := range []string{"", "a@b.c\r\nBcc: x@y.z", "Name <a@b.c>", "not-an-address"} {
		if err := m.SendCode(context.Background(), bad, code); err == nil {
			t.Errorf("SendCode(%q) succeeded", bad)
		}
	}
	if h.req != nil {
		t.Error("a request was sent")
	}
}

func TestNewRejectsAnUnknownProvider(t *testing.T) {
	if _, err := New(config.Email{Provider: "carrier-pigeon"}, nil, nil, nil); err == nil {
		t.Fatal("New accepted an unknown provider")
	}
}
