package pricechange

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxReasonChars = 500

// Meters are the usage meters of spec 7b.3.
var Meters = []string{"tasks_run", "storage_bytes", "members"}

// Business and individual are the account kinds a credit limit names.
var accountKinds = []string{"individual", "business"}

func invalid(field, msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "price_invalid", Message: msg, Detail: map[string]any{"field": field}}
}

// wholeNumber reads a JSON number that is an integer: no sign rules, no
// fraction, no exponent, no quotes.
func wholeNumber(raw json.RawMessage) (int64, bool) {
	s := string(bytes.TrimSpace(raw))
	if s == "" || strings.ContainsAny(s, ".eE\"") || s == "null" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

// seatPrice is the value of a seat price field: ok is false for null.
func seatPrice(raw json.RawMessage) (v int64, ok bool) { return wholeNumber(raw) }

type rate struct {
	Per   int64 `json:"per"`
	Price int64 `json:"price"`
}

func decodeStrict(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("trailing data")
	}
	return nil
}

func parseRates(raw json.RawMessage) (map[string]rate, error) {
	// Numbers are read as int64, which refuses a fraction.
	out := map[string]rate{}
	if err := decodeStrict(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

type terms struct {
	DiscountPct map[string]int64 `json:"discount_pct"`
	CreditLimit map[string]int64 `json:"credit_limit"`
}

func parseTerms(raw json.RawMessage) (terms, error) {
	var t terms
	return t, decodeStrict(raw, &t)
}

// validatePrices checks the new values of a change against the plan (spec
// 7b.4 rule 4) and returns the plan's current values of the same fields.
func validatePrices(p PlanRow, req Request, max int64) (old Prices, err *Error) {
	if len(req.New) == 0 {
		return nil, invalid("new", "a price change names at least one price field")
	}
	for _, f := range slices.Sorted(maps.Keys(req.New)) {
		if !slices.Contains(PriceFields, f) {
			return nil, invalid(f, f+" is not a price field")
		}
	}
	seat := map[string]int64{}
	for _, f := range []string{FieldSeatPriceMonth, FieldSeatPriceYear} {
		raw, ok := req.New[f]
		if !ok {
			continue
		}
		n, ok := wholeNumber(raw)
		if !ok || n < 0 {
			return nil, invalid(f, f+" must be a whole number of 0 or more")
		}
		if n > max {
			return nil, invalid(f, f+" is above the maximum price")
		}
		seat[f] = n
	}
	if raw, ok := req.New[FieldUsageRates]; ok {
		rates, err := parseRates(raw)
		if err != nil {
			return nil, invalid(FieldUsageRates, "usage_rates maps a meter to {per, price} in whole numbers")
		}
		for meter, r := range rates {
			if !slices.Contains(Meters, meter) {
				return nil, invalid(FieldUsageRates, "unknown meter "+meter)
			}
			if r.Per <= 0 || r.Price < 0 || r.Price > max {
				return nil, invalid(FieldUsageRates, "per must be positive and price from 0 to the maximum price")
			}
		}
	}
	if raw, ok := req.New[FieldUsageTerms]; ok {
		t, err := parseTerms(raw)
		if err != nil {
			return nil, invalid(FieldUsageTerms, "usage_terms holds discount_pct and credit_limit objects of whole numbers")
		}
		for kind, pct := range t.DiscountPct {
			if kind != "business" || pct < 0 || pct > 100 {
				return nil, invalid(FieldUsageTerms, "discount_pct.business is from 0 to 100")
			}
		}
		for kind, limit := range t.CreditLimit {
			if !slices.Contains(accountKinds, kind) || limit < 0 || limit > max {
				return nil, invalid(FieldUsageTerms, "credit_limit is from 0 to the maximum price, per account kind")
			}
		}
	}

	_, hasMonth := req.New[FieldSeatPriceMonth]
	_, hasYear := req.New[FieldSeatPriceYear]
	switch {
	case (hasMonth || hasYear) && p.BillingModel == "usage":
		return nil, invalid(firstSeatField(req.New), "a usage plan has no seat price")
	case (hasMonth || hasYear) && p.Key == "free":
		for _, f := range []string{FieldSeatPriceMonth, FieldSeatPriceYear} {
			if seat[f] != 0 {
				return nil, invalid(f, "the free plan costs 0")
			}
		}
	case hasMonth || hasYear:
		month, year := seatAfter(p, seat, FieldSeatPriceMonth), seatAfter(p, seat, FieldSeatPriceYear)
		if month <= 0 {
			return nil, invalid(FieldSeatPriceMonth, "a paid seat plan has a positive monthly price")
		}
		if year <= 0 {
			return nil, invalid(FieldSeatPriceYear, "a paid seat plan has a positive yearly price")
		}
		if year > 12*month {
			return nil, invalid(FieldSeatPriceYear, "the yearly price is at most 12 times the monthly price")
		}
	}

	old = Prices{}
	for f := range req.New {
		old[f] = p.Prices[f]
		if old[f] == nil {
			old[f] = json.RawMessage("null")
		}
	}
	return old, nil
}

func firstSeatField(n Prices) string {
	if _, ok := n[FieldSeatPriceMonth]; ok {
		return FieldSeatPriceMonth
	}
	return FieldSeatPriceYear
}

// seatAfter is a seat price once the change is applied: the new value, else the
// plan's current one. A null or missing price is 0.
func seatAfter(p PlanRow, seat map[string]int64, field string) int64 {
	if n, ok := seat[field]; ok {
		return n
	}
	n, _ := seatPrice(p.Prices[field])
	return n
}

func touchesUsage(n Prices) bool {
	_, rates := n[FieldUsageRates]
	_, terms := n[FieldUsageTerms]
	return rates || terms
}

// atMidnightUTC reports whether t is 00:00:00 UTC.
func atMidnightUTC(t time.Time) bool {
	t = t.UTC()
	return t.Equal(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))
}

func validateRequest(req Request) *Error {
	if req.EffectiveFrom.IsZero() {
		return invalid("effective_from", "effective_from is required")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(req.Reason)); n == 0 || n > maxReasonChars {
		return invalid("reason", "reason is required and at most 500 characters")
	}
	return nil
}

// unitPrice is the price of one unit of a meter.
func unitPrice(r rate) float64 {
	if r.Per <= 0 {
		return 0
	}
	return float64(r.Price) / float64(r.Per)
}

// valueOf reads a stored or validated price field. A value that does not parse
// reads as empty: only validated input and rows this package wrote reach here.
func ratesOf(raw json.RawMessage) map[string]rate {
	r, _ := parseRates(raw)
	return r
}

func termsOf(raw json.RawMessage) terms {
	t, _ := parseTerms(raw)
	return t
}

// isIncrease reports whether the change raises a price: a seat price or a unit
// price of a meter goes up, or the business discount goes down (rule 7). A
// meter that the old rates lack was free.
func isIncrease(old, next Prices) bool {
	for _, f := range []string{FieldSeatPriceMonth, FieldSeatPriceYear} {
		raw, ok := next[f]
		if !ok {
			continue
		}
		n, _ := seatPrice(raw)
		o, _ := seatPrice(old[f])
		if n > o {
			return true
		}
	}
	if raw, ok := next[FieldUsageRates]; ok {
		oldRates := ratesOf(old[FieldUsageRates])
		for meter, r := range ratesOf(raw) {
			if unitPrice(r) > unitPrice(oldRates[meter]) {
				return true
			}
		}
	}
	if raw, ok := next[FieldUsageTerms]; ok {
		if termsOf(raw).DiscountPct["business"] < termsOf(old[FieldUsageTerms]).DiscountPct["business"] {
			return true
		}
	}
	return false
}

// changePct is the largest relative change, in percent and rounded to two
// decimals, of a price whose current value is not 0 (rule 5).
func changePct(old, next Prices) float64 {
	var biggest float64
	consider := func(o, n float64) {
		if o == 0 {
			return
		}
		if pct := math.Abs(n-o) * 100 / o; pct > biggest {
			biggest = pct
		}
	}
	for _, f := range []string{FieldSeatPriceMonth, FieldSeatPriceYear} {
		if raw, ok := next[f]; ok {
			n, _ := seatPrice(raw)
			o, _ := seatPrice(old[f])
			consider(float64(o), float64(n))
		}
	}
	if raw, ok := next[FieldUsageRates]; ok {
		oldRates := ratesOf(old[FieldUsageRates])
		for meter, r := range ratesOf(raw) {
			consider(unitPrice(oldRates[meter]), unitPrice(r))
		}
	}
	return math.Round(biggest*100) / 100
}
