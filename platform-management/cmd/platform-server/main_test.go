package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/config"
)

func TestWarnEmptyBankLogsOneWarningNamingTheEmptyVariables(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	warnEmptyBank(log, config.Billing{Bank: billing.BankDetails{AccountName: "ACME"}})
	out := buf.String()
	if strings.Count(out, "level=WARN") != 1 || strings.Count(out, "\n") != 1 {
		t.Fatalf("want one warning line:\n%s", out)
	}
	for _, name := range []string{"BILLING_BANK_ACCOUNT_NUMBER", "BILLING_BANK_NAME"} {
		if !strings.Contains(out, name) {
			t.Errorf("warning does not name %s:\n%s", name, out)
		}
	}
	if strings.Contains(out, "BILLING_BANK_ACCOUNT_NAME") || strings.Contains(out, "ACME") {
		t.Errorf("warning names a set variable or its value:\n%s", out)
	}
}

func TestWarnEmptyBankIsSilentWhenComplete(t *testing.T) {
	var buf bytes.Buffer
	warnEmptyBank(slog.New(slog.NewTextHandler(&buf, nil)), config.Billing{Bank: billing.BankDetails{AccountName: "a", AccountNumber: "b", BankName: "c"}})
	if buf.Len() != 0 {
		t.Fatalf("logged: %s", buf.String())
	}
}
