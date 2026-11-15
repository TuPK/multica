// Package billing is the platform's billing core: pricing, VAT, proration, the
// order state machine, the payment provider interface with its manual
// implementation, and the transaction that applies a confirmed payment to a
// workspace's subscription (spec 5.8).
//
// Money is integer VND. No amount is ever held in a floating-point number.
package billing

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// maxRateDigits bounds the decimal places of a rate, so a rate is always an
// exact fraction over at most 10^6.
const maxRateDigits = 6

// Rate is a ratio between 0 and 1 held as an exact fraction, so VAT is computed
// in integers. The zero value is not a rate; build one with ParseRate.
type Rate struct {
	num, den int64
}

// DefaultVATRate is the VAT rate when BILLING_VAT_RATE is not set: 10%.
var DefaultVATRate = Rate{num: 1, den: 10}

// ParseRate reads a decimal such as "0.10" between 0 and 1 inclusive, with at
// most six decimal places. It accepts nothing else: no sign, exponent, percent
// sign, or missing integer or fraction part.
func ParseRate(s string) (Rate, error) {
	s = strings.TrimSpace(s)
	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole != "0" && whole != "1" {
		return Rate{}, errors.New("must be a decimal from 0 to 1, such as 0.10")
	}
	if hasFrac && (frac == "" || len(frac) > maxRateDigits) {
		return Rate{}, fmt.Errorf("must be a decimal from 0 to 1 with at most %d decimal places", maxRateDigits)
	}
	var num, den int64 = 0, 1
	for _, c := range whole + frac {
		if c < '0' || c > '9' {
			return Rate{}, errors.New("must be a decimal from 0 to 1, such as 0.10")
		}
		num = num*10 + int64(c-'0')
	}
	for range frac {
		den *= 10
	}
	if num > den {
		return Rate{}, errors.New("must not be more than 1")
	}
	return Rate{num: num, den: den}, nil
}

func (r Rate) valid() bool { return r.den > 0 }

// divRoundHalfUp returns n/d rounded to the nearest integer, halves up. n is
// not negative and d is positive.
func divRoundHalfUp(n, d *big.Int) *big.Int {
	twice := new(big.Int).Lsh(n, 1)
	twice.Add(twice, d)
	return twice.Div(twice, new(big.Int).Lsh(d, 1))
}
