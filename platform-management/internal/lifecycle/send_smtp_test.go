package lifecycle

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/config"
	"github.com/multica-ai/multica/platform-management/internal/mailer"
)

type fakeSession struct {
	authErrs   []error // the result of each Auth call, in order
	extension  string
	from       string
	rcpts      []string
	data       bytes.Buffer
	authCalls  int
	quitCalled bool
}

func (s *fakeSession) Auth(smtp.Auth) error {
	s.authCalls++
	if len(s.authErrs) == 0 {
		return nil
	}
	err := s.authErrs[0]
	s.authErrs = s.authErrs[1:]
	return err
}
func (s *fakeSession) Extension(string) (bool, string) { return s.extension != "", s.extension }
func (s *fakeSession) Mail(from string) error          { s.from = from; return nil }
func (s *fakeSession) Rcpt(to string) error            { s.rcpts = append(s.rcpts, to); return nil }
func (s *fakeSession) Data() (io.WriteCloser, error)   { return nopCloser{&s.data}, nil }
func (s *fakeSession) Quit() error                     { s.quitCalled = true; return nil }
func (s *fakeSession) Close() error                    { return nil }

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

type fakeDialer struct {
	sessions []*fakeSession
	dials    int
}

func (d *fakeDialer) Dial(context.Context) (mailer.SMTPSession, error) {
	s := d.sessions[d.dials]
	d.dials++
	return s, nil
}

func smtpConfig(username string) config.Email {
	return config.Email{
		Provider: config.ProviderSMTP, From: "billing@example.com",
		SMTPHost: "mail.example.com", SMTPUsername: username, SMTPPassword: "pw",
	}
}

func TestSenderSMTPMessageIsUTF8AndEncoded(t *testing.T) {
	sess := &fakeSession{}
	s, err := NewMailSender(smtpConfig("user"), nil, &fakeDialer{sessions: []*fakeSession{sess}}, sendNow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), vietnamese); err != nil {
		t.Fatal(err)
	}
	if sess.from != "billing@example.com" || strings.Join(sess.rcpts, ",") != "owner@example.com,admin@example.com" || !sess.quitCalled {
		t.Errorf("session = from %q, rcpts %v, quit %v", sess.from, sess.rcpts, sess.quitCalled)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(sess.data.Bytes()))
	if err != nil {
		t.Fatalf("not a message: %v\n%s", err, sess.data.String())
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subject != vietnamese.Subject {
		t.Errorf("subject = %q, %v; want %q", subject, err, vietnamese.Subject)
	}
	if strings.ContainsFunc(msg.Header.Get("Subject"), func(r rune) bool { return r > 127 }) {
		t.Errorf("the subject header holds raw non-ASCII: %q", msg.Header.Get("Subject"))
	}
	if ct := msg.Header.Get("Content-Type"); ct != "text/plain; charset=UTF-8" {
		t.Errorf("content type = %q", ct)
	}
	if msg.Header.Get("Content-Transfer-Encoding") != "base64" || msg.Header.Get("MIME-Version") != "1.0" {
		t.Errorf("transfer encoding or MIME version missing: %v", msg.Header)
	}
	raw, _ := io.ReadAll(msg.Body)
	body, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(raw), "\r\n", ""))
	if err != nil || string(body) != vietnamese.Body {
		t.Errorf("body = %q, %v; want %q", body, err, vietnamese.Body)
	}
	for _, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 76 {
			t.Errorf("body line of %d characters", len(line))
		}
	}
	for _, h := range []string{"To", "Date", "Message-Id"} {
		if msg.Header.Get(h) == "" {
			t.Errorf("header %s is missing: %v", h, msg.Header)
		}
	}
	if msg.Header.Get("From") != "billing@example.com" || sess.authCalls != 1 {
		t.Errorf("From %q, auth calls %d", msg.Header.Get("From"), sess.authCalls)
	}
}

func TestSenderSMTPWithoutUsernameDoesNotAuthenticate(t *testing.T) {
	sess := &fakeSession{}
	s, _ := NewMailSender(smtpConfig(""), nil, &fakeDialer{sessions: []*fakeSession{sess}}, sendNow, nil)
	if err := s.Send(context.Background(), vietnamese); err != nil {
		t.Fatal(err)
	}
	if sess.authCalls != 0 {
		t.Errorf("auth calls = %d, want 0", sess.authCalls)
	}
}

// A relay that refuses PLAIN and offers LOGIN gets a second connection.
func TestSenderSMTPFallsBackToLogin(t *testing.T) {
	first := &fakeSession{authErrs: []error{errors.New("504 5.7.4 Unrecognized authentication type")}, extension: "PLAIN LOGIN"}
	second := &fakeSession{}
	d := &fakeDialer{sessions: []*fakeSession{first, second}}
	s, _ := NewMailSender(smtpConfig("user"), nil, d, sendNow, nil)
	if err := s.Send(context.Background(), vietnamese); err != nil {
		t.Fatal(err)
	}
	if d.dials != 2 || second.authCalls != 1 || len(second.rcpts) != 2 || len(first.rcpts) != 0 {
		t.Errorf("dials %d, second auth calls %d, second rcpts %v, first rcpts %v", d.dials, second.authCalls, second.rcpts, first.rcpts)
	}
}

func TestSenderRefusesWhatItCannotSendSafely(t *testing.T) {
	sess := &fakeSession{}
	s, _ := NewMailSender(smtpConfig(""), nil, &fakeDialer{sessions: []*fakeSession{sess, sess, sess, sess}}, sendNow, nil)
	for name, m := range map[string]Message{
		"no recipient":      {Subject: "s", Body: "b"},
		"display name":      {To: []string{"Boss <boss@example.com>"}, Subject: "s", Body: "b"},
		"header injection":  {To: []string{"a@example.com\r\nBcc: b@example.com"}, Subject: "s", Body: "b"},
		"multi-line header": {To: []string{"a@example.com"}, Subject: "s\r\nBcc: b@example.com", Body: "b"},
	} {
		if err := s.Send(context.Background(), m); err == nil {
			t.Errorf("%s: sent", name)
		}
	}
	if len(sess.rcpts) != 0 {
		t.Errorf("recipients reached the server: %v", sess.rcpts)
	}
}

func TestNewMailSenderChecksTheProvider(t *testing.T) {
	if _, err := NewMailSender(config.Email{Provider: config.ProviderResend}, nil, nil, sendNow, nil); err == nil {
		t.Error("Resend without an HTTP client")
	}
	if _, err := NewMailSender(config.Email{Provider: config.ProviderSMTP}, nil, nil, sendNow, nil); err == nil {
		t.Error("SMTP without a dialer")
	}
	if _, err := NewMailSender(config.Email{Provider: "carrier-pigeon"}, nil, nil, sendNow, nil); err == nil {
		t.Error("unknown provider")
	}
	if _, err := NewMailSender(resendConfig(), &fakeHTTP{}, nil, nil, nil); err == nil {
		t.Error("no clock")
	}
}
