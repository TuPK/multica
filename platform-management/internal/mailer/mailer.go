// Package mailer sends the sign-in code by email. It duplicates the transport
// choices of the Multica server's email service (spec 15.3): SMTP when
// SMTP_HOST is set, otherwise Resend. The code is never logged or placed in an
// error; logs carry the email hash only.
package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/config"
	"github.com/multica-ai/multica/platform-management/internal/emailhash"
)

const (
	resendURL = "https://api.resend.com/emails"
	subject   = "Your Multica platform sign-in code"
)

// HTTPClient is the part of *http.Client the Resend path uses.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// SMTPSession is one open SMTP connection after the TLS and EHLO steps.
// *smtp.Client satisfies it.
type SMTPSession interface {
	Auth(smtp.Auth) error
	Extension(name string) (bool, string)
	Mail(from string) error
	Rcpt(to string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

// SMTPDialer opens SMTP sessions.
type SMTPDialer interface {
	Dial(ctx context.Context) (SMTPSession, error)
}

// Mailer sends sign-in codes through the configured provider.
type Mailer struct {
	cfg    config.Email
	http   HTTPClient
	dialer SMTPDialer
	log    *slog.Logger
}

// New returns a Mailer for cfg.Provider. httpClient is used for Resend and
// dialer for SMTP; the one the provider does not use may be nil. A nil logger
// means slog.Default.
func New(cfg config.Email, httpClient HTTPClient, dialer SMTPDialer, logger *slog.Logger) (*Mailer, error) {
	switch cfg.Provider {
	case config.ProviderResend:
		if httpClient == nil {
			return nil, errors.New("mailer: Resend needs an HTTP client")
		}
	case config.ProviderSMTP:
		if dialer == nil {
			return nil, errors.New("mailer: SMTP needs a dialer")
		}
	default:
		return nil, fmt.Errorf("mailer: unknown provider %q", cfg.Provider)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Mailer{cfg: cfg, http: httpClient, dialer: dialer, log: logger}, nil
}

// SendCode emails the code to the address, which must be a plain address.
func (m *Mailer) SendCode(ctx context.Context, to, code string) error {
	if addr, err := mail.ParseAddress(to); err != nil || addr.Address != to || addr.Name != "" {
		return errors.New("mailer: recipient is not a plain email address")
	}
	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 400px; margin: 0 auto;">
<h2>Your sign-in code</h2>
<p style="font-size: 32px; font-weight: bold; letter-spacing: 8px; margin: 24px 0;">%s</p>
<p>This code expires in 10 minutes.</p>
<p style="color: #666; font-size: 14px;">If you did not request this code, you can ignore this email.</p>
</div>`, code)

	var err error
	switch m.cfg.Provider {
	case config.ProviderResend:
		err = m.sendResend(ctx, to, body)
	default:
		err = m.sendSMTP(ctx, to, body)
	}
	if err != nil {
		m.log.Warn("sign-in code was not sent", "provider", m.cfg.Provider, "email_hash", emailhash.Of(to), "error", err)
		return err
	}
	m.log.Info("sign-in code sent", "provider", m.cfg.Provider, "email_hash", emailhash.Of(to))
	return nil
}

func (m *Mailer) sendResend(ctx context.Context, to, htmlBody string) error {
	payload, err := json.Marshal(map[string]any{
		"from":    m.cfg.From,
		"to":      []string{to},
		"subject": subject,
		"html":    htmlBody,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.http.Do(req)
	if err != nil {
		// The transport error can quote the request URL; it holds no key or code.
		return fmt.Errorf("resend request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("resend answered status %d", resp.StatusCode)
	}
	return nil
}

func (m *Mailer) sendSMTP(ctx context.Context, to, htmlBody string) (err error) {
	s, err := m.dialer.Dial(ctx)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer func() { s.Close() }()

	if m.cfg.SMTPUsername != "" {
		useLogin, authErr := authWithFallback(s, m.cfg.SMTPHost, m.cfg.SMTPUsername, m.cfg.SMTPPassword)
		if authErr != nil {
			if !useLogin {
				return fmt.Errorf("smtp auth: %w", authErr)
			}
			// LOGIN is a new handshake: the first connection cannot retry it.
			s.Close()
			if s, err = m.dialer.Dial(ctx); err != nil {
				return fmt.Errorf("smtp dial for LOGIN: %w", err)
			}
			login := &loginAuth{username: m.cfg.SMTPUsername, password: m.cfg.SMTPPassword, host: m.cfg.SMTPHost}
			if err := s.Auth(login); err != nil {
				return fmt.Errorf("smtp auth: LOGIN fallback: %w", err)
			}
		}
	}

	if err := s.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := s.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	w, err := s.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	// The message is ASCII, so no transfer encoding is needed.
	msg := "From: " + m.cfg.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n" +
		"Message-ID: <" + fmt.Sprint(time.Now().UnixNano()) + "@" + m.cfg.SMTPHost + ">\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 7bit\r\n" +
		"\r\n" + strings.ReplaceAll(htmlBody, "\n", "\r\n") + "\r\n"
	if _, err := io.WriteString(w, msg); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp end data: %w", err)
	}
	return s.Quit()
}

// authWithFallback tries PLAIN. It reports useLogin when the server rejected
// PLAIN as unsupported and advertises LOGIN.
func authWithFallback(s SMTPSession, host, username, password string) (useLogin bool, err error) {
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
