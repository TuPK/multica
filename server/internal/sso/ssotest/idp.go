// Package ssotest is a fake OpenID Connect identity provider for tests. It runs
// in the test process behind an httptest server and serves discovery, a JWKS
// and the authorization and token endpoints, so a test drives the whole code
// flow without the network.
package ssotest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const keyID = "fake-idp-key"

// IdP is the fake provider. The exported fields are knobs a test sets before
// it drives a login; each makes the next ID tokens differ from a good one.
type IdP struct {
	ClientID     string
	ClientSecret string

	Subject       string
	Email         string
	EmailVerified bool

	// WrongNonce puts a nonce into the ID token that the client did not send.
	WrongNonce bool
	// BadSignature signs the ID token with a key the JWKS does not publish.
	BadSignature bool
	// WrongAudience issues the token for another client.
	WrongAudience bool
	// TokenError makes the token endpoint answer 400 with the secret it was
	// sent echoed in the body, as a careless provider might.
	TokenError bool

	srv    *httptest.Server
	key    *rsa.PrivateKey
	other  *rsa.PrivateKey
	mu     sync.Mutex
	grants map[string]grant
}

type grant struct {
	nonce     string
	challenge string
	redirect  string
}

// New starts the provider and stops it with the test.
func New(t *testing.T) *IdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &IdP{
		ClientID:      "client-" + uuid.NewString()[:8],
		ClientSecret:  "planted-secret-" + uuid.NewString(),
		Subject:       "subject-" + uuid.NewString()[:8],
		Email:         "person@example.test",
		EmailVerified: true,
		key:           key,
		other:         other,
		grants:        map[string]grant{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("/jwks", p.jwks)
	mux.HandleFunc("/authorize", p.authorize)
	mux.HandleFunc("/token", p.token)
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

// Issuer is the issuer URL a connection stores.
func (p *IdP) Issuer() string { return p.srv.URL }

// HTTPClient is a client that reaches the provider.
func (p *IdP) HTTPClient() *http.Client { return p.srv.Client() }

// Authorize plays the browser: it opens the authorization URL the server built
// and returns the callback URL the provider redirects to, with code and state.
func (p *IdP) Authorize(t *testing.T, authURL string) string {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(authURL)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("authorize answered %d, want 302", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

func (p *IdP) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                p.srv.URL,
		"authorization_endpoint":                p.srv.URL + "/authorize",
		"token_endpoint":                        p.srv.URL + "/token",
		"jwks_uri":                              p.srv.URL + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (p *IdP) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := p.key.PublicKey
	writeJSON(w, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": keyID,
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (p *IdP) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != p.ClientID || q.Get("response_type") != "code" || q.Get("state") == "" || q.Get("nonce") == "" ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		http.Error(w, "bad authorization request", http.StatusBadRequest)
		return
	}
	code := uuid.NewString()
	p.mu.Lock()
	p.grants[code] = grant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri")}
	p.mu.Unlock()
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		http.Error(w, "bad redirect_uri", http.StatusBadRequest)
		return
	}
	v := back.Query()
	v.Set("code", code)
	v.Set("state", q.Get("state"))
	back.RawQuery = v.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

func (p *IdP) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if p.TokenError {
		http.Error(w, "invalid_client: "+secret, http.StatusBadRequest)
		return
	}
	if id != p.ClientID || secret != p.ClientSecret {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	p.mu.Lock()
	g, ok := p.grants[r.PostForm.Get("code")]
	delete(p.grants, r.PostForm.Get("code"))
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || g.challenge != base64.RawURLEncoding.EncodeToString(sum[:]) || g.redirect != r.PostForm.Get("redirect_uri") {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	nonce := g.nonce
	if p.WrongNonce {
		nonce = "not-the-nonce-the-client-sent"
	}
	aud := p.ClientID
	if p.WrongAudience {
		aud = "some-other-client"
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": p.srv.URL, "sub": p.Subject, "aud": aud,
		"iat": now.Unix(), "exp": now.Add(10 * time.Minute).Unix(),
		"nonce": nonce, "email": p.Email, "email_verified": p.EmailVerified,
	})
	tok.Header["kid"] = keyID
	signer := p.key
	if p.BadSignature {
		signer = p.other
	}
	idToken, err := tok.SignedString(signer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"access_token": "access-" + uuid.NewString(), "token_type": "Bearer", "expires_in": 600, "id_token": idToken})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
