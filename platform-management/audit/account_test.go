package audit

import "testing"

// The account request actions are stored in audit_event.action (spec 7b.5, the
// audit table), so a rename is a data change.
func TestAccountProvisionActionsAreDefined(t *testing.T) {
	cases := map[string]string{
		"account.provision_requested": ActionAccountProvisionRequested,
		"account.provision_canceled":  ActionAccountProvisionCanceled,
		"account.provision_failed":    ActionAccountProvisionFailed,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		found := false
		for _, a := range Catalog {
			found = found || a == want
		}
		if !found {
			t.Errorf("%s is not in Catalog", want)
		}
	}
}

func TestBillingProfileUpdatedActionIsDefined(t *testing.T) {
	if ActionBillingProfileUpdated != "billing_profile.updated" {
		t.Fatalf("constant = %q", ActionBillingProfileUpdated)
	}
	for _, a := range Catalog {
		if a == ActionBillingProfileUpdated {
			return
		}
	}
	t.Fatal("billing_profile.updated is not in Catalog")
}
