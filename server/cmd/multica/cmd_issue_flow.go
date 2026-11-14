package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// `multica issue flow`: the commands of a request that a workflow runs
// (docs/engineering/kitviet-workflow-spec.md, section 7.2). The server decides
// the steps, the tasks and the verdicts; these commands report the lead's plan
// and ask the server to advance.

var issueFlowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Work with a request that a workflow runs",
	Long: "A request given to a squad whose workflow runs on a flow is moved by the server:\n" +
		"it opens a sub-issue for each task of a step, waits for all of them, and routes the\n" +
		"request on the verdicts. Do not set the status of such a request and do not create\n" +
		"sub-issues under it; use these commands.\n\n" +
		"Start with `multica issue flow show <id>`. If it says the issue is not run by a\n" +
		"flow, follow the ordinary procedure instead.",
}

var issueFlowShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show the flow state of a request",
	Long: "Print where the request is: its step, the rework rounds used, the switches, the tasks\n" +
		"with their status and outcome, and the ways out of the step.\n\n" +
		"When the issue is not run by a flow the command prints {\"managed\": false} (the server's\n" +
		"flow_not_managed) and succeeds, so a lead can branch on it.",
	Example: "  multica issue flow show MUL-12",
	Args:    exactArgs(1),
	RunE:    runIssueFlowShow,
}

var issueFlowPlanCmd = &cobra.Command{
	Use:   "plan <id>",
	Short: "Give the plan of a request and start it",
	Long: "Leave the plan step: choose the switches of the flow and let the server open the first\n" +
		"tasks. Only a request at the plan step can be planned.\n\n" +
		"When the request lacks grounds (a document the work needs), report it instead:\n" +
		"  multica issue flow plan <id> --blocked --missing \"invoice\" --reason \"...\"\n" +
		"The server stops the request and tells the requester.",
	Example: "  multica issue flow plan MUL-12 --switch customs_lookup=true",
	Args:    exactArgs(1),
	RunE:    runIssueFlowPlan,
}

var issueFlowAdvanceCmd = &cobra.Command{
	Use:   "advance <id>",
	Short: "Move a request on",
	Long: "Ask the server to open the missing tasks of the step, or to route the request when every\n" +
		"task is closed. It is idempotent: run it again and nothing happens when there is\n" +
		"nothing to do. It answers with what it did and what the request still waits for.\n" +
		"Run it after a task closes; do not set the status of the request and do not create\n" +
		"sub-issues under it yourself.",
	Example: "  multica issue flow advance MUL-12",
	Args:    exactArgs(1),
	RunE:    runIssueFlowAdvance,
}

var issueFlowSwitchCmd = &cobra.Command{
	Use:   "switch <id> <key> <value>",
	Short: "Set a switch of a request",
	Long: "Set one switch of the flow. A bool switch takes true or false; an enum switch takes one\n" +
		"of its values. The lead sets switches when it plans; a member who manages the squad\n" +
		"may set one at any step.",
	Example: "  multica issue flow switch MUL-12 customs_lookup true\n  multica issue flow switch <id> <key> <value>",
	Args:    exactArgs(3),
	RunE:    runIssueFlowSwitch,
}

var issueFlowResumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Continue a request that stopped (squad managers)",
	Long: "Continue a request the server stopped. After a stop for missing grounds the tasks that\n" +
		"blocked open again; after the rework rounds ran out the count starts over and the step's\n" +
		"tasks open again. Only a member who manages the squad can resume.",
	Example: "  multica issue flow resume MUL-12\n  multica issue flow resume MUL-12 --to-step draft --reset-rework",
	Args:    exactArgs(1),
	RunE:    runIssueFlowResume,
}

var issueFlowDetachCmd = &cobra.Command{
	Use:   "detach <id>",
	Short: "Take a request off its flow (squad managers)",
	Long: "Stop the workflow from running the request. Its tasks become ordinary sub-issues, or\n" +
		"are cancelled with --cancel-tasks. Only a member who manages the squad can detach.",
	Example: "  multica issue flow detach MUL-12 --cancel-tasks",
	Args:    exactArgs(1),
	RunE:    runIssueFlowDetach,
}

var issueFlowStartCmd = &cobra.Command{
	Use:   "start <id>",
	Short: "Put a request on its squad's flow",
	Long: "Start the flow for an issue given to a squad whose workflow runs on a flow, when it did\n" +
		"not start by itself. An issue with a parent or with sub-issues cannot start one.\n" +
		"It runs on the active workflow of the squad. With --workflow it runs on another version of\n" +
		"the squad's workflows; with --trial the run is a rehearsal, counted apart in the insights.\n" +
		"Without --workflow, a trial rehearses the active workflow. Both are for the members who\n" +
		"manage the squad.",
	Example: "  multica issue flow start MUL-12\n" +
		"  multica issue flow start MUL-12 --workflow 0e9bbf5a-3bd4-4c34-8d1c-6f6b1f1a9d10 --trial",
	Args: exactArgs(1),
	RunE: runIssueFlowStart,
}

