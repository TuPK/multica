// Package plan holds the plan catalogue: the commercial plans, their limits and
// features, and the rule that turns a plan and a workspace's overrides into an
// effective limit (spec 5.5).
package plan

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// The gates a plan carries a limit for. The entitlement policy serves the first
// seven (spec 5.7); Multica enforces the first five and reads the next two for
// the API rate limit and the audit retention job. GateMemberGroups is read by
// the member-group feature (spec 7b.2) and is not served by the policy yet.
const (
	GateIssueCount        = "issue_count"
	GateAutopilotRuns     = "autopilot_runs"
	GateAgents            = "agents"
	GateRuntimes          = "runtimes"
	GateStorageBytes      = "storage_bytes"
	GateAPIRatePerMinute  = "api_rate_per_minute"
	GateAuditRetentionDay = "audit_retention_days"
)

// GateMemberGroups caps the member groups of a workspace (spec 7b.2, 7b.7).
const GateMemberGroups = "member_groups"

// Audiences: the kind of workspace that may buy a plan (spec 7b.1, rule 2).
const (
	AudienceIndividual = "individual"
	AudienceBusiness   = "business"
	AudienceAny        = "any"
)

// Billing models: how a plan is charged (spec 7b.1; the usage kind is spec 7b.3).
const (
	BillingSeat  = "seat"
	BillingUsage = "usage"
)

// Audiences and BillingModels list the values a plan may carry.
var (
	Audiences     = []string{AudienceIndividual, AudienceBusiness, AudienceAny}
	BillingModels = []string{BillingSeat, BillingUsage}
)

// UnlimitedKey is the catalogue plan with every counted limit off (spec 7b.3,
// rule 1; decision D42).
const UnlimitedKey = "unlimited"

// CountedGates are the limits an unlimited plan leaves empty. The API rate and
// the audit retention are plan values, not counted limits.
var CountedGates = []string{
	GateIssueCount, GateAutopilotRuns, GateAgents, GateRuntimes,
	GateStorageBytes, GateMemberGroups,
}

// IsUnlimited reports whether p sets no counted limit.
func IsUnlimited(p Plan) bool {
	for _, gate := range CountedGates {
		if p.Limits[gate] != nil {
			return false
		}
	}
	return true
}

// FreeKey is the plan of a workspace that has no subscription row.
const FreeKey = "free"

// LimitGates lists every key a plan's limits or a workspace's overrides may use.
var LimitGates = []string{
	GateIssueCount, GateAutopilotRuns, GateAgents, GateRuntimes,
	GateStorageBytes, GateAPIRatePerMinute, GateAuditRetentionDay,
	GateMemberGroups,
}

// FeatureKeys lists every key a plan's features may use.
var FeatureKeys = []string{"audit_export", "sso", "workspace_policies", "viewer_role"}

// Limits maps a gate to its limit. A nil value, like an absent key, is
// unlimited. In a workspace's overrides a nil value is meaningful: it lifts the
// plan's limit.
type Limits map[string]*int64

// Plan is one row of the plan table. Prices are whole units of the currency's
// smallest denomination; VND has no decimals.
//
// Audience and BillingModel may be empty on a write, which means "not given":
// a new plan takes the defaults (any, seat) and an existing plan keeps its value.
// Version is the table's: 0 on creation, one more after every edit. A write
// never sets it.
//
// UsageRates and UsageTerms (spec 7b.3) are read from the catalogue file and
// written by the seed only. A plan read from the table does not carry them, and
// an edit through UpsertPlan leaves the stored values alone: price changes
// reach them through plan_price_change.
type Plan struct {
	Key            string          `json:"key"`
	Name           string          `json:"name"`
	Limits         Limits          `json:"limits"`
	Features       map[string]bool `json:"features"`
	Currency       string          `json:"currency"`
	SeatPriceMonth *int64          `json:"seat_price_month"`
	SeatPriceYear  *int64          `json:"seat_price_year"`
	SeatsFixed     *int            `json:"seats_fixed"`
	MinSeats       *int            `json:"min_seats"`
	TrialDays      int             `json:"trial_days"`
	IsPublic       bool            `json:"is_public"`
	Sort           int             `json:"sort"`
	ArchivedAt     *time.Time      `json:"archived_at"`
	Audience       string          `json:"audience"`
	BillingModel   string          `json:"billing_model"`
	Version        int64           `json:"version"`
	UsageRates     json.RawMessage `json:"usage_rates,omitempty"`
	UsageTerms     json.RawMessage `json:"usage_terms,omitempty"`
}

//go:embed catalogue.json
var catalogueJSON []byte

