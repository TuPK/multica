package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// multica issue criteria {list|add|archive} and multica outcome {show|verify} (durable
// spec 2.7). Every command exits non-zero when the server refuses the request: the client
// returns an error for any 4xx or 5xx status.

var issueCriteriaCmd = &cobra.Command{
	Use:   "criteria",
	Short: "List, add and archive the acceptance criteria of an issue",
	Long: `Acceptance criteria say how the work on an issue is checked. A criterion is one
sentence and a check kind:

  manual          a person decides
  agent_judgment  an agent verifier judges the work against --spec '{"rubric":"..."}'
  command         an agent verifier runs --spec '{"run":"pnpm test","expect_exit":0}'
  artifact        a document of the kind --spec '{"kind":"spec"}' is published from the issue
  metric          a company metric reaches --spec '{"metric_key":"k","scope":"company","op":">=","value":10,"within_days":7}'

An issue carries at most 20 open criteria. Once its outcome settles, criteria can no
longer be edited.`,
}

var issueCriteriaListCmd = &cobra.Command{
	Use:   "list <issue>",
	Short: "List the criteria of an issue",
	Args:  exactArgs(1),
	RunE:  runIssueCriteriaList,
}

var issueCriteriaAddCmd = &cobra.Command{
	Use:   "add <issue> <text>",
	Short: "Add a criterion to an issue",
	Args:  exactArgs(2),
	RunE:  runIssueCriteriaAdd,
}

var issueCriteriaArchiveCmd = &cobra.Command{
	Use:   "archive <issue> <criterion-id>",
	Short: "Archive a criterion, so it no longer counts",
	Args:  exactArgs(2),
	RunE:  runIssueCriteriaArchive,
}

var outcomeCmd = &cobra.Command{
	Use:   "outcome",
	Short: "Show the work outcome of an issue and record checks on it",
}

var outcomeShowCmd = &cobra.Command{
	Use:   "show <issue>",
	Short: "Show the current outcome of an issue, with its checks",
	Args:  exactArgs(1),
	RunE:  runOutcomeShow,
}

var outcomeVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Record one check on an outcome",
	Long: `Record a check on an outcome. A verifier agent records a check only through the
verification task it was given for that criterion: the platform sends the task id with the
request. Pass --criterion for a check on one criterion; leave it out for a conclusion on the
whole outcome. --result is pass, partial, fail or unable.`,
	Args: exactArgs(0),
	RunE: runOutcomeVerify,
}

// The evidence types a check may cite (spec 2.3).
var outcomeEvidenceTypes = map[string]bool{
	"run": true, "comment": true, "document": true, "metric": true, "pr": true, "attachment": true, "url": true,
}

func init() {
	issueCriteriaCmd.AddCommand(issueCriteriaListCmd)
	issueCriteriaCmd.AddCommand(issueCriteriaAddCmd)
	issueCriteriaCmd.AddCommand(issueCriteriaArchiveCmd)
	issueCriteriaListCmd.Flags().String("output", "table", "Output format: table or json")
	addIssueCriteriaAddFlags(issueCriteriaAddCmd)
	issueCriteriaArchiveCmd.Flags().String("output", "table", "Output format: table or json")
	issueCmd.AddCommand(issueCriteriaCmd)

	outcomeCmd.AddCommand(outcomeShowCmd)
	outcomeCmd.AddCommand(outcomeVerifyCmd)
	outcomeShowCmd.Flags().String("output", "table", "Output format: table or json")
	outcomeVerifyCmd.Flags().String("outcome", "", "Id of the outcome being checked — required")
	outcomeVerifyCmd.Flags().String("criterion", "", "Id of the criterion checked; empty for a conclusion on the whole outcome")
	outcomeVerifyCmd.Flags().String("result", "", "pass, partial, fail or unable — required")
	outcomeVerifyCmd.Flags().String("severity", "none", "none, low, medium or high")
	outcomeVerifyCmd.Flags().String("note", "", "Why the result is what it is (up to 4000 characters)")
	outcomeVerifyCmd.Flags().StringArray("evidence", nil, "Evidence as type:ref, repeatable; type is run, comment, document, metric, pr, attachment or url")
	outcomeVerifyCmd.Flags().String("output", "table", "Output format: table or json")
	rootCmd.AddCommand(outcomeCmd)
}

func addIssueCriteriaAddFlags(cmd *cobra.Command) {
	cmd.Flags().String("check", "agent_judgment", "manual, agent_judgment, command, artifact or metric")
	cmd.Flags().String("spec", "{}", "The check's spec as JSON, for example '{\"rubric\":\"...\"}'")
	cmd.Flags().String("output", "table", "Output format: table or json")
}

func runIssueCriteriaList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result struct {
		Criteria []map[string]any `json:"criteria"`
	}
	if err := client.GetJSON(ctx, "/api/issues/"+strings.TrimSpace(args[0])+"/criteria", &result); err != nil {
		return fmt.Errorf("list criteria: %w", err)
	}
	if output, _ := cmd.Flags().GetString("output"); output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	for _, c := range result.Criteria {
		state := "open"
		if strVal(c, "archived_at") != "" {
			state = "archived"
		}
		fmt.Printf("%s  %-14s %-8s %s\n", strVal(c, "id"), strVal(c, "check_kind"), state, strVal(c, "text"))
	}
	if len(result.Criteria) == 0 {
		fmt.Println("No criteria on this issue.")
	}
	return nil
}

