package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// Open Company Format (durable E2): `multica company export --history` writes a
// package, and `multica company import` reads one. The server decides; these
// functions only move the bytes. Each is called first thing from its command, and
// returns handled=false for the legacy JSON, which the command then runs itself.

func init() {
	companyExportCmd.Flags().Bool("history", false, "Write an Open Company Format package (company.ocf.zip) with the settled history: outcomes, track records and the governance log. Needs --file")
}

// runCompanyExportOCF writes the package when --history is given.
func runCompanyExportOCF(cmd *cobra.Command) (bool, error) {
	history, _ := cmd.Flags().GetBool("history")
	if !history {
		return false, nil
	}
	file, _ := cmd.Flags().GetString("file")
	if file == "" {
		return true, fmt.Errorf("--history writes a zip package: pass --file, for example --file company.ocf.zip")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return true, err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	pkg, err := client.GetBytes(ctx, "/api/company/export?format=ocf&history=true")
	if err != nil {
		return true, fmt.Errorf("export company: %w", err)
	}
	if err := os.WriteFile(file, pkg, 0o600); err != nil {
		return true, fmt.Errorf("write export: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Exported the company with its history to %s\n", file)
	return true, nil
}

// runCompanyImportOCF imports data when it is a zip with a manifest.json, the
// Open Company Format package. Anything else is the legacy JSON, and handled is false.
func runCompanyImportOCF(cmd *cobra.Command, data []byte) (bool, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false, nil
	}
	if !zipHasEntry(zr, "manifest.json") {
		return true, fmt.Errorf("the zip is not an Open Company Format package: it has no manifest.json")
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	runtimeID, _ := cmd.Flags().GetString("runtime")
	query := url.Values{}
	if dryRun {
		query.Set("dry_run", "true")
	}
	if runtimeID != "" {
		query.Set("runtime_id", runtimeID)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return true, err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var report map[string]any
	if err := client.PostBytes(ctx, "/api/company/import?"+query.Encode(), "application/zip", data, &report); err != nil {
		return true, fmt.Errorf("import company: %w", err)
	}
	if output, _ := cmd.Flags().GetString("output"); output == "json" {
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return true, fmt.Errorf("format report: %w", err)
		}
		_, err = fmt.Fprintln(os.Stdout, string(raw))
		return true, err
	}
	return true, printImportReport(report)
}

func zipHasEntry(zr *zip.Reader, name string) bool {
	for _, f := range zr.File {
		if f.Name == name {
			return true
		}
	}
	return false
}
