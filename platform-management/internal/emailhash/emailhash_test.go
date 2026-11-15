package emailhash

import "testing"

func TestOfIgnoresCaseAndSpace(t *testing.T) {
	a := Of("Ops@Example.com ")
	b := Of("ops@example.com")
	if a != b {
		t.Fatalf("%q != %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("len = %d, want 64 hex characters", len(a))
	}
	if Of("other@example.com") == a {
		t.Error("different emails share a hash")
	}
}
