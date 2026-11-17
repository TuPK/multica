package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The plans and orders of the platform's billing (platform-management spec 5.8).
// plan and billing_order are platform tables in the shared database. The server
// reads them, and its one write is the buyer of a pending order: the details the
// customer wants on the invoice. Everything else about an order is the
// platform's.

// maxBillingOrders bounds the order list. A workspace places few orders; the
// newest are the ones the page needs.
const maxBillingOrders = 50

// Limits of the buyer fields, in characters.
const (
	maxBuyerNameChars    = 200
	maxBuyerAddressChars = 500
	maxBuyerEmailChars   = 254
)

// taxCodePattern is a Vietnamese tax code: 10 digits, or 13 for a branch.
var taxCodePattern = regexp.MustCompile(`^(\d{10}|\d{13})$`)

type workspacePlanJSON struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	Currency       string `json:"currency"`
	SeatPriceMonth *int64 `json:"seat_price_month"`
	SeatPriceYear  *int64 `json:"seat_price_year"`
	SeatsFixed     *int32 `json:"seats_fixed"`
	MinSeats       *int32 `json:"min_seats"`
	TrialDays      int32  `json:"trial_days"`

	BillingModel         string                   `json:"billing_model"`
	UsageRates           map[string]usageRateJSON `json:"usage_rates"`
	EffectiveDiscountPct int                      `json:"effective_discount_pct"`

	// ScheduledPriceChange is set only on the plan the workspace is on, once the
	// notice of the change is sent.
	ScheduledPriceChange *scheduledPriceChangeJSON `json:"scheduled_price_change"`
}

type scheduledPriceChangeJSON struct {
	EffectiveFrom  string `json:"effective_from"`
	SeatPriceMonth *int64 `json:"seat_price_month"`
	SeatPriceYear  *int64 `json:"seat_price_year"`
}

// usageRateJSON is one meter of a usage plan: price VND for per units.
type usageRateJSON struct {
	Per   int64 `json:"per"`
	Price int64 `json:"price"`
}

