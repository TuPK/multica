package schemacheck

import (
	"slices"
	"testing"
)

// The R4 payment notice tables (spec 7.5, P42.2) are platform-owned and are
// read and written by the platform service, so their migrations must be in
// Required. TestEveryPlatformTableMigrationIsRequired scans 651 to 699 only;
// this test covers the 1007 to 1010 range that P42.2 allocates.
func TestPaymentNoticeMigrationsAreRequired(t *testing.T) {
	const lo, hi = 1007, 1010
	probe, err := unlistedMigrations(migrationsDir, lo, hi, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(probe) != 4 {
		t.Fatalf("scan found %v in %d to %d; want the four payment_notice migrations", probe, lo, hi)
	}
	unlisted, err := unlistedMigrations(migrationsDir, lo, hi, Required)
	if err != nil {
		t.Fatal(err)
	}
	if len(unlisted) > 0 {
		t.Fatalf("payment_notice migrations missing from schemacheck.Required: %v", unlisted)
	}
	for _, v := range []string{
		"1007_payment_notice",
		"1008_payment_notice_event_uidx",
		"1009_payment_notice_id_uidx",
		"1010_payment_notice_review_idx",
	} {
		if !slices.Contains(Required, v) {
			t.Errorf("Required lacks %s", v)
		}
	}
}
