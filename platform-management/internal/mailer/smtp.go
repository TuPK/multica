package mailer

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/config"
)

type netDialer struct{ cfg config.Email }

// NewSMTPDialer returns the dialer that connects to the configured relay: an
// immediate TLS handshake for implicit TLS, otherwise STARTTLS when the relay
// advertises it.
func NewSMTPDialer(cfg config.Email) SMTPDialer { return netDialer{cfg: cfg} }

func (d netDialer) Dial(ctx context.Context) (SMTPSession, error) {
	addr := net.JoinHostPort(d.cfg.SMTPHost, d.cfg.SMTPPort)
	tlsCfg := &tls.Config{
		ServerName:         d.cfg.SMTPHost,
		InsecureSkipVerify: d.cfg.SMTPTLSInsecure, //nolint:gosec // opt-in through SMTP_TLS_INSECURE, as in the Multica server
	}

	var conn net.Conn
	var err error
	if d.cfg.SMTPTLSImplicit {
		conn, err = (&tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	c, err := smtp.NewClient(conn, d.cfg.SMTPHost)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("client: %w", err)
	}

	// net/smtp says "localhost" in EHLO unless told otherwise, and strict relays
	// refuse that from a public address.
	ehlo := d.cfg.SMTPEHLOName
	if ehlo == "" {
		ehlo, _ = os.Hostname()
	}
	if ehlo != "" {
		if err := c.Hello(ehlo); err != nil {
			c.Close()
			return nil, fmt.Errorf("EHLO: %w", err)
		}
	}
	if !d.cfg.SMTPTLSImplicit {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				c.Close()
				return nil, fmt.Errorf("STARTTLS: %w", err)
			}
		}
	}
	return c, nil
}

// loginAuth is the LOGIN mechanism, for relays that do not offer PLAIN.
type loginAuth struct {
	username, password, host string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !isLocalhost(server.Name) {
		return "", nil, fmt.Errorf("unencrypted connection")
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
	default:
		return nil, fmt.Errorf("unexpected LOGIN challenge %q", raw)
	}
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}
