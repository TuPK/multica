package billing

import (
	"errors"
	"testing"
)

func noSecret(string) string { return "" }

func TestRegistryDefaultsToManual(t *testing.T) {
	r, err := NewRegistry("", testBank(), noSecret)
	if err != nil {
		t.Fatal(err)
	}
	p, name, err := r.ForCurrency("vnd")
	if err != nil {
		t.Fatal(err)
	}
	if name != ProviderManual {
		t.Fatalf("provider for vnd = %q, want %q", name, ProviderManual)
	}
	if _, ok := p.(*Manual); !ok {
		t.Fatalf("provider for vnd is %T, want *Manual", p)
	}
}

func TestStartupRejectsAutoProviderWithoutSecret(t *testing.T) {
	if _, err := NewRegistry("vnd=sepay", testBank(), noSecret); err == nil {
		t.Fatal("accepted an automatic provider with no webhook secret")
	}
	secret := func(name string) string {
		if name == ProviderSePay {
			return testSePaySecret
		}
		return ""
	}
	r, err := NewRegistry("vnd=sepay", testBank(), secret)
	if err != nil {
		t.Fatalf("with a secret: %v", err)
	}
	if _, ok := r.ByName(ProviderSePay); !ok {
		t.Fatal("ByName(sepay) not found after a valid start")
	}
}

func TestRegistryRejectsUnknownProviderName(t *testing.T) {
	if _, err := NewRegistry("vnd=nosuch", testBank(), noSecret); err == nil {
		t.Fatal("accepted an unknown provider name")
	}
}

func TestRegistryRejectsDuplicateCurrencyMapping(t *testing.T) {
	if _, err := NewRegistry("vnd=manual,vnd=manual", testBank(), noSecret); err == nil {
		t.Fatal("accepted a currency mapped to two providers")
	}
}

func TestForCurrencyWithoutProviderReturnsErrNoProvider(t *testing.T) {
	r, err := NewRegistry("vnd=manual", testBank(), noSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.ForCurrency("usd"); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("ForCurrency(usd) error = %v, want ErrNoProvider", err)
	}
}

func TestRegistryMapsEachCurrencyToItsProvider(t *testing.T) {
	secret := func(name string) string {
		if name == ProviderSePay {
			return testSePaySecret
		}
		return ""
	}
	r, err := NewRegistry("vnd=manual,usd=sepay", testBank(), secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, name, err := r.ForCurrency("usd"); err != nil || name != ProviderSePay {
		t.Fatalf("provider for usd = %q, %v; want %q", name, err, ProviderSePay)
	}
	if _, name, err := r.ForCurrency("vnd"); err != nil || name != ProviderManual {
		t.Fatalf("provider for vnd = %q, %v; want %q", name, err, ProviderManual)
	}
}
