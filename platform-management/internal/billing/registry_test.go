package billing

import "testing"

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
