package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMFAPendingTokenRoundTrip(t *testing.T) {
	now := time.Now()
	token, pending, err := IssueMFAPendingToken("user-1", []string{AMROTP}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if pending.TokenID == "" {
		t.Fatal("pending token has no id; attempts cannot be counted per token")
	}

	got, err := ParseMFAPendingToken(token, now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.UserID != "user-1" || got.TokenID != pending.TokenID || strings.Join(got.AMR, ",") != AMROTP {
		t.Fatalf("parsed %+v, want user-1 / %s / [otp]", got, pending.TokenID)
	}
}

func TestMFAPendingTokenLivesFiveMinutes(t *testing.T) {
	now := time.Now()
	token, _, err := IssueMFAPendingToken("user-1", []string{AMROTP}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ParseMFAPendingToken(token, now.Add(5*time.Minute-time.Second)); err != nil {
		t.Fatalf("token refused just before 5 minutes: %v", err)
	}
	if _, err := ParseMFAPendingToken(token, now.Add(5*time.Minute+time.Second)); err == nil {
		t.Fatal("token accepted after 5 minutes")
	}
}

// The pending token must not read as a session anywhere a session is parsed,
// even by code that does not know about the audience.
func TestMFAPendingTokenIsNotASession(t *testing.T) {
	now := time.Now()
	token, _, err := IssueMFAPendingToken("user-1", []string{AMROTP}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ParseSessionToken(token); err == nil {
		t.Fatal("ParseSessionToken accepted a pending token")
	}

	// The same claims with a `sub` (as a session carries) are still refused:
	// the audience alone is enough.
	forged := signSession(t, jwt.MapClaims{
		"sub": "user-1",
		"aud": MFAPendingAudience,
		"exp": float64(now.Add(time.Hour).Unix()),
	})
	if _, err := ParseSessionToken(forged); err == nil {
		t.Fatal("ParseSessionToken accepted a token with aud=mfa_pending")
	}
}

func TestMFAPendingParseRefusesSessionTokens(t *testing.T) {
	session := signSession(t, sessionClaims(t, time.Now().Add(time.Hour), "sid-1"))
	if _, err := ParseMFAPendingToken(session, time.Now()); err == nil {
		t.Fatal("a session token was accepted as a pending token")
	}
}

func TestMFAPendingTokenCarriesNoSessionIdentity(t *testing.T) {
	token, _, err := IssueMFAPendingToken("user-1", []string{AMRGoogle}, time.Now())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse unverified: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	// Parsers that read `sub` (WebSocket, daemon auth) must find nothing.
	if _, present := claims["sub"]; present {
		t.Fatal("pending token carries a sub claim")
	}
	if _, present := claims[AuthTimeClaim]; present {
		t.Fatal("pending token carries auth_time")
	}
}