var issueFlowCompleteCmd = &cobra.Command{
	Use:   "complete <id> <step>",
	Short: "Complete a lead step of a request",
	Long: "Report the verdict of a lead step and set its variables in one call: the request goes on\n" +
		"or, with --blocked, stops for missing grounds. `plan` is this command on the plan step.\n" +
		"Only the lead of the squad and the members who manage it can complete a step.",
	Example: "  multica issue flow complete MUL-12 plan --verdict planned --set depth=full\n" +
		"  multica issue flow complete MUL-12 plan --blocked --missing \"invoice\" --reason \"...\"",
	Args: exactArgs(2),
	RunE: runIssueFlowComplete,
}

var issueFlowVarsCmd = &cobra.Command{
	Use:   "vars <id> [<key> <value>]",
	Short: "Show or set the variables of a request",
	Long: "Without a key, print the variables of the request. With a key and a value, set one: the\n" +
		"role the variable names sets it at the steps it names, and a member who manages the\n" +
		"squad may set it at any step. A list takes comma separated items.",
	Example: "  multica issue flow vars MUL-12\n  multica issue flow vars MUL-12 depth full",
	Args:    cobra.RangeArgs(1, 3),
	RunE:    runIssueFlowVars,
}

var issueFlowSetCmd = &cobra.Command{
	Use:   "set <task-id> <key> <value>",
	Short: "Set a variable from the task you work on",
	Long: "Set a variable that your task lists (its description names them) while the task is open.\n" +
		"A task that requires a variable does not close before it is set. A list takes comma\n" +
		"separated items. The value is data of the declared type, never taken from text.",
	Example: "  multica issue flow set MUL-13 topics \"pricing,regulation,competitors\"",
	Args:    exactArgs(3),
	RunE:    runIssueFlowSet,
}

var issueFlowExitsCmd = &cobra.Command{
	Use:   "exits <id> [<edge>]",
	Short: "List the ways out of a manual step, or take one",
	Long: "A request at a manual step waits for a person. Without an edge, print the ways out of the\n" +
		"step; with one, take it. The member of the role of the step, and the members who manage\n" +
		"the squad, may choose; the choice is the human checkpoint of the request.",
	Example: "  multica issue flow exits MUL-12\n  multica issue flow exits MUL-12 release_done",
	Args:    cobra.RangeArgs(1, 2),
	RunE:    runIssueFlowExits,
}

func init() {
	issueCmd.AddCommand(issueFlowCmd)
	for _, c := range []*cobra.Command{
		issueFlowShowCmd, issueFlowPlanCmd, issueFlowAdvanceCmd, issueFlowSwitchCmd,
		issueFlowResumeCmd, issueFlowDetachCmd, issueFlowStartCmd,
		issueFlowCompleteCmd, issueFlowVarsCmd, issueFlowSetCmd, issueFlowExitsCmd,
	} {
		issueFlowCmd.AddCommand(c)
		c.Flags().String("output", "json", "Output format: json")
	}
	issueFlowPlanCmd.Flags().StringArray("switch", nil, "A switch to set, as key=value; repeat for several")
	issueFlowPlanCmd.Flags().Bool("blocked", false, "Report that the request lacks grounds (stop 1) instead of planning it")
	issueFlowPlanCmd.Flags().StringArray("missing", nil, "With --blocked: something the request lacks; repeat for several")
	issueFlowPlanCmd.Flags().String("reason", "", "With --blocked: why the request is blocked")
	issueFlowResumeCmd.Flags().String("to-step", "", "Continue at this step instead of the one the request stopped at")
	issueFlowCompleteCmd.Flags().String("verdict", "", "The verdict of the step (default: the first verdict that passes)")
	issueFlowCompleteCmd.Flags().StringArray("set", nil, "A variable to set, as key=value; repeat for several")
	issueFlowCompleteCmd.Flags().Bool("blocked", false, "Report that the request lacks grounds (stop 1) instead of completing the step")
	issueFlowCompleteCmd.Flags().StringArray("missing", nil, "With --blocked: something the request lacks; repeat for several")
	issueFlowCompleteCmd.Flags().String("reason", "", "With --blocked: why the request is blocked")
	issueFlowResumeCmd.Flags().Bool("reset-rework", false, "Start the rework count over")
	issueFlowResumeCmd.Flags().StringArray("reset-loops", nil, "A loop whose count starts over, or \"all\"; repeat for several")
	issueFlowResumeCmd.Flags().Bool("skip-cancelled", false, "Let the step go on without the work tasks a person cancelled")
	issueFlowResumeCmd.Flags().Bool("acknowledge-skip-checkpoint", false, "Resume a request stalled because no person has seen the work of an agent, taking the place of that check")
	issueFlowStartCmd.Flags().String("workflow", "", "The id of the version of the squad's workflows to run on, instead of the active one")
	issueFlowStartCmd.Flags().Bool("trial", false, "Make the run a rehearsal (a trial run), counted apart from real requests")
	issueFlowDetachCmd.Flags().Bool("cancel-tasks", false, "Cancel the open tasks instead of keeping them as sub-issues")
}

