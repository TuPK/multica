// Package sso holds the single sign-on rules of the platform spec 6.3. This
// file is the domain verification part: a workspace owner proves control of a
// domain by publishing a DNS TXT record that holds a token Multica issued.
package sso

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"strings"

	"github.com/multica-ai/multica/server/internal/workspacepolicy"
)

// TXTLookup resolves the TXT records of a DNS name. *net.Resolver satisfies it,
// so production passes net.DefaultResolver and tests pass a fake.
type TXTLookup interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

const (
	challengeLabel = "_multica-challenge"
	valuePrefix    = "multica-verification="
)

// publicEmailDomains are mailbox providers anyone can register at. A workspace
// cannot verify one of them, or a subdomain of one: SSO and the invite policy
// trust a verified domain to mean "our people". Every entry is lower case.
var publicEmailDomains = []string{
	"gmail.com", "googlemail.com",
	"outlook.com", "hotmail.com", "live.com", "msn.com", "hotmail.co.uk", "outlook.com.vn",
	"yahoo.com", "yahoo.co.uk", "yahoo.co.jp", "yahoo.com.vn", "ymail.com",
	"icloud.com", "me.com", "mac.com",
	"aol.com",
	"proton.me", "protonmail.com", "pm.me",
	"gmx.com", "gmx.net", "gmx.de",
	"mail.com", "zoho.com", "fastmail.com", "hey.com", "tutanota.com", "tuta.com",
	"yandex.com", "yandex.ru", "mail.ru",
	"qq.com", "163.com", "126.com", "sina.com",
	"naver.com", "daum.net", "hanmail.net",
}

// NormalizeDomain returns domain lower-cased, without a trailing dot, or an
// error when it is not a valid host name with at least two labels.
func NormalizeDomain(raw string) (string, error) {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if strings.HasPrefix(d, "@") {
		return "", errors.New("not a valid domain")
	}
	out, err := workspacepolicy.NormalizeDomains([]string{d})
	if err != nil {
		return "", err
	}
	return out[0], nil
}

// IsPublicEmailDomain reports whether domain is a public email provider or a
// subdomain of one.
func IsPublicEmailDomain(domain string) bool {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	for _, p := range publicEmailDomains {
		if d == p || strings.HasSuffix(d, "."+p) {
			return true
		}
	}
	return false
}

// NewToken returns a random token for one workspace and domain.
func NewToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ChallengeName is the DNS name that must hold the record of domain.
func ChallengeName(domain string) string { return challengeLabel + "." + domain }

// ChallengeValue is the TXT value that proves the holder of token.
func ChallengeValue(token string) string { return valuePrefix + token }

// CheckTXT reports whether the challenge name of domain holds the value of
// token. A name with no TXT record is a clean "no"; any other lookup failure is
// returned as an error, so a resolver outage is not read as a wrong record.
func CheckTXT(ctx context.Context, lookup TXTLookup, domain, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	records, err := lookup.LookupTXT(ctx, ChallengeName(domain))
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return false, nil
		}
		return false, err
	}
	want := ChallengeValue(token)
	for _, r := range records {
		if strings.TrimSpace(r) == want {
			return true, nil
		}
	}
	return false, nil
}
