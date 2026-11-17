package plan

import (
	"fmt"
	"slices"
)

// ValidateCurrency refuses a currency that is not in the enabled set
// (PLATFORM_CURRENCIES). The match is exact; currencies are lower case.
func ValidateCurrency(currency string, enabled []string) error {
	if !slices.Contains(enabled, currency) {
		return fmt.Errorf("currency %q is not enabled on this platform", currency)
	}
	return nil
}