// orderUsageLineJSON is the charge of one day and meter on a usage invoice.
type orderUsageLineJSON struct {
	Day         string `json:"day"`
	Meter       string `json:"meter"`
	Quantity    int64  `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	Per         int64  `json:"per"`
	DiscountPct int32  `json:"discount_pct"`
	NetAmount   int64  `json:"net_amount"`
	Note        string `json:"note"`
}

type orderUsageResponse struct {
	OrderID string               `json:"order_id"`
	Lines   []orderUsageLineJSON `json:"lines"`
}

// businessDiscountRequiresVerification mirrors the platform's rule (spec 7b.3
// rule 9): the Business discount needs a verified profile unless the env is "false".
func businessDiscountRequiresVerification() bool {
	return strings.TrimSpace(os.Getenv("PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION")) != "false"
}

type workspacePlansResponse struct {
	Plans []workspacePlanJSON `json:"plans"`
}

func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	return &v.Int32
}

// orderBuyerJSON is the buyer column: who the invoice is made out to.
type orderBuyerJSON struct {
	Name         string `json:"name"`
	TaxCode      string `json:"tax_code"`
	Address      string `json:"address"`
	InvoiceEmail string `json:"invoice_email"`
}

// orderBankJSON is where the customer sends the transfer. The deployment's
// BILLING_BANK_* variables name it; an order does not store it.
type orderBankJSON struct {
	BankName      string `json:"bank_name"`
	AccountName   string `json:"account_name"`
	AccountNumber string `json:"account_number"`
}

func billingBankDetails() orderBankJSON {
	return orderBankJSON{
		BankName:      strings.TrimSpace(os.Getenv("BILLING_BANK_NAME")),
		AccountName:   strings.TrimSpace(os.Getenv("BILLING_BANK_ACCOUNT_NAME")),
		AccountNumber: strings.TrimSpace(os.Getenv("BILLING_BANK_ACCOUNT_NUMBER")),
	}
}

// workspaceOrderJSON is an order as the order page reads it. Amounts are whole
// units of the currency (VND has no decimals); total_amount is what the customer
// transfers, VAT included.
type workspaceOrderJSON struct {
	ID           string         `json:"id"`
	WorkspaceID  string         `json:"workspace_id"`
	Kind         string         `json:"kind"`
	PlanKey      string         `json:"plan_key"`
	Interval     string         `json:"interval"`
	Seats        int32          `json:"seats"`
	Amount       int64          `json:"amount"`
	VATAmount    int64          `json:"vat_amount"`
	TotalAmount  int64          `json:"total_amount"`
	Currency     string         `json:"currency"`
	Status       string         `json:"status"`
	Provider     string         `json:"provider"`
	TransferCode string         `json:"transfer_code"`
	BankDetails  orderBankJSON  `json:"bank_details"`
	Buyer        orderBuyerJSON `json:"buyer"`
	PeriodStart  *string        `json:"period_start"`
	PeriodEnd    *string        `json:"period_end"`
	ExpiresAt    string         `json:"expires_at"`
	PaidAt       *string        `json:"paid_at"`
	CreatedAt    string         `json:"created_at"`
}

type workspaceOrdersResponse struct {
	Orders []workspaceOrderJSON `json:"orders"`
}

func toWorkspaceOrderJSON(o db.BillingOrder, bank orderBankJSON) workspaceOrderJSON {
	var buyer orderBuyerJSON
	// A buyer that is not an object of strings reads as empty; the platform
	// writes {} and the buyer route writes the four fields.
	if err := json.Unmarshal(o.Buyer, &buyer); err != nil {
		buyer = orderBuyerJSON{}
	}
	return workspaceOrderJSON{
		ID: uuidToString(o.ID), WorkspaceID: uuidToString(o.WorkspaceID), Kind: o.Kind, PlanKey: o.PlanKey,
		Interval: o.Interval, Seats: o.Seats, Amount: o.Amount, VATAmount: o.VatAmount,
		TotalAmount: o.Amount + o.VatAmount, Currency: o.Currency, Status: o.Status, Provider: o.Provider,
		TransferCode: o.TransferCode, BankDetails: bank, Buyer: buyer,
		PeriodStart: timestampToPtr(o.PeriodStart), PeriodEnd: timestampToPtr(o.PeriodEnd),
		ExpiresAt: timestampToString(o.ExpiresAt), PaidAt: timestampToPtr(o.PaidAt), CreatedAt: timestampToString(o.CreatedAt),
	}
}

// requireBillingWorkspace is the handler-level backstop behind the router's role
// middleware, as requireCloudSubscriptionWorkspace is for the Cloud routes: a
// human actor who is an owner or admin of the workspace, with the workspace
// subscriptions flag on. It returns the workspace's UUID.
func (h *Handler) requireBillingWorkspace(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return pgtype.UUID{}, false
	}
	if !featureflags.BillingWorkspaceSubscriptionsEnabled(r.Context(), h.FeatureFlags) {
		writeFeatureDisabled(w, "workspace_subscriptions_disabled", "workspace subscriptions are not enabled")
		return pgtype.UUID{}, false
	}
	member, ok := ctxMember(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "workspace membership required")
		return pgtype.UUID{}, false
	}
	if !roleAllowed(member.Role, "owner", "admin") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return pgtype.UUID{}, false
	}
	return parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
}

// ListWorkspacePlans serves GET /api/workspaces/{id}/plans: the public plans and
// their seat prices, for the plan selector. Owner and admin.
func (h *Handler) ListWorkspacePlans(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := h.requireBillingWorkspace(w, r)
	if !ok {
		return
	}
	rows, err := h.Queries.ListPublicPlans(r.Context())
	if err != nil {
		slog.Warn("list public plans failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	// A workspace without a billing profile is an unverified individual.
	profile, err := h.Queries.GetWorkspaceBillingProfile(r.Context(), workspaceUUID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("get billing profile failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	businessDiscount := err == nil && profile.AccountKind == "business" &&
		(profile.VerifiedAt.Valid || !businessDiscountRequiresVerification())
	inUse, scheduled, err := h.scheduledPriceChange(r.Context(), workspaceUUID)
	if err != nil {
		slog.Warn("read scheduled price change failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	plans := make([]workspacePlanJSON, len(rows))
	for i, p := range rows {
		plans[i] = workspacePlanJSON{
			Key: p.Key, Name: p.Name, Currency: p.Currency, SeatPriceMonth: int8Ptr(p.SeatPriceMonth),
			SeatPriceYear: int8Ptr(p.SeatPriceYear), SeatsFixed: int4Ptr(p.SeatsFixed), MinSeats: int4Ptr(p.MinSeats),
			TrialDays: p.TrialDays, BillingModel: p.BillingModel, UsageRates: map[string]usageRateJSON{},
		}
		if scheduled != nil && p.Key == inUse {
			plans[i].ScheduledPriceChange = scheduled.json(p)
		}
		if p.BillingModel != "usage" {
			continue
		}
		// Malformed rates or terms read as none: the platform validates them on write.
		_ = json.Unmarshal(p.UsageRates, &plans[i].UsageRates)
		if plans[i].UsageRates == nil {
			plans[i].UsageRates = map[string]usageRateJSON{}
		}
		if businessDiscount {
			var terms struct {
				DiscountPct map[string]int `json:"discount_pct"`
			}
			_ = json.Unmarshal(p.UsageTerms, &terms)
			plans[i].EffectiveDiscountPct = terms.DiscountPct["business"]
		}
	}
	writeJSON(w, http.StatusOK, workspacePlansResponse{Plans: plans})
}

// scheduledChange is a plan's scheduled price change: the changed price fields
// of the row, which a seat price that is absent from leaves as it is.
type scheduledChange struct {
	effectiveFrom time.Time
	prices        map[string]*int64
}

// json returns the change as the page sees it: a price the change does not set
// is the plan's current one.
func (c *scheduledChange) json(p db.Plan) *scheduledPriceChangeJSON {
	price := func(field string, current pgtype.Int8) *int64 {
		if v, set := c.prices[field]; set {
			return v
		}
		return int8Ptr(current)
	}
	return &scheduledPriceChangeJSON{
		EffectiveFrom:  c.effectiveFrom.UTC().Format(time.RFC3339),
		SeatPriceMonth: price("seat_price_month", p.SeatPriceMonth),
		SeatPriceYear:  price("seat_price_year", p.SeatPriceYear),
	}
}

// scheduledPriceChange returns the key of the plan the workspace is on (free
// without a subscription row) and its scheduled change once the notice is sent,
// nil when it has none.
func (h *Handler) scheduledPriceChange(ctx context.Context, workspaceID pgtype.UUID) (string, *scheduledChange, error) {
	key, err := h.Queries.GetWorkspacePlanKey(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		key, err = "free", nil
	}
	if err != nil {
		return "", nil, err
	}
	row, err := h.Queries.GetNoticedScheduledPlanPriceChange(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return key, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	c := &scheduledChange{effectiveFrom: row.EffectiveFrom.Time, prices: map[string]*int64{}}
	var changed map[string]*int64
	// Malformed prices read as no change: the platform validates them on write.
	_ = json.Unmarshal(row.New, &changed)
	for _, field := range []string{"seat_price_month", "seat_price_year"} {
		if v, set := changed[field]; set {
			c.prices[field] = v
		}
	}
	return key, c, nil
}

// ListWorkspaceOrders serves GET /api/workspaces/{id}/orders: the workspace's
// newest orders first. Owner and admin.
func (h *Handler) ListWorkspaceOrders(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := h.requireBillingWorkspace(w, r)
	if !ok {
		return
	}
	rows, err := h.Queries.ListWorkspaceBillingOrders(r.Context(), db.ListWorkspaceBillingOrdersParams{
		WorkspaceID: workspaceUUID, Limit: maxBillingOrders,
	})
	if err != nil {
		slog.Warn("list billing orders failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}
	bank := billingBankDetails()
	orders := make([]workspaceOrderJSON, len(rows))
	for i, o := range rows {
		orders[i] = toWorkspaceOrderJSON(o, bank)
	}
	writeJSON(w, http.StatusOK, workspaceOrdersResponse{Orders: orders})
}

// GetWorkspaceOrder serves GET /api/workspaces/{id}/orders/{orderId}. An order of
// another workspace is a 404.
func (h *Handler) GetWorkspaceOrder(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := h.requireBillingWorkspace(w, r)
	if !ok {
		return
	}
	orderUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "orderId"), "order id")
	if !ok {
		return
	}
	order, err := h.Queries.GetWorkspaceBillingOrder(r.Context(), db.GetWorkspaceBillingOrderParams{
		WorkspaceID: workspaceUUID, ID: orderUUID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if err != nil {
		slog.Warn("get billing order failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to get order")
		return
	}
	writeJSON(w, http.StatusOK, toWorkspaceOrderJSON(order, billingBankDetails()))
}

// GetWorkspaceOrderUsage serves GET /api/workspaces/{id}/orders/{orderId}/usage:
// the day lines of a usage invoice. An order of another workspace, or one that
// is not a usage invoice, is a 404.
func (h *Handler) GetWorkspaceOrderUsage(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := h.requireBillingWorkspace(w, r)
	if !ok {
		return
	}
	orderUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "orderId"), "order id")
	if !ok {
		return
	}
	order, err := h.Queries.GetWorkspaceBillingOrder(r.Context(), db.GetWorkspaceBillingOrderParams{
		WorkspaceID: workspaceUUID, ID: orderUUID,
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && order.Kind != "usage") {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if err != nil {
		slog.Warn("get billing order failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to get order usage")
		return
	}
	charges, err := h.Queries.ListUsageChargesForOrder(r.Context(), order.ID)
	if err != nil {
		slog.Warn("list order usage failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to get order usage")
		return
	}
	lines := make([]orderUsageLineJSON, len(charges))
	for i, c := range charges {
		lines[i] = orderUsageLineJSON{
			Day: c.Day.Time.Format(time.DateOnly), Meter: c.Meter, Quantity: c.Quantity, UnitPrice: c.UnitPrice,
			Per: c.Per, DiscountPct: c.DiscountPct, NetAmount: c.NetAmount, Note: c.Note,
		}
	}
	writeJSON(w, http.StatusOK, orderUsageResponse{OrderID: uuidToString(order.ID), Lines: lines})
}

// buyerFieldError names the field a buyer value was refused for.
type buyerFieldError struct {
	field, msg string
}

// normalizeOrderBuyer trims the four buyer fields and checks them. Every field
// may be empty; the invoice details are the customer's to give.
func normalizeOrderBuyer(in orderBuyerJSON) (orderBuyerJSON, *buyerFieldError) {
	in = orderBuyerJSON{
		Name: strings.TrimSpace(in.Name), TaxCode: strings.TrimSpace(in.TaxCode),
		Address: strings.TrimSpace(in.Address), InvoiceEmail: strings.TrimSpace(in.InvoiceEmail),
	}
	for _, f := range []struct {
		field, value string
		max          int
	}{
		{"name", in.Name, maxBuyerNameChars},
		{"address", in.Address, maxBuyerAddressChars},
		{"invoice_email", in.InvoiceEmail, maxBuyerEmailChars},
	} {
		if utf8.RuneCountInString(f.value) > f.max || strings.ContainsRune(f.value, 0) {
			return in, &buyerFieldError{f.field, f.field + " is too long"}
		}
	}
	if in.TaxCode != "" && !taxCodePattern.MatchString(in.TaxCode) {
		return in, &buyerFieldError{"tax_code", "tax_code must be 10 or 13 digits"}
	}
	if in.InvoiceEmail != "" {
		addr, err := mail.ParseAddress(in.InvoiceEmail)
		if err != nil || addr.Address != in.InvoiceEmail {
			return in, &buyerFieldError{"invoice_email", "invoice_email must be an email address"}
		}
	}
	return in, nil
}

// UpdateWorkspaceOrderBuyer serves PUT /api/workspaces/{id}/orders/{orderId}/buyer:
// it replaces the buyer of a pending order of the workspace. Any other order is
// left alone: an order of another workspace is a 404, and one that is no longer
// pending is a 409 order_not_pending. This is the one write the server makes to
// a platform table other than platform_workspace.
func (h *Handler) UpdateWorkspaceOrderBuyer(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := h.requireBillingWorkspace(w, r)
	if !ok {
		return
	}
	orderUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "orderId"), "order id")
	if !ok {
		return
	}
	var in orderBuyerJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	buyer, ferr := normalizeOrderBuyer(in)
	if ferr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ferr.msg, "code": "buyer_invalid", "field": ferr.field})
		return
	}
	raw, err := json.Marshal(buyer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save buyer")
		return
	}

	order, err := h.Queries.SetPendingBillingOrderBuyer(r.Context(), db.SetPendingBillingOrderBuyerParams{
		WorkspaceID: workspaceUUID, ID: orderUUID, Buyer: raw,
	})
	if err == nil {
		writeJSON(w, http.StatusOK, toWorkspaceOrderJSON(order, billingBankDetails()))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("save order buyer failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save buyer")
		return
	}
	// Nothing was written: the order is not this workspace's, or it is not pending.
	existing, err := h.Queries.GetWorkspaceBillingOrder(r.Context(), db.GetWorkspaceBillingOrderParams{
		WorkspaceID: workspaceUUID, ID: orderUUID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if err != nil {
		slog.Warn("get billing order failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save buyer")
		return
	}
	writeJSON(w, http.StatusConflict, map[string]string{
		"error": "the order is no longer pending", "code": "order_not_pending", "status": existing.Status,
	})
}
