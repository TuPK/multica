package config

import "testing"

// PLATFORM_ACCOUNT_CREATE_PER_HOUR (default 30) and PLATFORM_INDIVIDUAL_MAX_SEATS
// (default 5) are spec 3.4 values; both are whole numbers of 1 or more.
func TestLoadAccountRequestLimits(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccountCreatePerHour != 30 {
		t.Errorf("default AccountCreatePerHour = %d, want 30", cfg.AccountCreatePerHour)
	}
	if cfg.IndividualMaxSeats != 5 {
		t.Errorf("default IndividualMaxSeats = %d, want 5", cfg.IndividualMaxSeats)
	}

	env := resendEnv()
	env["PLATFORM_ACCOUNT_CREATE_PER_HOUR"] = " 12 "
	env["PLATFORM_INDIVIDUAL_MAX_SEATS"] = "8"
	cfg, err = load(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccountCreatePerHour != 12 || cfg.IndividualMaxSeats != 8 {
		t.Errorf("got %d and %d, want 12 and 8", cfg.AccountCreatePerHour, cfg.IndividualMaxSeats)
	}
}

func TestLoadAccountRequestLimitsRefuseBadNumbers(t *testing.T) {
	for _, name := range []string{"PLATFORM_ACCOUNT_CREATE_PER_HOUR", "PLATFORM_INDIVIDUAL_MAX_SEATS"} {
		for _, raw := range []string{"abc", "1.5", "0", "-1", "99999999999999999999999"} {
			env := resendEnv()
			env[name] = raw
			_, err := load(env)
			if got := variables(err); len(got) != 1 || got[0] != name {
				t.Errorf("%s=%q: variables = %v, want [%s] (err %v)", name, raw, got, name, err)
			}
		}
	}
}
