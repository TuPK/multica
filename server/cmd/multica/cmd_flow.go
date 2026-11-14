package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// `multica flow`: what a person or an agent needs to write a workflow file (spec 7.1 and 16.4).
// Both commands only read: the check writes nothing, and drafts and versions are made in the app.

// flowManifestMaxBytes is a little over the largest manifest the server accepts, so a wrong file
// fails here without being read whole.
const flowManifestMaxBytes = 1 << 20

var flowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Check a workflow file and read what the server accepts",
	Long: "Commands for writing a workflow as a file. A workflow is a manifest, a JSON file of format\n" +
		"multica.flow-manifest/v1 that holds a flow document of schema 2. These commands check one\n" +
		"and list the limits of the server; they do not create drafts or publish. For a request that a\n" +
		"workflow runs, see `multica issue flow`.",
}

var flowValidateCmd = &cobra.Command{
	Use:   "validate --file <path> [--squad <id>]",
	Short: "Check a workflow manifest",
	Long: "Send a manifest (multica.flow-manifest/v1) to the server and print every issue it finds, one\n" +
		"per line, ordered by path: severity, path, code and message. Nothing is saved.\n\n" +
		"With --squad the roles are checked against the members of that squad (who leads, who can\n" +
		"take a task, which skills they have); without it only what does not depend on a squad is\n" +
		"checked. A manifest has no people in it, so a role with no one bound is not reported.\n\n" +
		"The exit code is 0 when there is no error (warnings do not count) and not 0 when there is\n" +
		"one or the file is not a manifest, so a script or an agent can loop until it passes.",
	Example: "  multica flow validate --file flow.json\n  multica flow validate --file flow.json --squad 0b7d5a0e-3f3c-4a6e-9d58-5b0c1f1b7a11",
	Args:    cobra.NoArgs,
	RunE:    runFlowValidate,
}

var flowCapabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Print what this server accepts in a workflow",
	Long: "Print the answer of the server on what a flow document may hold: the schema versions, the\n" +
		"limits (steps, tasks, edges, sizes), the kinds and operators, the facts a condition can\n" +
		"read, and the features turned on. Read it instead of remembering numbers.",
	Example: "  multica flow capabilities",
	Args:    cobra.NoArgs,
	RunE:    runFlowCapabilities,
}

func init() {
	flowCmd.AddCommand(flowValidateCmd)
	flowCmd.AddCommand(flowCapabilitiesCmd)
	flowValidateCmd.Flags().String("file", "", "Path of the manifest to check (required)")
	flowValidateCmd.Flags().String("squad", "", "Check the roles against this squad (id)")
	flowValidateCmd.Flags().String("output", "table", "Output format: table or json")
	flowCapabilitiesCmd.Flags().String("output", "json", "Output format: json")
}

// flowValidationAnswer is the part of the answer of the server that the command reads.
type flowValidationAnswer struct {
	OK     bool `json:"ok"`
	Issues []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Path     string `json:"path"`
		Message  string `json:"message"`
	} `json:"issues"`
}

func runFlowValidate(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	squad, _ := cmd.Flags().GetString("squad")
	output, _ := cmd.Flags().GetString("output")
	if strings.TrimSpace(file) == "" {
		return errors.New("--file is required: the path of the manifest to check")
	}
	if output != "table" && output != "json" {
		return fmt.Errorf("--output must be table or json, not %q", output)
	}
	raw, err := readFlowManifestFile(file)
	if err != nil {
		return err
	}
	body := map[string]any{"manifest": json.RawMessage(raw)}
	if squad = strings.TrimSpace(squad); squad != "" {
		body["squad_id"] = squad
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var answer map[string]any
	if err := client.PostJSON(ctx, "/api/flow/validate", body, &answer); err != nil {
		// A file that is not a manifest is answered with 400 and the issues that say why.
		var httpErr *cli.HTTPError
		if !errors.As(err, &httpErr) || json.Unmarshal([]byte(httpErr.Body), &answer) != nil || answer["issues"] == nil {
			return fmt.Errorf("check the manifest: %w", err)
		}
		if answer["ok"] == nil {
			answer["ok"] = false
		}
	}
	if output == "json" {
		if err := cli.PrintJSON(os.Stdout, answer); err != nil {
			return err
		}
		return flowValidationError(answer)
	}
	if err := printFlowIssues(os.Stdout, answer); err != nil {
		return err
	}
	return flowValidationError(answer)
}

func readFlowManifestFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, flowManifestMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(raw) > flowManifestMaxBytes {
		return nil, fmt.Errorf("%s is larger than a manifest may be (%d bytes)", path, flowManifestMaxBytes)
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%s is not valid JSON", path)
	}
	return raw, nil
}

func decodeFlowAnswer(answer map[string]any) flowValidationAnswer {
	var out flowValidationAnswer
	raw, _ := json.Marshal(answer)
	_ = json.Unmarshal(raw, &out)
	return out
}

// flowValidationError is nil when the answer has no error and a failure naming the count when it has.
func flowValidationError(answer map[string]any) error {
	parsed := decodeFlowAnswer(answer)
	errs := 0
	for _, issue := range parsed.Issues {
		if issue.Severity == "error" {
			errs++
		}
	}
	switch {
	case errs == 1:
		return errors.New("the manifest has 1 error")
	case errs > 1:
		return fmt.Errorf("the manifest has %d errors", errs)
	case !parsed.OK:
		return errors.New("the server did not accept the manifest")
	}
	return nil
}

// printFlowIssues lists the issues of an answer ordered by path; an issue about the whole
// document has no path and comes first.
func printFlowIssues(w io.Writer, answer map[string]any) error {
	parsed := decodeFlowAnswer(answer)
	issues := parsed.Issues
	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Path < issues[j].Path })
	errs, warnings := 0, 0
	for _, issue := range issues {
		switch issue.Severity {
		case "error":
			errs++
		case "warning":
			warnings++
		}
		path := issue.Path
		if path == "" {
			path = "(document)"
		}
		if _, err := fmt.Fprintf(w, "%-7s %s  %s: %s\n", issue.Severity, path, issue.Code, issue.Message); err != nil {
			return err
		}
	}
	summary := "no errors"
	if errs > 0 {
		summary = fmt.Sprintf("%d %s", errs, plural(errs, "error", "errors"))
	}
	if warnings > 0 {
		summary += fmt.Sprintf(", %d %s", warnings, plural(warnings, "warning", "warnings"))
	}
	_, err := fmt.Fprintln(w, summary)
	return err
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func runFlowCapabilities(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.GetJSON(ctx, "/api/flow/capabilities", &result); err != nil {
		return fmt.Errorf("read capabilities: %w", err)
	}
	return cli.PrintJSON(os.Stdout, result)
}
