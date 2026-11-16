package usage

import (
	"errors"
	"fmt"
	"math/big"
	"time"
)

// The three meters of a pay as you go plan (platform management spec 7b.3,
// rule 7). Adding a meter adds a constant here, a source column in the usage
// snapshot, and a test.
const (
	MeterTasksRun     = "tasks_run"
	MeterStorageBytes = "storage_bytes"
	MeterMembers      = "members"
)

// Meters lists the meters in the order their rows are written.
var Meters = []string{MeterTasksRun, MeterStorageBytes, MeterMembers}

// Workspace statuses the rating reads (platform spec 5.3).
const (
	StatusActive          = "active"
	StatusReadOnly        = "read_only"
	StatusSuspended       = "suspended"
	StatusPendingDeletion = "pending_deletion"
)

// NoteWorkspaceSuspended is the note on a zero row of a day whose workspace is
// suspended or pending deletion (spec 7b.3, rule 10).
const NoteWorkspaceSuspended = "workspace_suspended"

const accountBusiness = "business"

// Rate is a plan's price for a meter: Price for Per units. It is the
// usage_rates entry of the plan row.
type Rate struct {
	Per   int64 `json:"per"`
	Price int64 `json:"price"`
}

// Rates maps a meter to its rate. A meter the map does not name costs nothing.
type Rates map[string]Rate

// Terms is the usage_terms of a plan. DiscountPct["business"] is the Business
// discount in whole percent (spec 7b.3, rule 9).
//
// CreditLimit maps an account kind to the amount of uninvoiced usage at which a
// temporary invoice is issued (rule 12). A kind it does not name has no limit.
type Terms struct {
	DiscountPct map[string]int64 `json:"discount_pct"`
	CreditLimit map[string]int64 `json:"credit_limit"`
}

// DaysFor returns the number of days a meter's day is a share of: 1 for
// tasks_run, a flow over the day, and the length of the UTC month that holds
// the day for the two stock meters (rule 8).
func DaysFor(meter string, day time.Time) int64 {
	if meter == MeterTasksRun {
		return 1
	}
	y, m, _ := day.UTC().Date()
	return int64(time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day())
}

// Charge returns the net amount of one row of a meter:
//
//	round_half_up(quantity * price * (100 - discount) / (per * days * 100))
//
// with one rounding, at the end. The arithmetic is big.Int, so a quantity of
// 100 TB at any listed price does not overflow; a result that does not fit an
// int64 is an error.
func Charge(quantity int64, r Rate, discountPct, days int64) (int64, error) {
	switch {
	case quantity < 0:
		return 0, errors.New("quantity is negative")
	case r.Price < 0:
		return 0, errors.New("price is negative")
	case r.Per <= 0:
		return 0, errors.New("per must be positive")
	case discountPct < 0 || discountPct > 100:
		return 0, errors.New("discount must be from 0 to 100 percent")
	case days <= 0:
		return 0, errors.New("days must be positive")
	}
	num := new(big.Int).Mul(big.NewInt(quantity), big.NewInt(r.Price))
	num.Mul(num, big.NewInt(100-discountPct))
	den := new(big.Int).Mul(big.NewInt(r.Per), big.NewInt(days))
	den.Mul(den, big.NewInt(100))

	// floor((2 num + den) / (2 den)) is num/den rounded half up.
	twice := new(big.Int).Lsh(num, 1)
	twice.Add(twice, den)
	twice.Quo(twice, new(big.Int).Lsh(den, 1))
	if !twice.IsInt64() {
		return 0, errors.New("charge does not fit an int64")
	}
	return twice.Int64(), nil
}

// BusinessDiscountPct returns the discount a workspace gets on its usage
// charges (spec 7b.3, rule 9; D45). It applies only to a business account, and
// when requireVerification is set, only to a verified profile. Otherwise it is
// 0.
func BusinessDiscountPct(t Terms, accountKind string, verified, requireVerification bool) int64 {
	if accountKind != accountBusiness {
		return 0
	}
	if requireVerification && !verified {
		return 0
	}
	return t.DiscountPct[accountBusiness]
}

// Input is what one day of one workspace is rated from. Quantities holds the
// meters' counts for the day, keyed by meter.
type Input struct {
	Day         time.Time
	PlanKey     string
	Rates       Rates
	DiscountPct int64
	Status      string
	Quantities  map[string]int64
}

// Row is one usage_charge row, before it is written. It keeps the price it was
// rated at.
type Row struct {
	Meter       string
	Quantity    int64
	PlanKey     string
	UnitPrice   int64
	Per         int64
	DiscountPct int64
	NetAmount   int64
	Note        string
}

// RateDay returns one row per meter for a day, in Meters order, including the
// zero rows (rule 10). A suspended or pending deletion workspace gets zero
// amounts and the note; its quantities are still written, so the numbers can be
// checked.
func RateDay(in Input) ([]Row, error) {
	rows := make([]Row, 0, len(Meters))
	for _, meter := range Meters {
		quantity := in.Quantities[meter]
		if quantity < 0 {
			return nil, fmt.Errorf("meter %s: quantity %d is negative", meter, quantity)
		}
		rate, ok := in.Rates[meter]
		if !ok {
			rate = Rate{Per: 1}
		}
		net, err := Charge(quantity, rate, in.DiscountPct, DaysFor(meter, in.Day))
		if err != nil {
			return nil, fmt.Errorf("meter %s: %w", meter, err)
		}
		row := Row{
			Meter: meter, Quantity: quantity, PlanKey: in.PlanKey,
			UnitPrice: rate.Price, Per: rate.Per, DiscountPct: in.DiscountPct,
		}
		if in.Status == StatusSuspended || in.Status == StatusPendingDeletion {
			net = 0
			row.Note = NoteWorkspaceSuspended
		}
		row.NetAmount = net
		rows = append(rows, row)
	}
	return rows, nil
}
