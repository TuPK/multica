package lifecycle

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/config"
	"github.com/multica-ai/multica/platform-management/internal/mailer"
)

// The platform's mailer package sends only the sign-in code, in ASCII, with a
// fixed subject. The notices carry Vietnamese text and several recipients, so
// this sender takes the same provider settings and the same SMTP dialer and
// speaks the same two protocols with a UTF-8 message. It belongs next to
// internal/mailer, which this slice does not own.

const resendURL = "https://api.resend.com/emails"

// HTTPClient is the part of *http.Client the Resend path uses.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// MailSender is the Sender over the configured provider: SMTP when SMTP is
// configured, else Resend.
type MailSender struct {
	cfg    config.Email
	http   HTTPClient
	dialer mailer.SMTPDialer
	now    func() time.Time
	log    *slog.Logger
}

// NewMailSender returns a Sender for cfg.Provider. httpClient is used for Resend
// and dialer for SMTP; the one the provider does not use may be nil. now stamps
// the Date header. A nil logger means slog.Default.
func NewMailSender(cfg config.Email, httpClient HTTPClient, dialer mailer.SMTPDialer, now func() time.Time, log *slog.Logger) (*MailSender, error) {
	switch cfg.Provider {
	case config.ProviderResend:
		if httpClient == nil {
			return nil, errors.New("lifecycle: Resend needs an HTTP client")
		}
	case config.ProviderSMTP:
		if dialer == nil {
			return nil, errors.New("lifecycle: SMTP needs a dialer")
		}
	default:
		return nil, fmt.Errorf("lifecycle: unknown mail provider %q", cfg.Provider)
	}
	if now == nil {
		return nil, errors.New("lifecycle: the sender needs a clock")
	}
	if log == nil {
		log = slog.Default()
	}
	return &MailSender{cfg: cfg, http: httpClient, dialer: dialer, now: now, log: log}, nil
}

// check refuses what cannot go out as one safe message: no recipient, an
// address that is more than a plain address, or a subject with a line break.
func check(m Message) error {
	if len(m.To) == 0 {
		return errors.New("lifecycle: the message has no recipient")
	}
	for _, to := range m.To {
		if a, err := mail.ParseAddress(to); err != nil || a.Address != to || a.Name != "" {
			return errors.New("lifecycle: a recipient is not a plain email address")
		}
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		return errors.New("lifecycle: the subject spans lines")
	}
	return nil
}

// Send implements Sender. Its errors name neither a recipient nor a key.
func (s *MailSender) Send(ctx context.Context, m Message) error {
	if err := check(m); err != nil {
		return err
	}
	var err error
	if s.cfg.Provider == config.ProviderResend {
		err = s.sendResend(ctx, m)
	} else {
		err = s.sendSMTP(ctx, m)
	}
	if err != nil {
		return err
	}
	s.log.Info("lifecycle: notice sent", "provider", s.cfg.Provider, "recipients", len(m.To))
	return nil
}

func (s *MailSender) sendResend(ctx context.Context, m Message) error {
	payload, err := json.Marshal(map[string]any{
		"from": s.cfg.From, "to": m.To, "subject": m.Subject, "text": m.Body,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("resend request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("resend answered status %d", resp.StatusCode)
	}
	return nil
}

// smtpMessage builds the message: headers in ASCII, the subject as an RFC 2047
// encoded word, and the body as UTF-8 text in base64 lines.
func (s *MailSender) smtpMessage(m Message) (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	body := strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(body))
	var b strings.Builder
	for _, h := range [][2]string{
		{"From", s.cfg.From},
		{"To", strings.Join(m.To, ", ")},
		{"Subject", mime.BEncoding.Encode("UTF-8", m.Subject)},
		{"Date", s.now().UTC().Format(time.RFC1123Z)},
		{"Message-ID", "<" + hex.EncodeToString(id[:]) + "@" + s.cfg.SMTPHost + ">"},
		{"MIME-Version", "1.0"},
		{"Content-Type", "text/plain; charset=UTF-8"},
		{"Content-Transfer-Encoding", "base64"},
	} {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	b.WriteString("\r\n")
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.String(), nil
}

func (s *MailSender) sendSMTP(ctx context.Context, m Message) (err error) {
	msg, err := s.smtpMessage(m)
	if err != nil {
		return err
	}
	sess, err := s.dialer.Dial(ctx)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer func() { sess.Close() }()

	if s.cfg.SMTPUsername != "" {
		useLogin, authErr := authWithFallback(sess, s.cfg.SMTPHost, s.cfg.SMTPUsername, s.cfg.SMTPPassword)
		if authErr != nil {
			if !useLogin {
				return fmt.Errorf("smtp auth: %w", authErr)
			}
			// LOGIN is a new handshake: the first connection cannot retry it.
			sess.Close()
			if sess, err = s.dialer.Dial(ctx); err != nil {
				return fmt.Errorf("smtp dial for LOGIN: %w", err)
			}
			login := &loginAuth{username: s.cfg.SMTPUsername, password: s.cfg.SMTPPassword, host: s.cfg.SMTPHost}
			if err := sess.Auth(login); err != nil {
				return fmt.Errorf("smtp auth: LOGIN fallback: %w", err)
			}
		}
	}
	if err := sess.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, to := range m.To {
		if err := sess.Rcpt(to); err != nil {
			return errors.New("smtp RCPT TO was refused")
		}
	}
	w, err := sess.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := io.WriteString(w, msg); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp end data: %w", err)
	}
	return sess.Quit()
}

// authWithFallback tries PLAIN. It reports useLogin when the server rejected
// PLAIN as unsupported and advertises LOGIN.
func authWithFallback(s mailer.SMTPSession, host, username, password string) (useLogin bool, err error) {
	plainErr := s.Auth(smtp.PlainAuth("", username, password, host))
	if plainErr == nil {
		return false, nil
	}
	msg := strings.ToLower(plainErr.Error())
	if !strings.Contains(msg, "unrecognized authentication type") && !strings.Contains(msg, "504 5.7.4") {
		return false, plainErr
	}
	ok, line := s.Extension("AUTH")
	if !ok || !strings.Contains(strings.ToUpper(line), "LOGIN") {
		return false, plainErr
	}
	return true, plainErr
}

// loginAuth is the LOGIN mechanism, for relays that do not offer PLAIN.
type loginAuth struct{ username, password, host string }

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	local := server.Name == "localhost" || server.Name == "127.0.0.1" || server.Name == "::1"
	if !server.TLS && !local {
		return "", nil, errors.New("unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, fmt.Errorf("wrong host name: %q does not match expected %q", server.Name, a.host)
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	raw := strings.TrimSpace(string(fromServer))
	challenge := strings.ToLower(raw)
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
		challenge = strings.ToLower(strings.TrimSpace(string(decoded)))
	}
	switch {
	case strings.Contains(challenge, "username") || strings.Contains(challenge, "user name"):
		return []byte(a.username), nil
	case strings.Contains(challenge, "password"):
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected LOGIN challenge %q", raw)
}
