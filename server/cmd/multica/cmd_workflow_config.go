package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// The workspace's choice for issues that are not requests of a flow squad
// (squad-room-workflow spec, section 4). The server checks the admin role on
// write; the CLI reports its error as is.

var workflowConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or set the workflow for issues that are not requests of a flow squad",
	Long: `Show or set what an issue follows when it is not a request of a squad whose
workflow is a flow, and the squad's own workflow does not cover it.

  no-action  refuse such writes until a choice is stored (the default)
  default    follow the workspace Default workflow
  workflow   follow the named workflow

Only workspace admins can change it.`,
}

var workflowConfigGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show the workspace's choice",
	Args:  cobra.NoArgs,
	RunE:  runWorkflowConfigGet,
}

var workflowConfigSetCmd = &cobra.Command{
	Use:   "set <no-action|default|workflow> [workflow]",
	Short: "Store the workspace's choice",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runWorkflowConfigSet,
}

func init() {
	workflowCmd.AddCommand(workflowConfigCmd)
	workflowConfigCmd.AddCommand(workflowConfigGetCmd)
	workflowConfigCmd.AddCommand(workflowConfigSetCmd)
	workflowConfigGetCmd.Flags().String("output", "table", "Output format: table or json")
}

// roomWorkflowPolicyBody is the choice as the API carries it.
type roomWorkflowPolicyBody struct {
	Kind       string `json:"kind"`
	WorkflowID string `json:"workflow_id,omitempty"`
}

type roomWorkflowPolicyEnvelope struct {
	NonRequestWorkflow roomWorkflowPolicyBody `json:"non_request_workflow"`
}

func roomWorkflowPath(client *cli.APIClient) string {
	return "/api/workspaces/" + client.WorkspaceID + "/room-workflow"
}

func runWorkflowConfigGet(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var out roomWorkflowPolicyEnvelope
	if err := client.GetJSON(ctx, roomWorkflowPath(client), &out); err != nil {
		return fmt.Errorf("get workflow config: %w", err)
	}
	if output, _ := cmd.Flags().GetString("output"); output == "json" {
		return cli.PrintJSON(os.Stdout, out)
	}
	fmt.Fprintf(os.Stdout, "non-request issues: %s", out.NonRequestWorkflow.Kind)
	if out.NonRequestWorkflow.Kind == "workflow" {
		fmt.Fprintf(os.Stdout, " (%s)", out.NonRequestWorkflow.WorkflowID)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}

func runWorkflowConfigSet(cmd *cobra.Command, args []string) error {
	kind := strings.ReplaceAll(strings.TrimSpace(args[0]), "-", "_")
	body := roomWorkflowPolicyBody{Kind: kind}
	switch kind {
	case "no_action", "default":
		if len(args) == 2 {
			return fmt.Errorf("%s takes no workflow", kind)
		}
	case "workflow":
		if len(args) != 2 {
			return fmt.Errorf("workflow needs the workflow to follow: multica workflow config set workflow <workflow>")
		}
	default:
		return fmt.Errorf("unknown choice %q: use no-action, default, or workflow", args[0])
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	if kind == "workflow" {
		wf, err := resolveWorkflow(ctx, client, args[1])
		if err != nil {
			return fmt.Errorf("resolve workflow: %w", err)
		}
		body.WorkflowID = strVal(wf, "id")
	}
	var out roomWorkflowPolicyEnvelope
	if err := client.PutJSON(ctx, roomWorkflowPath(client), map[string]any{"non_request_workflow": body}, &out); err != nil {
		return fmt.Errorf("set workflow config: %w", err)
	}
	fmt.Fprintf(os.Stdout, "non-request issues: %s\n", out.NonRequestWorkflow.Kind)
	return nil
}

// roomWorkflowRefusal turns the room rules' refusal (HTTP 422 with a code) into
// the step the user needs next. Any other error passes through unchanged.
func roomWorkflowRefusal(err error) error {
	var httpErr *cli.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnprocessableEntity {
		return err
	}
	var body struct {
		Code string `json:"code"`
	}
	if json.Unmarshal([]byte(httpErr.Body), &body) != nil {
		return err
	}
	switch body.Code {
	case "room_workflow_required":
		return cli.WithUserMessageUnlessNetwork("the squad has no workflow yet: choose a workflow for it in the app's Workflow tab, then run this command again", err)
	case "room_no_workflow_for_non_request":
		return cli.WithUserMessageUnlessNetwork("this issue is not a request of its squad, and the workspace has not chosen a workflow for such issues: run `multica workflow config set default` (or `multica workflow config set workflow <workflow>`), then run this command again", err)
	}
	return err
}
