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
		"not start by itself. An issue with a parent or with sub-issues cannot start one.",
	Example: "  multica issue flow start MUL-12",
	Args:    exactArgs(1),
	RunE:    runIssueFlowStart,
}

func init() {
	issueCmd.AddCommand(issueFlowCmd)
	for _, c := range []*cobra.Command{
		issueFlowShowCmd, issueFlowPlanCmd, issueFlowAdvanceCmd, issueFlowSwitchCmd,
		issueFlowResumeCmd, issueFlowDetachCmd, issueFlowStartCmd,
	} {
		issueFlowCmd.AddCommand(c)
		c.Flags().String("output", "json", "Output format: json")
	}
	issueFlowPlanCmd.Flags().StringArray("switch", nil, "A switch to set, as key=value; repeat for several")
	issueFlowPlanCmd.Flags().Bool("blocked", false, "Report that the request lacks grounds (stop 1) instead of planning it")
	issueFlowPlanCmd.Flags().StringArray("missing", nil, "With --blocked: something the request lacks; repeat for several")
	issueFlowPlanCmd.Flags().String("reason", "", "With --blocked: why the request is blocked")
	issueFlowResumeCmd.Flags().String("to-step", "", "Continue at this step instead of the one the request stopped at")
	issueFlowResumeCmd.Flags().Bool("reset-rework", false, "Start the rework count over")
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
	body := map[string]any{"reset_rework": reset}
	if toStep != "" {
		body["to_step"] = toStep
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
	var result map[string]any
	if err := client.PostJSON(ctx, path+"/start", map[string]any{}, &result); err != nil {
		return fmt.Errorf("start flow: %w", err)
	}
	return printFlowResult(cmd, result)
}
