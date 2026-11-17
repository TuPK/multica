package billing

import (
	"context"
	"net/http"
	"reflect"
	"sort"
	"testing"
)

// fakeProvider stands in for a second payment provider. It exists only in
// tests: no second real provider is wired (spec 7.4, O-R4-5).
type fakeProvider struct{}

func (fakeProvider) CreatePaymentRequest(o Order) (Instructions, error) {
	return Instructions{Provider: "fake", Currency: o.Currency}, nil
}

func (fakeProvider) ParseNotification([]byte, http.Header) (Notification, error) {
	return Notification{}, ErrUnsupported
}

func (fakeProvider) QueryStatus(Order) (Status, error) { return "", ErrUnsupported }

func TestInterfaceUnchanged(t *testing.T) {
	typ := reflect.TypeOf((*PaymentProvider)(nil)).Elem()
	var got []string
	for i := 0; i < typ.NumMethod(); i++ {
		got = append(got, typ.Method(i).Name)
	}
	sort.Strings(got)
	want := []string{"CreatePaymentRequest", "ParseNotification", "QueryStatus"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PaymentProvider methods = %v, want %v", got, want)
	}
}

func TestSecondFakeProviderEnabledCreatesOrderInItsCurrency(t *testing.T) {
	fake := fakeProvider{}
	r := &Registry{
		byCurrency: map[string]string{"usd": "fake"},
		providers:  map[string]PaymentProvider{"fake": fake},
	}
	p, name, err := r.ForCurrency("usd")
	if err != nil {
		t.Fatal(err)
	}
	if name != "fake" {
		t.Fatalf("provider name for usd = %q, want fake", name)
	}
	if _, ok := p.(fakeProvider); !ok {
		t.Fatalf("provider for usd is %T, want fakeProvider", p)
	}

	next := &recordingOrders{}
	gate := CurrencyGate{Registry: r, Next: next}
	if _, _, err := gate.CreateOrder(context.Background(), Draft{Currency: "usd"}); err != nil {
		t.Fatalf("CreateOrder(usd) error = %v", err)
	}
	if next.calls != 1 {
		t.Fatalf("usd draft written %d times, want 1", next.calls)
	}
}

func TestSecondFakeProviderDisabledChangesNothing(t *testing.T) {
	r, err := NewRegistry("vnd=manual", testBank(), noSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.ByName("fake"); ok {
		t.Fatal("fake provider is present in a registry built from config")
	}
	p, name, err := r.ForCurrency("vnd")
	if err != nil {
		t.Fatal(err)
	}
	if name != ProviderManual {
		t.Fatalf("provider name for vnd = %q, want %q", name, ProviderManual)
	}
	if _, ok := p.(*Manual); !ok {
		t.Fatalf("provider for vnd is %T, want *Manual", p)
	}

	next := &recordingOrders{}
	gate := CurrencyGate{Registry: r, Next: next}
	if _, _, err := gate.CreateOrder(context.Background(), Draft{Currency: "vnd"}); err != nil {
		t.Fatalf("CreateOrder(vnd) error = %v", err)
	}
	if next.calls != 1 {
		t.Fatalf("vnd draft written %d times, want 1", next.calls)
	}
}
