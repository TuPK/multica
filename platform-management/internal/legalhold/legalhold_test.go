package legalhold

import "testing"

// Spec R6 section 3c.7 rule 2: a known scope kind, a reason and a case reference.
func TestCheckInputRequiresKindReasonAndCaseRef(t *testing.T) {
	if _, _, e := CheckInput(KindWorkspace, "court order", "CASE-7"); e != nil {
		t.Fatalf("valid input refused: %+v", e)
	}
	for _, c := range []struct{ kind, reason, caseRef string }{
		{"planet", "r", "c"},
		{KindTenant, "  ", "c"},
		{KindTenant, "r", ""},
	} {
		if _, _, e := CheckInput(c.kind, c.reason, c.caseRef); e == nil || e.Code != CodeInvalid {
			t.Errorf("%+v must be refused as %s, got %+v", c, CodeInvalid, e)
		}
	}
}