// flowIssuePath resolves the issue argument and returns the flow path of it.
func flowIssuePath(cmd *cobra.Command, ref string) (*cli.APIClient, context.Context, context.CancelFunc, string, error) {
	client, err := newAPIClient(cmd)
	if err != nil {
		return nil, nil, nil, "", err
	}
	ctx, cancel := cli.APIContext(context.Background())
	issue, err := resolveIssueRef(ctx, client, ref)
	if err != nil {
		cancel()
		return nil, nil, nil, "", fmt.Errorf("resolve issue: %w", err)
	}
	return client, ctx, cancel, "/api/issues/" + issue.ID + "/flow", nil
}

func printFlowResult(cmd *cobra.Command, result map[string]any) error {
	return cli.PrintJSON(os.Stdout, result)
}

func runIssueFlowShow(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	if err := client.GetJSON(ctx, path, &result); err != nil {
		var httpErr *cli.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound && strings.Contains(httpErr.Body, "flow_not_managed") {
			return cli.PrintJSON(os.Stdout, map[string]any{"managed": false})
		}
		return fmt.Errorf("show flow: %w", err)
	}
	return printFlowResult(cmd, result)
}

// parseFlowSwitches reads key=value pairs. A value of true or false is a bool;
// anything else is a string.
func parseFlowSwitches(pairs []string) (map[string]any, error) {
	out := map[string]any{}
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("--switch %q must be key=value", pair)
		}
		out[key] = parseFlowSwitchValue(value)
	}
	return out, nil
}

func parseFlowSwitchValue(value string) any {
	switch value {
	case "true":
		return true
	case "false":
		return false
	}
	return value
}

func runIssueFlowPlan(cmd *cobra.Command, args []string) error {
	pairs, _ := cmd.Flags().GetStringArray("switch")
	switches, err := parseFlowSwitches(pairs)
	if err != nil {
		return err
	}
	blocked, _ := cmd.Flags().GetBool("blocked")
	missing, _ := cmd.Flags().GetStringArray("missing")
	reason, _ := cmd.Flags().GetString("reason")
	if !blocked && (len(missing) > 0 || reason != "") {
		return fmt.Errorf("--missing and --reason are for --blocked")
	}
	body := map[string]any{"switches": switches, "blocked": nil}
	if blocked {
		if missing == nil {
			missing = []string{}
		}
		body["blocked"] = map[string]any{"missing": missing, "reason": reason}
	}
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, path+"/plan", body, &result); err != nil {
		return fmt.Errorf("plan flow: %w", err)
	}
	return printFlowResult(cmd, result)
}

func runIssueFlowAdvance(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, path+"/advance", map[string]any{}, &result); err != nil {
		return fmt.Errorf("advance flow: %w", err)
	}
	return printFlowResult(cmd, result)
}

func runIssueFlowSwitch(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	body := map[string]any{"value": parseFlowSwitchValue(args[2])}
	if err := client.PutJSON(ctx, path+"/switches/"+url.PathEscape(args[1]), body, &result); err != nil {
		return fmt.Errorf("set switch: %w", err)
	}
	return printFlowResult(cmd, result)
}

func runIssueFlowResume(cmd *cobra.Command, args []string) error {
	toStep, _ := cmd.Flags().GetString("to-step")
	reset, _ := cmd.Flags().GetBool("reset-rework")
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	skipCancelled, _ := cmd.Flags().GetBool("skip-cancelled")
	acknowledgeSkip, _ := cmd.Flags().GetBool("acknowledge-skip-checkpoint")
	resetLoops, _ := cmd.Flags().GetStringArray("reset-loops")
	body := map[string]any{"reset_rework": reset}
	if toStep != "" {
		body["to_step"] = toStep
	}
	if skipCancelled {
		body["skip_cancelled"] = true
	}
	if acknowledgeSkip {
		body["acknowledge_skip_checkpoint"] = true
	}
	if len(resetLoops) > 0 {
		body["reset_loops"] = resetLoops
	}
	var result map[string]any
	if err := client.PostJSON(ctx, path+"/resume", body, &result); err != nil {
		return fmt.Errorf("resume flow: %w", err)
	}
	return printFlowResult(cmd, result)
}

