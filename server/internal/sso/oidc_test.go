package sso

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/sso/ssotest"
)

const testRedirect = "https://api.example.test/auth/sso/callback"

func testCrypto(t *testing.T) *mfa.Crypto {
	t.Helper()
	t.Setenv(mfa.EnvKey, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	c, err := mfa.LoadCrypto()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// login drives one code flow against the fake provider and returns the pieces
// Complete needs: the pending login, and the code and state the provider sent
// back to the callback.
func login(t *testing.T, idp *ssotest.IdP) (Client, Provider, Pending, string, string) {
	t.Helper()
	client := Client{HTTP: idp.HTTPClient()}
	prov := Provider{Issuer: idp.Issuer(), ClientID: idp.ClientID, ClientSecret: idp.ClientSecret}
	pend, err := NewPending("conn-1", "/ws/issues", time.Now())
	if err != nil {
		t.Fatalf("NewPending: %v", err)
	}
	authURL, err := client.AuthURL(context.Background(), prov, testRedirect, pend)
	if err != nil {
		t.Fatalf("AuthURL: %v", err)
	}
	cb, err := url.Parse(idp.Authorize(t, authURL))
	if err != nil {
		t.Fatal(err)
	}
	return client, prov, pend, cb.Query().Get("code"), cb.Query().Get("state")
}

func TestSSOCompleteReturnsTheVerifiedIdentity(t *testing.T) {
	idp := ssotest.New(t)
	idp.Email = "Jane@Example.Test"
	client, prov, pend, code, state := login(t, idp)

	id, err := client.Complete(context.Background(), prov, testRedirect, code, state, pend)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if id.Email != "jane@example.test" || id.Subject != idp.Subject {
		t.Fatalf("identity = %+v, want email jane@example.test and subject %q", id, idp.Subject)
	}
}

func TestSSOCompleteRefusesWhatAC17Lists(t *testing.T) {
	cases := []struct {
		name string
		set  func(*ssotest.IdP)
		want error
	}{
		{"wrong nonce", func(p *ssotest.IdP) { p.WrongNonce = true }, ErrNonce},
		{"bad signature", func(p *ssotest.IdP) { p.BadSignature = true }, ErrToken},
		{"email not verified", func(p *ssotest.IdP) { p.EmailVerified = false }, ErrEmailNotVerified},
		{"wrong audience", func(p *ssotest.IdP) { p.WrongAudience = true }, ErrToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idp := ssotest.New(t)
			tc.set(idp)
			client, prov, pend, code, state := login(t, idp)
			_, err := client.Complete(context.Background(), prov, testRedirect, code, state, pend)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Complete error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSSOCompleteRefusesAWrongState(t *testing.T) {
	idp := ssotest.New(t)
	client, prov, pend, code, _ := login(t, idp)
	_, err := client.Complete(context.Background(), prov, testRedirect, code, "state-of-another-login", pend)
	if !errors.Is(err, ErrState) {
		t.Fatalf("Complete error = %v, want ErrState", err)
	}
	_, err = client.Complete(context.Background(), prov, testRedirect, code, "", pend)
	if !errors.Is(err, ErrState) {
		t.Fatalf("empty state: error = %v, want ErrState", err)
	}
}

func TestSSOCompleteSendsThePKCEVerifier(t *testing.T) {
	idp := ssotest.New(t)
	client, prov, pend, code, state := login(t, idp)
	pend.Verifier = "a-verifier-that-does-not-match-the-challenge-0000000000"
	if _, err := client.Complete(context.Background(), prov, testRedirect, code, state, pend); !errors.Is(err, ErrExchange) {
		t.Fatalf("Complete error = %v, want ErrExchange", err)
	}
}

func TestSSOExchangeErrorDoesNotCarryTheClientSecret(t *testing.T) {
	idp := ssotest.New(t)
	idp.TokenError = true
	client, prov, pend, code, state := login(t, idp)
	_, err := client.Complete(context.Background(), prov, testRedirect, code, state, pend)
	if err == nil {
		t.Fatal("Complete succeeded against a failing token endpoint")
	}
	if strings.Contains(err.Error(), idp.ClientSecret) {
		t.Fatalf("error text holds the client secret: %v", err)
	}
}

func TestSSOPendingSurvivesTheCookie(t *testing.T) {
	c := testCrypto(t)
	now := time.Now()
	p, err := NewPending("conn-1", "/ws/issues", now)
	if err != nil {
		t.Fatal(err)
	}
	if p.State == "" || p.Nonce == "" || p.Verifier == "" || p.State == p.Nonce {
		t.Fatalf("pending lacks fresh random values: %+v", p)
	}
	sealed, err := SealPending(c, p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, p.State) || strings.Contains(sealed, p.Verifier) {
		t.Fatal("the cookie value shows the state or the verifier")
	}
	got, err := OpenPending(c, sealed, now.Add(time.Minute))
	if err != nil || got != p {
		t.Fatalf("OpenPending = %+v, %v; want %+v", got, err, p)
	}
	if _, err := OpenPending(c, sealed, now.Add(PendingTTL+time.Second)); !errors.Is(err, ErrPendingInvalid) {
		t.Fatalf("expired pending: error = %v, want ErrPendingInvalid", err)
	}
	if _, err := OpenPending(c, sealed[:len(sealed)-4]+"AAAA", now); !errors.Is(err, ErrPendingInvalid) {
		t.Fatalf("altered pending: error = %v, want ErrPendingInvalid", err)
	}
	if _, err := OpenPending(c, "", now); !errors.Is(err, ErrPendingInvalid) {
		t.Fatalf("empty pending: error = %v, want ErrPendingInvalid", err)
	}
}

func TestSSOAMRNamesTheConnection(t *testing.T) {
	if got := AMR("c1"); got != "sso:c1" {
		t.Fatalf("AMR = %q, want sso:c1", got)
	}
	if !HasAMR([]string{"otp", "sso:c1"}, "c1") || HasAMR([]string{"sso:c2"}, "c1") || HasAMR([]string{"otp"}, "c1") || HasAMR([]string{"sso:c1"}, "") {
		t.Fatal("HasAMR does not match exactly the method of the given connection")
	}
}

func TestSSOEmailDomain(t *testing.T) {
	for in, want := range map[string]string{"a@Example.COM": "example.com", "a@b@c.test": "c.test", "nodomain": "", "a@": "", "": ""} {
		if got := EmailDomain(in); got != want {
			t.Errorf("EmailDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