func runIssueCriteriaAdd(cmd *cobra.Command, args []string) error {
	issue := strings.TrimSpace(args[0])
	text := strings.TrimSpace(args[1])
	if issue == "" {
		return fmt.Errorf("the issue is required")
	}
	if text == "" {
		return fmt.Errorf("the criterion text is empty")
	}
	kind, _ := cmd.Flags().GetString("check")
	switch kind {
	case "manual", "agent_judgment", "command", "artifact", "metric":
	default:
		return fmt.Errorf("--check must be manual, agent_judgment, command, artifact or metric")
	}
	specRaw, _ := cmd.Flags().GetString("spec")
	var spec map[string]any
	if err := json.Unmarshal([]byte(specRaw), &spec); err != nil || spec == nil {
		return fmt.Errorf("--spec must be a JSON object: %w", err)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	req := map[string]any{"text": text, "check_kind": kind, "check_spec": spec}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+issue+"/criteria", req, &result); err != nil {
		return fmt.Errorf("add criterion: %w", err)
	}
	if output, _ := cmd.Flags().GetString("output"); output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	fmt.Printf("Criterion added: %s\n", strVal(result, "id"))
	return nil
}

func runIssueCriteriaArchive(cmd *cobra.Command, args []string) error {
	issue := strings.TrimSpace(args[0])
	criterion := strings.TrimSpace(args[1])
	if issue == "" || criterion == "" {
		return fmt.Errorf("the issue and the criterion id are required")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	req := map[string]any{"archived": true}
	if err := client.PatchJSON(ctx, "/api/issues/"+issue+"/criteria/"+criterion, req, &result); err != nil {
		return fmt.Errorf("archive criterion: %w", err)
	}
	if output, _ := cmd.Flags().GetString("output"); output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	fmt.Printf("Criterion archived: %s\n", strVal(result, "id"))
	return nil
}

func runOutcomeShow(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.GetJSON(ctx, "/api/issues/"+strings.TrimSpace(args[0])+"/outcome", &result); err != nil {
		return fmt.Errorf("show outcome: %w", err)
	}
	if output, _ := cmd.Flags().GetString("output"); output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	fmt.Printf("Outcome %s\n", strVal(result, "id"))
	fmt.Printf("  verdict:  %s", strVal(result, "verdict"))
	if cause := strVal(result, "fail_cause"); cause != "" {
		fmt.Printf(" (%s)", cause)
	}
	fmt.Println()
	fmt.Printf("  settles:  %s\n", strVal(result, "settle_at"))
	return nil
}

func runOutcomeVerify(cmd *cobra.Command, _ []string) error {
	outcome, _ := cmd.Flags().GetString("outcome")
	outcome = strings.TrimSpace(outcome)
	if !uuidPattern.MatchString(outcome) {
		return fmt.Errorf("--outcome is required and must be an outcome id; run `multica outcome show <issue>`")
	}
	result, _ := cmd.Flags().GetString("result")
	switch result {
	case "pass", "partial", "fail", "unable":
	case "":
		return fmt.Errorf("--result is required: pass, partial, fail or unable")
	default:
		return fmt.Errorf("--result must be pass, partial, fail or unable, not %q", result)
	}
	severity, _ := cmd.Flags().GetString("severity")
	switch severity {
	case "none", "low", "medium", "high":
	default:
		return fmt.Errorf("--severity must be none, low, medium or high")
	}
	note, _ := cmd.Flags().GetString("note")
	body := map[string]any{"result": result, "severity": severity, "note": note}

	criterion, _ := cmd.Flags().GetString("criterion")
	criterion = strings.TrimSpace(criterion)
	if criterion != "" {
		if !uuidPattern.MatchString(criterion) {
			return fmt.Errorf("--criterion %q is not a criterion id", criterion)
		}
		body["criterion_id"] = criterion
	}
	evidence, _ := cmd.Flags().GetStringArray("evidence")
	items := make([]map[string]string, 0, len(evidence))
	for _, e := range evidence {
		typ, ref, ok := strings.Cut(e, ":")
		if !ok || !outcomeEvidenceTypes[typ] || strings.TrimSpace(ref) == "" {
			return fmt.Errorf("--evidence %q must be type:ref, with type run, comment, document, metric, pr, attachment or url", e)
		}
		items = append(items, map[string]string{"type": typ, "ref": strings.TrimSpace(ref)})
	}
	if len(items) > 0 {
		body["evidence"] = items
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var out map[string]any
	if err := client.PostJSON(ctx, "/api/outcomes/"+outcome+"/verifications", body, &out); err != nil {
		return fmt.Errorf("record check: %w", err)
	}
	if output, _ := cmd.Flags().GetString("output"); output == "json" {
		return cli.PrintJSON(os.Stdout, out)
	}
	fmt.Printf("Check recorded: %s (%s)\n", strVal(out, "id"), strVal(out, "result"))
	return nil
}