// Catalogue returns the embedded initial plans, in display order.
func Catalogue() ([]Plan, error) {
	var file struct {
		Plans []Plan `json:"plans"`
	}
	dec := json.NewDecoder(bytes.NewReader(catalogueJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("plan catalogue: %w", err)
	}
	if len(file.Plans) == 0 {
		return nil, errors.New("plan catalogue: no plans")
	}
	return file.Plans, nil
}

var (
	keyPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	currencyPattern = regexp.MustCompile(`^[a-z]{3}$`)
)

const (
	maxNameChars  = 100
	maxTrialDays  = 365
	maxSeatsValue = 1_000_000
)

// summaryGates are the gates the billing page's schema shows. That schema holds
// a positive limit or "unlimited" and cannot show 0, so these two may not be 0.
func summaryGate(gate string) bool { return gate == GateIssueCount || gate == GateAutopilotRuns }

// ValidateLimits checks the keys and values of a plan's limits or a workspace's
// overrides.
func ValidateLimits(l Limits) error {
	for gate, v := range l {
		if !slices.Contains(LimitGates, gate) {
			return fmt.Errorf("unknown limit %q", gate)
		}
		if v == nil {
			continue
		}
		if *v < 0 {
			return fmt.Errorf("limit %s must not be negative", gate)
		}
		if *v == 0 && summaryGate(gate) {
			return fmt.Errorf("limit %s must be positive, or null for unlimited", gate)
		}
	}
	return nil
}

// ValidateKey checks a plan key.
func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return errors.New("key must be 1 to 32 lower-case letters, digits or underscores, starting with a letter")
	}
	return nil
}

// Validate checks a plan before it is stored.
func Validate(p Plan) error {
	if err := ValidateKey(p.Key); err != nil {
		return err
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(p.Name)); n == 0 || n > maxNameChars || strings.ContainsRune(p.Name, 0) {
		return errors.New("name is required and at most 100 characters")
	}
	if !currencyPattern.MatchString(p.Currency) {
		return errors.New("currency must be a three-letter lower-case code")
	}
	for name, v := range map[string]*int64{"seat_price_month": p.SeatPriceMonth, "seat_price_year": p.SeatPriceYear} {
		if v != nil && *v < 0 {
			return fmt.Errorf("%s must not be negative", name)
		}
	}
	if p.SeatsFixed != nil && (*p.SeatsFixed < 0 || *p.SeatsFixed > maxSeatsValue) {
		return errors.New("seats_fixed must be 0 or more")
	}
	if p.MinSeats != nil && (*p.MinSeats < 1 || *p.MinSeats > maxSeatsValue) {
		return errors.New("min_seats must be 1 or more")
	}
	if p.SeatsFixed != nil && p.MinSeats != nil {
		return errors.New("a plan has seats_fixed or min_seats, not both")
	}
	if p.TrialDays < 0 || p.TrialDays > maxTrialDays {
		return errors.New("trial_days must be from 0 to 365")
	}
	if p.Audience != "" && !slices.Contains(Audiences, p.Audience) {
		return fmt.Errorf("unknown audience %q", p.Audience)
	}
	if p.BillingModel != "" && !slices.Contains(BillingModels, p.BillingModel) {
		return fmt.Errorf("unknown billing_model %q", p.BillingModel)
	}
	if p.BillingModel == BillingUsage && (p.SeatPriceMonth != nil || p.SeatPriceYear != nil) {
		return errors.New("a usage plan has no seat_price_month or seat_price_year")
	}
	if p.Version < 0 {
		return errors.New("version must not be negative")
	}
	if err := ValidateLimits(p.Limits); err != nil {
		return err
	}
	if p.Key == UnlimitedKey && !IsUnlimited(p) {
		return errors.New("the unlimited plan has no counted limit")
	}
	for f := range p.Features {
		if !slices.Contains(FeatureKeys, f) {
			return fmt.Errorf("unknown feature %q", f)
		}
	}
	return nil
}

// EffectiveLimit returns the limit that applies to gate: the workspace's
// override when it has one for the gate, otherwise the plan's limit. A nil
// result is unlimited.
func EffectiveLimit(plan, overrides Limits, gate string) *int64 {
	if v, ok := overrides[gate]; ok {
		return v
	}
	return plan[gate]
}

// DefaultAPIRateCeiling is the API rate per minute that an empty rate on a
// limited plan or override becomes, when the platform sets no ceiling of its
// own (PLATFORM_API_RATE_CEILING, spec 7b.3 rule 2).
const DefaultAPIRateCeiling int64 = 6000

// EffectiveAPIRate returns the API rate per minute that applies to a workspace:
// the override or the plan's own value when it is set, and used as it is even
// above the ceiling. An empty rate on a limited plan is the ceiling; an
// unlimited plan's cap is off (nil), not a number (contract section 2).
func EffectiveAPIRate(p Plan, overrides Limits, ceiling int64) *int64 {
	if v := EffectiveLimit(p.Limits, overrides, GateAPIRatePerMinute); v != nil {
		return v
	}
	if IsUnlimited(p) {
		return nil
	}
	c := ceiling
	return &c
}

// Reader reads plans.
type Reader interface {
	// Get returns found == false for an unknown key.
	Get(ctx context.Context, key string) (p Plan, found bool, err error)
	// List returns every plan, archived ones included, in display order.
	List(ctx context.Context) ([]Plan, error)
}
