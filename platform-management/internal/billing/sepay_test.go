package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Fixtures in testdata/sepay are written from the SePay contract table in spec
// 7.2 (read 2026-11-17), not captured from the live provider. The secret is a
// fixed test value.
const testSePaySecret = "test-secret-not-real"

var testSePayZone = time.FixedZone("UTC+7", 7*3600)

func readSePayFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sepay/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// signSePay computes X-SePay-Signature over "{timestamp}.{raw_body}".
func signSePay(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func sepayHeaders(secret string, body []byte) http.Header {
	const ts = "1794900000"
	h := http.Header{}
	h.Set("X-SePay-Timestamp", ts)
	h.Set("X-SePay-Signature", signSePay(secret, ts, body))
	return h
}

func TestProviderFixturesParse(t *testing.T) {
	p := NewSePayProvider(testBank(), testSePaySecret)
	cases := []struct {
		file string
		want Notification
	}{
		{"incoming.json", Notification{
			TransferCode: "MULABCDEFGH", Amount: 491700, EventID: "92704", Currency: "VND",
			OccurredAt: time.Date(2026, 11, 17, 9, 30, 15, 0, testSePayZone),
		}},
		{"incoming_messy_note.json", Notification{
			TransferCode: "MULABCDEFGH", Amount: 491700, EventID: "92705", Currency: "VND",
			OccurredAt: time.Date(2026, 11, 17, 9, 31, 2, 0, testSePayZone),
		}},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			body := readSePayFixture(t, c.file)
			got, err := p.ParseNotification(body, sepayHeaders(testSePaySecret, body))
			if err != nil {
				t.Fatalf("ParseNotification: %v", err)
			}
			if got.TransferCode != c.want.TransferCode || got.Amount != c.want.Amount ||
				got.EventID != c.want.EventID || got.Currency != c.want.Currency {
				t.Fatalf("Notification = %+v, want fields %+v", got, c.want)
			}
			if !got.OccurredAt.Equal(c.want.OccurredAt) {
				t.Fatalf("OccurredAt = %v, want %v", got.OccurredAt, c.want.OccurredAt)
			}
		})
	}
}

func TestOutgoingTransferIsNotAnIncomingNotice(t *testing.T) {
	p := NewSePayProvider(testBank(), testSePaySecret)
	body := readSePayFixture(t, "outgoing.json")
	_, err := p.ParseNotification(body, sepayHeaders(testSePaySecret, body))
	if !errors.Is(err, ErrNotIncoming) {
		t.Fatalf("err = %v, want ErrNotIncoming", err)
	}
}

func TestBadOrMissingSignatureReturnsErrSignatureInvalid(t *testing.T) {
	p := NewSePayProvider(testBank(), testSePaySecret)
	body := readSePayFixture(t, "incoming.json")
	good := sepayHeaders(testSePaySecret, body)

	noPrefix := http.Header{}
	noPrefix.Set("X-SePay-Timestamp", good.Get("X-SePay-Timestamp"))
	noPrefix.Set("X-SePay-Signature", strings.TrimPrefix(good.Get("X-SePay-Signature"), "sha256="))

	noTimestamp := http.Header{}
	noTimestamp.Set("X-SePay-Signature", good.Get("X-SePay-Signature"))

	cases := []struct {
		name    string
		body    []byte
		headers http.Header
	}{
		{"no headers", body, http.Header{}},
		{"no timestamp", body, noTimestamp},
		{"signature without sha256 prefix", body, noPrefix},
		{"wrong secret", body, sepayHeaders("another-secret", body)},
		{"tampered body", append([]byte(" "), body...), good},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := p.ParseNotification(c.body, c.headers)
			if !errors.Is(err, ErrSignatureInvalid) {
				t.Fatalf("err = %v, want ErrSignatureInvalid", err)
			}
		})
	}
}

func TestSignatureCheckedBeforeBodyIsParsed(t *testing.T) {
	p := NewSePayProvider(testBank(), testSePaySecret)
	// Not JSON at all: with a bad signature the error must be the signature
	// error, which shows the body was never parsed.
	body := []byte(`not json at all`)
	_, err := p.ParseNotification(body, sepayHeaders("wrong-secret", body))
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("err = %v, want ErrSignatureInvalid before any body parsing", err)
	}
}

func TestSignatureComparisonIsConstantTime(t *testing.T) {
	src, err := os.ReadFile("sepay.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, "hmac.Equal(") {
		t.Fatal("sepay.go must compare signatures with hmac.Equal")
	}
	for _, forbidden := range []string{"bytes.Equal(", "sig ==", "== sig", "signature ==", "== signature"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("sepay.go compares signatures with %q, which is not constant time", forbidden)
		}
	}
}

func TestTransferCodeExtractionNormalizesNote(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"exact code", "MULABCDEFGH", "MULABCDEFGH"},
		{"lower case", "mulabcdefgh", "MULABCDEFGH"},
		{"spaces and hyphens", "MUL-ABCD EFGH", "MULABCDEFGH"},
		{"surrounding text", "chuyen tien mul abcd-efgh thanh toan", "MULABCDEFGH"},
		{"same code twice", "MULABCDEFGH MULABCDEFGH", "MULABCDEFGH"},
		{"no code", "chuyen tien cho shop", ""},
		{"excluded letter I is not a code", "MULIIIIIIII", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ExtractTransferCode(c.content)
			if err != nil {
				t.Fatalf("ExtractTransferCode(%q): %v", c.content, err)
			}
			if got != c.want {
				t.Fatalf("ExtractTransferCode(%q) = %q, want %q", c.content, got, c.want)
			}
		})
	}
}

func TestTransferCodeTwoDifferentCodesIsAmbiguous(t *testing.T) {
	_, err := ExtractTransferCode("MULABCDEFGH MULJKMNPQRS")
	if !errors.Is(err, ErrTransferCodeAmbiguous) {
		t.Fatalf("err = %v, want ErrTransferCodeAmbiguous", err)
	}
}

func TestSePayCreatePaymentRequestNamesProvider(t *testing.T) {
	var p PaymentProvider = NewSePayProvider(testBank(), testSePaySecret)
	got, err := p.CreatePaymentRequest(testOrder())
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != ProviderSePay || got.TransferCode != "MULABCDEFGH" {
		t.Fatalf("Instructions = %+v, want provider %q and the order's code", got, ProviderSePay)
	}
}
