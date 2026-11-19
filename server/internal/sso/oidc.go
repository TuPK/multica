package sso

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/multica-ai/multica/server/internal/mfa"
)

// Errors a sign-in can end with. None of them carries text from a provider's
// token endpoint: that body can echo the client secret back.
var (
	ErrPendingInvalid   = errors.New("sso: pending login is missing, expired or altered")
	ErrState            = errors.New("sso: state does not match")
	ErrNonce            = errors.New("sso: nonce does not match")
	ErrToken            = errors.New("sso: id token rejected")
	ErrEmailNotVerified = errors.New("sso: email is not verified by the provider")
	ErrExchange         = errors.New("sso: code exchange failed")
	ErrDiscovery        = errors.New("sso: provider discovery failed")
)

const (
	// PendingCookie holds the sealed Pending between /auth/sso/start and the
	// callback.
	PendingCookie = "multica_sso"
	// PendingTTL is how long a started login may take to come back.
	PendingTTL = 10 * time.Minute

	amrPrefix = "sso:"
)

// Pending is a login the browser has started and not finished. It travels in a
// cookie sealed with the MFA key, so the browser can neither read the PKCE
// verifier nor change the state, nonce or connection.
type Pending struct {
	ConnectionID string `json:"c"`
	State        string `json:"s"`
	Nonce        string `json:"n"`
	Verifier     string `json:"v"`
	Next         string `json:"x"`
	ExpiresAt    int64  `json:"e"`
	// Mobile and Challenge mark a login the mobile app started: the callback
	// hands the session to the app, bound to the PKCE-style challenge.
	Mobile    bool   `json:"m,omitempty"`
	Challenge string `json:"k,omitempty"`
}

// Provider is what a connection holds, with the secret in the clear.
type Provider struct {
	Issuer       string
	ClientID     string
	ClientSecret string
}

// Identity is what a verified ID token says about the person.
type Identity struct {
	Subject string
	Email   string
}

// Client talks to identity providers. HTTP may be nil for the default client.
type Client struct{ HTTP *http.Client }

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewPending starts a login for connectionID that returns to next.
func NewPending(connectionID, next string, now time.Time) (Pending, error) {
	state, err := randomToken()
	if err != nil {
		return Pending{}, err
	}
	nonce, err := randomToken()
	if err != nil {
		return Pending{}, err
	}
	return Pending{
		ConnectionID: connectionID,
		State:        state,
		Nonce:        nonce,
		Verifier:     oauth2.GenerateVerifier(),
		Next:         next,
		ExpiresAt:    now.Add(PendingTTL).Unix(),
	}, nil
}

// SealPending returns the cookie value for p.
func SealPending(c *mfa.Crypto, p Pending) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sealed, err := c.Seal(string(raw))
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// OpenPending reverses SealPending. A value that is empty, altered, expired or
// not a pending login at all is ErrPendingInvalid.
func OpenPending(c *mfa.Crypto, value string, now time.Time) (Pending, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(sealed) == 0 {
		return Pending{}, ErrPendingInvalid
	}
	raw, err := c.Open(sealed)
	if err != nil {
		return Pending{}, ErrPendingInvalid
	}
	var p Pending
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Pending{}, ErrPendingInvalid
	}
	if p.ConnectionID == "" || p.State == "" || p.Nonce == "" || p.Verifier == "" || now.Unix() >= p.ExpiresAt {
		return Pending{}, ErrPendingInvalid
	}
	return p, nil
}

func (c Client) context(ctx context.Context) context.Context {
	if c.HTTP == nil {
		return ctx
	}
	return oidc.ClientContext(ctx, c.HTTP)
}

func (c Client) config(ctx context.Context, p Provider, redirectURI string) (*oidc.Provider, *oauth2.Config, error) {
	provider, err := oidc.NewProvider(ctx, p.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrDiscovery, err)
	}
	return provider, &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}, nil
}

// Discover checks that issuer serves OIDC discovery metadata.
func (c Client) Discover(ctx context.Context, issuer string) error {
	_, err := oidc.NewProvider(c.context(ctx), issuer)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDiscovery, err)
	}
	return nil
}

// AuthURL is where the browser goes to sign in at the provider: the
// authorization code flow with PKCE, the state and the nonce of pend.
func (c Client) AuthURL(ctx context.Context, p Provider, redirectURI string, pend Pending) (string, error) {
	_, conf, err := c.config(c.context(ctx), p, redirectURI)
	if err != nil {
		return "", err
	}
	return conf.AuthCodeURL(pend.State, oidc.Nonce(pend.Nonce), oauth2.S256ChallengeOption(pend.Verifier)), nil
}

// Complete finishes a login at the callback. It checks the state, exchanges
// the code with the PKCE verifier, and verifies the ID token: signature,
// issuer, audience, expiry and nonce. It requires a verified email. It does not
// decide whether the email may enter a workspace; the caller does.
func (c Client) Complete(ctx context.Context, p Provider, redirectURI, code, state string, pend Pending) (Identity, error) {
	if state == "" || pend.State == "" || subtle.ConstantTimeCompare([]byte(state), []byte(pend.State)) != 1 {
		return Identity{}, ErrState
	}
	if code == "" {
		return Identity{}, ErrExchange
	}
	ctx = c.context(ctx)
	provider, conf, err := c.config(ctx, p, redirectURI)
	if err != nil {
		return Identity{}, err
	}
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(pend.Verifier))
	if err != nil {
		// The error holds the provider's response body, which may echo the
		// secret. Report the failure and nothing of it.
		return Identity{}, ErrExchange
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return Identity{}, fmt.Errorf("%w: the token response has no id_token", ErrToken)
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: p.ClientID, SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256, oidc.PS256}}).Verify(ctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrToken, err)
	}
	if pend.Nonce == "" || subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(pend.Nonce)) != 1 {
		return Identity{}, ErrNonce
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("%w: claims: %v", ErrToken, err)
	}
	if !emailVerified(claims.EmailVerified) {
		return Identity{}, ErrEmailNotVerified
	}
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if email == "" || EmailDomain(email) == "" {
		return Identity{}, fmt.Errorf("%w: no usable email claim", ErrToken)
	}
	return Identity{Subject: idToken.Subject, Email: email}, nil
}

// emailVerified reads the email_verified claim. Some providers send the string
// "true" instead of a boolean; anything else, including absence, is false.
func emailVerified(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true"
	}
	return false
}

// AMR is the sign-in method a session gets from a connection.
func AMR(connectionID string) string { return amrPrefix + connectionID }

// HasAMR reports whether amr holds the method of connectionID.
func HasAMR(amr []string, connectionID string) bool {
	if connectionID == "" {
		return false
	}
	want := AMR(connectionID)
	for _, m := range amr {
		if m == want {
			return true
		}
	}
	return false
}

// EmailDomain returns the lower-cased text after the last @, or "".
func EmailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}
