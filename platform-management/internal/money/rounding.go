package money

import (
	"errors"
	"math/big"
)

// RoundHalfUp returns num/den rounded to the nearest integer. A tie rounds up,
// which is away from zero for non-negative num. den must be positive. The
// arguments are not modified.
func RoundHalfUp(num, den *big.Int) (*big.Int, error) {
	if den.Sign() <= 0 {
		return nil, errors.New("money: divisor must be positive")
	}
	// floor((2*num + den) / (2*den)) is round-half-up using integers only.
	twice := new(big.Int).Lsh(num, 1)
	twice.Add(twice, den)
	return twice.Div(twice, new(big.Int).Lsh(den, 1)), nil
}