func runIssueFlowDetach(cmd *cobra.Command, args []string) error {
	cancelTasks, _ := cmd.Flags().GetBool("cancel-tasks")
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, path+"/detach", map[string]any{"cancel_tasks": cancelTasks}, &result); err != nil {
		return fmt.Errorf("detach flow: %w", err)
	}
	return printFlowResult(cmd, result)
}

func runIssueFlowStart(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	// The body names a version or a trial only when asked to: a plain start is the same request as before.
	body := map[string]any{}
	if workflow, _ := cmd.Flags().GetString("workflow"); strings.TrimSpace(workflow) != "" {
		body["workflow_id"] = strings.TrimSpace(workflow)
	}
	if trial, _ := cmd.Flags().GetBool("trial"); trial {
		body["trial"] = true
	}
	var result map[string]any
	if err := client.PostJSON(ctx, path+"/start", body, &result); err != nil {
		return fmt.Errorf("start flow: %w", err)
	}
	return printFlowResult(cmd, result)
}

// parseFlowVarValue reads a value from the command line: true and false are bool,
// a whole number is a number, a text with commas is a list, anything else is text.
// The server checks the value against the type the variable declares, so a text the
// type does not take is refused there.
func parseFlowVarValue(value string) any {
	switch value {
	case "true":
		return true
	case "false":
		return false
	}
	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err == nil && fmt.Sprint(n) == value {
		return n
	}
	if strings.Contains(value, ",") {
		items := strings.Split(value, ",")
		for i := range items {
			items[i] = strings.TrimSpace(items[i])
		}
		return items
	}
	return value
}

func runIssueFlowComplete(cmd *cobra.Command, args []string) error {
	pairs, _ := cmd.Flags().GetStringArray("set")
	vars := map[string]any{}
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return fmt.Errorf("--set %q must be key=value", pair)
		}
		vars[key] = parseFlowVarValue(value)
	}
	verdict, _ := cmd.Flags().GetString("verdict")
	blocked, _ := cmd.Flags().GetBool("blocked")
	missing, _ := cmd.Flags().GetStringArray("missing")
	reason, _ := cmd.Flags().GetString("reason")
	if !blocked && (len(missing) > 0 || reason != "") {
		return fmt.Errorf("--missing and --reason are for --blocked")
	}
	body := map[string]any{"verdict": verdict, "vars": vars}
	if blocked {
		if missing == nil {
			missing = []string{}
		}
		body["blocked"] = map[string]any{"missing": missing, "reason": reason}
	}
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, path+"/steps/"+url.PathEscape(args[1])+"/complete", body, &result); err != nil {
		return fmt.Errorf("complete step: %w", err)
	}
	return printFlowResult(cmd, result)
}

func runIssueFlowVars(cmd *cobra.Command, args []string) error {
	if len(args) == 2 {
		return fmt.Errorf("give a key and a value, or neither")
	}
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	if len(args) == 1 {
		if err := client.GetJSON(ctx, path, &result); err != nil {
			return fmt.Errorf("show variables: %w", err)
		}
		return printFlowResult(cmd, map[string]any{"vars": result["vars"]})
	}
	if err := client.PutJSON(ctx, path+"/vars/"+url.PathEscape(args[1]), map[string]any{"value": parseFlowVarValue(args[2])}, &result); err != nil {
		return fmt.Errorf("set variable: %w", err)
	}
	return printFlowResult(cmd, result)
}

func runIssueFlowSet(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	if err := client.PutJSON(ctx, path+"/vars/"+url.PathEscape(args[1]), map[string]any{"value": parseFlowVarValue(args[2])}, &result); err != nil {
		return fmt.Errorf("set variable: %w", err)
	}
	return printFlowResult(cmd, result)
}

func runIssueFlowExits(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, path, err := flowIssuePath(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	var result map[string]any
	if len(args) == 1 {
		if err := client.GetJSON(ctx, path, &result); err != nil {
			return fmt.Errorf("show exits: %w", err)
		}
		exits := []any{}
		next, _ := result["next"].([]any)
		for _, e := range next {
			if edge, ok := e.(map[string]any); ok && edge["trigger"] == "manual" {
				exits = append(exits, edge)
			}
		}
		return printFlowResult(cmd, map[string]any{"step": result["step"], "exits": exits})
	}
	if err := client.PostJSON(ctx, path+"/exits/"+url.PathEscape(args[1]), map[string]any{}, &result); err != nil {
		return fmt.Errorf("take exit: %w", err)
	}
	return printFlowResult(cmd, result)
}
