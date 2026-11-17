package money

import "fmt"

// exponents maps an enabled currency to its minor-unit exponent. vnd has no
// minor unit, so an amount is a whole number of dong.
var exponents = map[string]int{
	"vnd": 0,
}

// Exponent returns the minor-unit exponent of an enabled currency. The match is
// exact and lowercase; any other code is an error.
func Exponent(code string) (int, error) {
	e, ok := exponents[code]
	if !ok {
		return 0, fmt.Errorf("money: currency %q is not enabled", code)
	}
	return e, nil
}
