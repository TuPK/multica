package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/platform-management/internal/money"
)

// ErrNoProvider means no payment provider is configured for the currency.
var ErrNoProvider = errors.New("billing: no payment provider for this currency")

// Registry maps a currency to its payment provider.
type Registry struct {
	byCurrency map[string]string
	providers  map[string]PaymentProvider
}

// NewRegistry builds a registry from a spec of "currency=provider" pairs
// separated by commas; an empty spec maps the default currency to the manual
// provider. secret returns the
// webhook secret of an automatic provider, which must not be empty.
func NewRegistry(spec string, bank BankDetails, secret func(provider string) string) (*Registry, error) {
	if strings.TrimSpace(spec) == "" {
		spec = money.Default + "=" + ProviderManual
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
				sepay := NewSePayProvider(bank, s)
				sepay.currency = strings.ToUpper(cur)
				r.providers[name] = sepay
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

// OrderCreator creates pending orders.
type OrderCreator interface {
	CreateOrder(ctx context.Context, d Draft) (Order, bool, error)
}

// CurrencyGate refuses an order whose currency has no provider before Next
// writes anything.
type CurrencyGate struct {
	Registry *Registry
	Next     OrderCreator
}

func (g CurrencyGate) CreateOrder(ctx context.Context, d Draft) (Order, bool, error) {
	if _, _, err := g.Registry.ForCurrency(d.Currency); err != nil {
		return Order{}, false, err
	}
	return g.Next.CreateOrder(ctx, d)
}
