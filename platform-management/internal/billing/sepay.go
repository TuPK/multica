package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ProviderSePay names the SePay bank-transfer webhook provider.
const ProviderSePay = "sepay"

var (
	// ErrSignatureInvalid means the webhook signature is missing or wrong.
	ErrSignatureInvalid = errors.New("billing: webhook signature invalid")
	// ErrNotIncoming means the notification is not an incoming transfer.
	ErrNotIncoming = errors.New("billing: notification is not an incoming transfer")
	// ErrTransferCodeAmbiguous means the note holds two different transfer codes.
	ErrTransferCodeAmbiguous = errors.New("billing: note contains more than one transfer code")
)

const sepayCurrency = "VND"

var sepayZone = time.FixedZone("UTC+7", 7*3600)

// SePay is the SePay provider: it confirms payment from a signed webhook.
type SePay struct {
	bank   BankDetails
	secret []byte
}

var _ PaymentProvider = (*SePay)(nil)

// NewSePayProvider returns the SePay provider for the bank account and webhook secret.
func NewSePayProvider(bank BankDetails, secret string) *SePay {
	return &SePay{bank: bank, secret: []byte(secret)}
}

// CreatePaymentRequest returns the manual instructions, named for SePay.
func (p *SePay) CreatePaymentRequest(o Order) (Instructions, error) {
	in, err := NewManualProvider(p.bank).CreatePaymentRequest(o)
	if err != nil {
		return Instructions{}, err
	}
	in.Provider = ProviderSePay
	return in, nil
}

// QueryStatus is ErrUnsupported: the contract has no status query.
func (*SePay) QueryStatus(Order) (Status, error) { return "", ErrUnsupported }

// ParseNotification verifies the signature, then reads an incoming transfer.
func (p *SePay) ParseNotification(body []byte, headers http.Header) (Notification, error) {
	if err := p.verify(body, headers); err != nil {
		return Notification{}, err
	}
	var in struct {
		ID           int64  `json:"id"`
		Content      string `json:"content"`
		Date         string `json:"transactionDate"`
		TransferType string `json:"transferType"`
		Amount       int64  `json:"transferAmount"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return Notification{}, fmt.Errorf("billing: sepay body: %w", err)
	}
	if in.TransferType != "in" {
		return Notification{}, ErrNotIncoming
	}
	if in.ID <= 0 {
		return Notification{}, errors.New("billing: sepay event id missing")
	}
	if in.Amount <= 0 {
		return Notification{}, errors.New("billing: sepay amount must be positive")
	}
	at, err := time.ParseInLocation("2006-01-02 15:04:05", in.Date, sepayZone)
	if err != nil {
		return Notification{}, fmt.Errorf("billing: sepay transaction date: %w", err)
	}
	code, err := ExtractTransferCode(in.Content)
	if err != nil {
		return Notification{}, err
	}
	id := strconv.FormatInt(in.ID, 10)
	return Notification{
		TransferCode: code,
		ProviderRef:  id,
		Amount:       in.Amount,
		EventID:      id,
		Currency:     sepayCurrency,
		OccurredAt:   at,
	}, nil
}

// verify checks X-SePay-Signature over "{timestamp}.{body}". It sets no
// timestamp window.
func (p *SePay) verify(body []byte, headers http.Header) error {
	ts := headers.Get("X-SePay-Timestamp")
	hexSum, ok := strings.CutPrefix(headers.Get("X-SePay-Signature"), "sha256=")
	if ts == "" || !ok {
		return ErrSignatureInvalid
	}
	got, err := hex.DecodeString(hexSum)
	if err != nil {
		return ErrSignatureInvalid
	}
	mac := hmac.New(sha256.New, p.secret)
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return ErrSignatureInvalid
	}
	return nil
}

// ExtractTransferCode finds the transfer code in a transfer note, ignoring
// case, spaces and hyphens. It returns "" when the note has none.
func ExtractTransferCode(content string) (string, error) {
	norm := strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "", "\n", "", "\r", "").Replace(content))
	found := ""
	for i := 0; i+len(transferPrefix)+transferLen <= len(norm); i++ {
		c := norm[i : i+len(transferPrefix)+transferLen]
		if !ValidTransferCode(c) {
			continue
		}
		if found != "" && found != c {
			return "", ErrTransferCodeAmbiguous
		}
		found = c
	}
	return found, nil
}
