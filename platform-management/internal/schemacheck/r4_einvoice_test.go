package schemacheck

import (
	"slices"
	"testing"
)

// The R4 invoice table (spec 7.5, P43.2) is platform-owned and is read and
// written by the platform service, so its migrations must be in Required.
// TestEveryPlatformTableMigrationIsRequired scans 651 to 699 only; this test
// covers the 1011 to 1013 range that P43.2 allocates.
func TestEinvoiceMigrationsAreRequired(t *testing.T) {
	const lo, hi = 1011, 1013
	probe, err := unlistedMigrations(migrationsDir, lo, hi, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(probe) != 3 {
		t.Fatalf("scan found %v in %d to %d; want the three einvoice_issuance migrations", probe, lo, hi)
	}
	unlisted, err := unlistedMigrations(migrationsDir, lo, hi, Required)
	if err != nil {
		t.Fatal(err)
	}
	if len(unlisted) > 0 {
		t.Fatalf("einvoice_issuance migrations missing from schemacheck.Required: %v", unlisted)
	}
	for _, v := range []string{
		"1011_einvoice_issuance",
		"1012_einvoice_issuance_order_uidx",
		"1013_einvoice_issuance_due_idx",
	} {
		if !slices.Contains(Required, v) {
			t.Errorf("Required lacks %s", v)
		}
	}
}
