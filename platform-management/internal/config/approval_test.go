package config

import (
	"slices"
	"testing"
)

// Spec R6 section 3c.6 rule 1: the approval list is empty by default and accepts
// only the two deletion actions; a typo fails the start.
func TestApprovalActionsDefaultEmptyAndKnownOnly(t *testing.T) {
	var errs []string
	fail := func(v, r string) { errs = append(errs, v+": "+r) }
	if got := approvalActions("", fail); len(got) != 0 || len(errs) != 0 {
		t.Fatalf("empty = %v, errs %v", got, errs)
	}
	got := approvalActions(" tenant.delete_now , account.delete_now", fail)
	if !slices.Equal(got, []string{"tenant.delete_now", "account.delete_now"}) || len(errs) != 0 {
		t.Fatalf("known = %v, errs %v", got, errs)
	}
	approvalActions("tenant.delet_now", fail)
	if len(errs) != 1 {
		t.Fatalf("a typo must fail: %v", errs)
	}
}
