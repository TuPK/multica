package usage

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

func TestUsageInvoiceUsesPlanCurrency(t *testing.T) {
	rows := []db.ListUninvoicedUsageChargesRow{{Day: pgtype.Date{Time: day("2026-10-30"), Valid: true}, NetAmount: 5000}}
	d, err := invoiceDraft("11111111-1111-4111-8111-111111111111", "payg", "usd", rows, tenPercent(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Currency != "usd" {
		t.Fatalf("invoice Currency = %q, want the plan's usd", d.Currency)
	}
}
