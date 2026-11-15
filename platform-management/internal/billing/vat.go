package billing

import (
	"errors"
	"math/big"
)

// WithVAT returns the VAT on amount and the total, in whole VND. The VAT is
// amount x rate rounded half up. amount is the price before VAT.
func WithVAT(amount int64, rate Rate) (vat, total int64, err error) {
	if amount < 0 {
		return 0, 0, errors.New("billing: amount must not be negative")
	}
	if !rate.valid() {
		return 0, 0, errors.New("billing: the VAT rate is not set")
	}
	n := new(big.Int).Mul(big.NewInt(amount), big.NewInt(rate.num))
	v := divRoundHalfUp(n, big.NewInt(rate.den))
	t := v.Add(v, big.NewInt(amount))
	if !t.IsInt64() {
		return 0, 0, errors.New("billing: amount with VAT is too large")
	}
	total = t.Int64()
	return total - amount, total, nil
}
