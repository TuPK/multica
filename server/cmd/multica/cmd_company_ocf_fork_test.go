package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCompanyExportTestCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "export"}
	companyExportCmd.Flags().VisitAll(func(f *pflag.Flag) {
		switch f.Value.Type() {
		case "bool":
			cmd.Flags().Bool(f.Name, f.DefValue == "true", f.Usage)
		case "string":
			cmd.Flags().String(f.Name, f.DefValue, f.Usage)
		default:
			t.Fatalf("unsupported flag type %q for --%s", f.Value.Type(), f.Name)
		}
	})
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("parse flags %v: %v", args, err)
	}
	return cmd
}

func TestCompanyExportSendsIncludePrincipalOnlyWithHistory(t *testing.T) {
	fakeZip := []byte("PK\x03\x04fake-ocf-package")
	var gotRequests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequests = append(gotRequests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeZip)
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)

	historyOnlyFile := filepath.Join(t.TempDir(), "history-only.ocf.zip")
	out, err := captureStdout(t, func() error {
		return runCompanyExport(newCompanyExportTestCmd(t, "--history", "--file", historyOnlyFile), nil)
	})
	if err != nil {
		t.Fatalf("export --history: %v", err)
	}
	if !strings.Contains(out, historyOnlyFile) {
		t.Errorf("stdout %q should name output file %s", out, historyOnlyFile)
	}
	written, err := os.ReadFile(historyOnlyFile)
	if err != nil {
		t.Fatalf("read written package: %v", err)
	}
	if !bytes.Equal(written, fakeZip) {
		t.Errorf("written bytes = %q, want %q", written, fakeZip)
	}

	withPrincipalFile := filepath.Join(t.TempDir(), "history-principal.ocf.zip")
	out, err = captureStdout(t, func() error {
		return runCompanyExport(newCompanyExportTestCmd(t, "--history", "--include-principal", "--file", withPrincipalFile), nil)
	})
	if err != nil {
		t.Fatalf("export --history --include-principal: %v", err)
	}
	if !strings.Contains(out, withPrincipalFile) {
		t.Errorf("stdout %q should name output file %s", out, withPrincipalFile)
	}
	written, err = os.ReadFile(withPrincipalFile)
	if err != nil {
		t.Fatalf("read written package with principal: %v", err)
	}
	if !bytes.Equal(written, fakeZip) {
		t.Errorf("written bytes = %q, want %q", written, fakeZip)
	}

	wantRequests := []string{
		"GET /api/company/export?format=ocf&history=true",
		"GET /api/company/export?format=ocf&history=true&include_principal=true",
	}
	if len(gotRequests) != len(wantRequests) {
		t.Fatalf("requests = %v, want %v", gotRequests, wantRequests)
	}
	for i := range wantRequests {
		if gotRequests[i] != wantRequests[i] {
			t.Errorf("request[%d] = %q, want %q", i, gotRequests[i], wantRequests[i])
		}
	}
}

func TestCompanyExportIncludePrincipalNeedsHistory(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	setCLITestServerEnv(t, srv.URL)

	cases := [][]string{
		{"--include-principal"},
		{"--include-principal", "--file", filepath.Join(t.TempDir(), "out.ocf.zip")},
	}
	for _, args := range cases {
		err := runCompanyExport(newCompanyExportTestCmd(t, args...), nil)
		if err == nil {
			t.Fatalf("args %v: expected error, got nil", args)
		}
		if !strings.Contains(err.Error(), "--include-principal") || !strings.Contains(err.Error(), "--history") {
			t.Errorf("args %v: err = %q, want it to mention both --include-principal and --history", args, err.Error())
		}
	}
	if calls != 0 {
		t.Fatalf("server was called %d times, want 0", calls)
	}
}
