package einvoice

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/invoice"
)

const (
	templateCode  = "1"
	statusPending = "Pending"
	statusSuccess = "Success"
	statusFailed  = "Failed"
	codeExisted   = "EINVOICE_DOCUMENT_EXISTED"
	timeLayout    = "2006-01-02 15:04:05"
	dateLayout    = "2006-01-02"
)

// Config configures the SePay provider. TaxRate is a whole percent: 0, 5, 8 or 10.
type Config struct {
	BaseURL           string
	APIKey            string // client_id:client_secret
	ProviderAccountID string
	TaxRate           int64
	Location          *time.Location
	HTTPClient        *http.Client
	PollInterval      time.Duration
	MaxCreatePolls    int
	Now               func() time.Time
}

type SePay struct {
	cfg Config

	mu      sync.Mutex
	token   string
	expires time.Time
}

var _ invoice.Provider = (*SePay)(nil)

// New validates cfg and applies defaults. Its errors never contain the API key.
func New(cfg Config) (*SePay, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		return nil, errors.New("einvoice: base URL is required")
	}
	id, secret, found := strings.Cut(cfg.APIKey, ":")
	if !found || id == "" || secret == "" {
		return nil, errors.New("einvoice: API key must be client_id:client_secret")
	}
	if strings.TrimSpace(cfg.ProviderAccountID) == "" {
		return nil, errors.New("einvoice: provider account ID is required")
	}
	switch cfg.TaxRate {
	case 0, 5, 8, 10:
	default:
		return nil, errors.New("einvoice: tax rate must be 0, 5, 8 or 10 percent")
	}
	if cfg.Location == nil {
		loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
		if err != nil {
			loc = time.FixedZone("ICT", 7*60*60)
		}
		cfg.Location = loc
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.MaxCreatePolls == 0 {
		cfg.MaxCreatePolls = 30
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &SePay{cfg: cfg}, nil
}

func (s *SePay) Name() string { return "sepay" }

// RequiredBuyerFields is an assumption: SePay does not document which buyer
// fields it requires, so this is the same list as the fake provider's.
func (s *SePay) RequiredBuyerFields() []string {
	return []string{"name", "tax_code", "address", "email"}
}

func (s *SePay) ReplaceSupported() bool { return false }

func (s *SePay) Replace(context.Context, string, invoice.IssueRequest) (invoice.Ticket, error) {
	return invoice.Ticket{}, invoice.ErrReplaceUnsupported
}

type checkData struct {
	Status  string `json:"status"`
	Invoice *struct {
		ReferenceCode string `json:"reference_code"`
		InvoiceNumber string `json:"invoice_number"`
		IssuedDate    string `json:"issued_date"`
		PDFURL        string `json:"pdf_url"`
	} `json:"invoice"`
}

type detailData struct {
	ReferenceCode string `json:"reference_code"`
	InvoiceNumber string `json:"invoice_number"`
	IssuedDate    string `json:"issued_date"`
	PDFURL        string `json:"pdf_url"`
	Status        string `json:"status"`
}

type trackingData struct {
	TrackingCode string `json:"tracking_code"`
}

// Issue creates a draft, waits until SePay has created it, then issues it. A
// draft that already exists under the reference is issued without a second create.
func (s *SePay) Issue(ctx context.Context, req invoice.IssueRequest) (invoice.Ticket, error) {
	if missing := invoice.CheckBuyer(s, req.Buyer); len(missing) > 0 {
		return invoice.Ticket{}, &invoice.ProviderError{Kind: invoice.KindMissingFields, Code: "VALIDATION_ERROR", Fields: missing}
	}
	series, err := s.invoiceSeries(ctx)
	if err != nil {
		return invoice.Ticket{}, err
	}

	var created trackingData
	err = s.do(ctx, http.MethodPost, "/v1/invoices/create", s.createBody(req, series), &created)
	switch {
	case err == nil:
		if err := s.waitCreated(ctx, created.TrackingCode); err != nil {
			return invoice.Ticket{}, err
		}
	case isStatus(err, http.StatusConflict) && errorCode(err) == codeExisted:
		var d detailData
		if derr := s.do(ctx, http.MethodGet, "/v1/invoices/"+url.PathEscape(req.ReferenceCode), nil, &d); derr != nil {
			return invoice.Ticket{}, derr
		}
		if d.Status != "draft" {
			return invoice.Ticket{}, providerErr(invoice.KindPermanent, codeExisted)
		}
	default:
		return invoice.Ticket{}, err
	}

	var issued trackingData
	body := map[string]string{"reference_code": req.ReferenceCode}
	if err := s.do(ctx, http.MethodPost, "/v1/invoices/issue", body, &issued); err != nil {
		return invoice.Ticket{}, err
	}
	return invoice.Ticket{ReferenceCode: req.ReferenceCode, TrackingCode: issued.TrackingCode}, nil
}

func (s *SePay) invoiceSeries(ctx context.Context) (string, error) {
	var acct struct {
		Templates []struct {
			TemplateCode  string `json:"template_code"`
			InvoiceSeries string `json:"invoice_series"`
		} `json:"templates"`
	}
	if err := s.do(ctx, http.MethodGet, "/v1/provider-accounts/"+url.PathEscape(s.cfg.ProviderAccountID), nil, &acct); err != nil {
		return "", err
	}
	for _, t := range acct.Templates {
		if t.TemplateCode == templateCode && t.InvoiceSeries != "" {
			return t.InvoiceSeries, nil
		}
	}
	return "", providerErr(invoice.KindPermanent, "template_not_found")
}

// createBody builds the create request. Assumption, for accounting review: one
// service line with fixed item code, name and unit, and a buyer typed company
// when it has a tax code. SePay does not document the buyer and item fields.
func (s *SePay) createBody(req invoice.IssueRequest, series string) map[string]any {
	buyer := map[string]any{
		"type": "personal", "name": req.Buyer.Name, "tax_code": req.Buyer.TaxCode,
		"address": req.Buyer.Address, "email": req.Buyer.Email,
	}
	if req.Buyer.TaxCode != "" {
		buyer["type"] = "company"
		buyer["legal_name"] = req.Buyer.Name
	}
	return map[string]any{
		"template_code":       templateCode,
		"invoice_series":      series,
		"issued_date":         s.cfg.Now().In(s.cfg.Location).Format(timeLayout),
		"currency":            strings.ToUpper(req.Currency),
		"provider_account_id": s.cfg.ProviderAccountID,
		"reference_code":      req.ReferenceCode,
		"is_draft":            true,
		"buyer":               buyer,
		"items": []map[string]any{{
			"line_number": 1, "line_type": 1, "item_code": "SERVICE-1",
			"item_name": "Dịch vụ phần mềm", "unit": "Gói", "quantity": "1.00",
			"unit_price": req.Amount, "tax_rate": s.cfg.TaxRate, "tax_amount": req.VATAmount,
			"before_discount_and_tax_amount": req.Amount,
		}},
		"total_amount": req.Amount + req.VATAmount,
	}
}

func (s *SePay) waitCreated(ctx context.Context, tracking string) error {
	path := "/v1/invoices/create/check/" + url.PathEscape(tracking)
	for i := 0; i < s.cfg.MaxCreatePolls; i++ {
		if i > 0 {
			if err := sleep(ctx, s.cfg.PollInterval); err != nil {
				return err
			}
		}
		var d checkData
		if err := s.do(ctx, http.MethodGet, path, nil, &d); err != nil {
			return err
		}
		switch d.Status {
		case statusSuccess:
			return nil
		case statusFailed:
			return providerErr(invoice.KindPermanent, "create_failed")
		}
	}
	return providerErr(invoice.KindTransient, "create_pending")
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return transportErr(ctx.Err())
	case <-t.C:
		return nil
	}
}

