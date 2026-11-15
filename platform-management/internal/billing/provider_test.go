package billing

import (
	"errors"
	"net/http"
	"testing"
)

func testBank() BankDetails {
	return BankDetails{AccountName: "CONG TY TNHH MULTICA", AccountNumber: "0123456789", BankName: "Vietcombank"}
}

func testOrder() Order {
	return Order{
		ID: "11111111-1111-1111-1111-111111111111", WorkspaceID: "22222222-2222-2222-2222-222222222222",
		Kind: KindSubscribe, PlanKey: "pro", Interval: "month", Seats: 3,
		Amount: 447000, VATAmount: 44700, Currency: "vnd", Status: StatusPending,
		Provider: ProviderManual, TransferCode: "MULABCDEFGH",
	}
}

func TestManualProviderInstructions(t *testing.T) {
	var p PaymentProvider = NewManualProvider(testBank())
	got, err := p.CreatePaymentRequest(testOrder())
	if err != nil {
		t.Fatal(err)
	}
	want := Instructions{
		Provider: "manual", BankAccountName: "CONG TY TNHH MULTICA", BankAccountNumber: "0123456789",
		BankName: "Vietcombank", TransferCode: "MULABCDEFGH", Amount: 491700, Currency: "vnd",
	}
	if got != want {
		t.Fatalf("Instructions = %+v, want %+v", got, want)
	}
}

func TestManualProviderRefusesAnOrderWithoutACode(t *testing.T) {
	o := testOrder()
	o.TransferCode = ""
	if _, err := NewManualProvider(testBank()).CreatePaymentRequest(o); err == nil {
		t.Fatal("accepted an order with no transfer code")
	}
}

func TestManualProviderUnsupported(t *testing.T) {
	p := NewManualProvider(testBank())
	if _, err := p.ParseNotification([]byte(`{}`), http.Header{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ParseNotification err = %v, want ErrUnsupported", err)
	}
	if _, err := p.QueryStatus(testOrder()); !errors.Is(err, ErrUnsupported) {
		t.Errorf("QueryStatus err = %v, want ErrUnsupported", err)
	}
}
