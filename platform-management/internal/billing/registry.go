package billing

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoProvider means no payment provider is configured for the currency.
var ErrNoProvider = errors.New("billing: no payment provider for this currency")

// Registry maps a currency to its payment provider.
type Registry struct {
	byCurrency map[string]string
	providers  map[string]PaymentProvider
}

// NewRegistry builds a registry from a spec of "currency=provider" pairs
// separated by commas; an empty spec means "vnd=manual". secret returns the
// webhook secret of an automatic provider, which must not be empty.
func NewRegistry(spec string, bank BankDetails, secret func(provider string) string) (*Registry, error) {
	if strings.TrimSpace(spec) == "" {
		spec = "vnd=" + ProviderManual
	}
	r := &Registry{byCurrency: map[string]string{}, providers: map[string]PaymentProvider{}}
	for _, pair := range strings.Split(spec, ",") {
		cur, name, ok := strings.Cut(pair, "=")
		cur = strings.ToLower(strings.TrimSpace(cur))
		name = strings.TrimSpace(name)
		if !ok || cur == "" {
			return nil, fmt.Errorf("billing: bad payment provider entry %q", strings.TrimSpace(pair))
		}
		if _, dup := r.byCurrency[cur]; dup {
			return nil, fmt.Errorf("billing: currency %q is set twice", cur)
		}
		if _, built := r.providers[name]; !built {
			switch name {
			case ProviderManual:
				r.providers[name] = NewManualProvider(bank)
			case ProviderSePay:
				s := secret(name)
				if s == "" {
					return nil, fmt.Errorf("billing: provider %q needs a webhook secret", name)
				}
				r.providers[name] = NewSePayProvider(bank, s)
			default:
				return nil, fmt.Errorf("billing: unknown payment provider %q", name)
			}
		}
		r.byCurrency[cur] = name
	}
	return r, nil
}

// ForCurrency returns the provider for the currency and the provider's name.
func (r *Registry) ForCurrency(currency string) (PaymentProvider, string, error) {
	name, ok := r.byCurrency[strings.ToLower(currency)]
	if !ok {
		return nil, "", ErrNoProvider
	}
	return r.providers[name], name, nil
}

// ByName returns a configured provider by name.
func (r *Registry) ByName(name string) (PaymentProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}