func (s *SePay) Status(ctx context.Context, trackingCode string) (invoice.Poll, error) {
	var d checkData
	if err := s.do(ctx, http.MethodGet, "/v1/invoices/issue/check/"+url.PathEscape(trackingCode), nil, &d); err != nil {
		return invoice.Poll{}, err
	}
	switch d.Status {
	case statusSuccess:
		if d.Invoice == nil {
			return invoice.Poll{}, providerErr(invoice.KindTransient, badResponse)
		}
		return invoice.Poll{State: invoice.PollSuccess, Invoice: s.issuedInvoice(d.Invoice.ReferenceCode, d.Invoice.InvoiceNumber, d.Invoice.PDFURL, d.Invoice.IssuedDate)}, nil
	case statusFailed:
		return invoice.Poll{State: invoice.PollFailed, Code: "issue_failed"}, nil
	}
	return invoice.Poll{State: invoice.PollPending}, nil
}

// Lookup finds an issued invoice by reference. A draft is not issued, so it is
// ErrNoInvoice and the next Issue picks the draft up.
func (s *SePay) Lookup(ctx context.Context, referenceCode string) (invoice.Invoice, error) {
	var d detailData
	if err := s.do(ctx, http.MethodGet, "/v1/invoices/"+url.PathEscape(referenceCode), nil, &d); err != nil {
		if isStatus(err, http.StatusNotFound) {
			return invoice.Invoice{}, invoice.ErrNoInvoice
		}
		return invoice.Invoice{}, err
	}
	if d.Status != "issued" {
		return invoice.Invoice{}, invoice.ErrNoInvoice
	}
	return s.issuedInvoice(d.ReferenceCode, d.InvoiceNumber, d.PDFURL, d.IssuedDate), nil
}

func (s *SePay) issuedInvoice(ref, number, pdf, issued string) invoice.Invoice {
	at, _ := time.ParseInLocation(dateLayout, issued, s.cfg.Location)
	return invoice.Invoice{ProviderInvoiceID: ref, Number: number, LookupURL: pdf, Issued: true, IssuedAt: at}
}
