package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// MFAPendingAudience marks a token that proves the first factor only. It
	// is not a session: Auth refuses every token with this audience, and
	// ParseSessionToken does too.
	MFAPendingAudience = "mfa_pending"

	// MFAPendingTTL is how long the user has to produce the second factor.
	MFAPendingTTL = 5 * time.Minute

	mfaPendingUserClaim = "uid"
	mfaPendingIDClaim   = "jti"
)

// ErrNotMFAPendingToken is returned for anything that is not a live, validly
// signed pending token.
var ErrNotMFAPendingToken = errors.New("not an MFA pending token")

// MFAPending is what a pending token proves: the user passed the first
// factor(s) in AMR, at most MFAPendingTTL ago.
type MFAPending struct {
	UserID    string
	TokenID   string
	AMR       []string
	ExpiresAt time.Time
}

// IssueMFAPendingToken signs a pending token for userID. The user id travels
// in `uid`, not `sub`: the other places that read a JWT (WebSocket, daemon
// auth) key on `sub`, so they find no identity in it even if they do not look
// at the audience.
func IssueMFAPendingToken(userID string, amr []string, now time.Time) (string, MFAPending, error) {
	id, err := NewSessionID()
	if err != nil {
		return "", MFAPending{}, fmt.Errorf("mfa pending token id: %w", err)
	}
	expiresAt := now.Add(MFAPendingTTL)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"aud":               MFAPendingAudience,
		mfaPendingUserClaim: userID,
		mfaPendingIDClaim:   id,
		AMRClaim:            amr,
		"iat":               now.Unix(),
		"exp":               expiresAt.Unix(),
	}).SignedString(JWTSecret())
	if err != nil {
		return "", MFAPending{}, err
	}
	return token, MFAPending{UserID: userID, TokenID: id, AMR: amr, ExpiresAt: expiresAt}, nil
}

// ParseMFAPendingToken verifies a pending token as of now.
func ParseMFAPendingToken(tokenString string, now time.Time) (MFAPending, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return JWTSecret(), nil
	}, jwt.WithAudience(MFAPendingAudience), jwt.WithExpirationRequired(), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || !token.Valid {
		return MFAPending{}, ErrNotMFAPendingToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return MFAPending{}, ErrNotMFAPendingToken
	}
	userID, _ := claims[mfaPendingUserClaim].(string)
	id, _ := claims[mfaPendingIDClaim].(string)
	amr := SessionAMR(claims)
	if userID == "" || id == "" || len(amr) == 0 {
		return MFAPending{}, ErrNotMFAPendingToken
	}
	return MFAPending{UserID: userID, TokenID: id, AMR: amr, ExpiresAt: SessionExpiry(claims)}, nil
}

// IsMFAPendingClaims reports whether claims carry the pending audience. An
// audience that cannot be read counts as pending: it is not a session's.
func IsMFAPendingClaims(claims jwt.MapClaims) bool {
	aud, err := claims.GetAudience()
	if err != nil {
		return true
	}
	for _, a := range aud {
		if a == MFAPendingAudience {
			return true
		}
	}
	return false
}
