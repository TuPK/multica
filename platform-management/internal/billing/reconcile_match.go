package billing

import (
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
)

// Review reasons the reconciler gives (spec 7.2, step 7). ambiguous_code and
// unparsable are set by the webhook listener, not here.
const (
	ReasonUnknownCode      = "unknown_code"
	ReasonOrderNotPending  = "order_not_pending"
	ReasonCurrencyMismatch = "currency_mismatch"
	ReasonAmountMismatch   = "amount_mismatch"
	ReasonApplyFailed      = "apply_failed"
)

// matchNotice returns "" when the notice pays the order exactly, and otherwise
// the review reason of the first check that fails. found says whether o is an
// order at all. A partial or excess amount never matches.
func matchNotice(n paymentnotice.Notice, o Order, found bool, now time.Time) string {
	switch {
	case !found:
		return ReasonUnknownCode
	case o.Status != StatusPending || (!o.ExpiresAt.IsZero() && !now.Before(o.ExpiresAt)):
		return ReasonOrderNotPending
	case !strings.EqualFold(n.Currency, o.Currency):
		return ReasonCurrencyMismatch
	case n.Amount != o.Total():
		return ReasonAmountMismatch
	}
	return ""
}
