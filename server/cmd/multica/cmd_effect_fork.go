package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// Exit codes of `multica effect declare` (durable spec 4.5): the agent reads the
// decision from the code and from the JSON.
const (
	effectExitAllow = 0
	effectExitHold  = 3 // confirm or rehearse: wait for the person
	effectExitBlock = 4 // block: do not do it
)

// effectExitCode maps a policy decision to the exit code of `effect declare`.
func effectExitCode(decision string) int {
	switch decision {
	case "allow":
		return effectExitAllow
	case "confirm", "rehearse":
		return effectExitHold
	default:
		return effectExitBlock
	}
}

var effectCmd = &cobra.Command{
	Use:   "effect",
	Short: "Declare, list and undo effects on systems outside the platform",
	Long: `An effect is an action with a result outside the platform: a send, a share, a
delete, a spend, or a write to an outside system. Declare one before you take it;
the platform answers with the policy's decision.

  multica effect declare --class send --reversibility irreversible \
      --target "smtp:customer@example.com" --summary "Send the quote to the customer"
  multica effect list --class send --json
  multica effect undo <effect-id>

declare exits 0 when the action may run, 3 when it waits for the person (confirm or
rehearse), and 4 when the policy blocks it. The declaration is an agreement, not a
barrier: the platform cannot stop an action it does not run.`,
}

var effectDeclareCmd = &cobra.Command{
	Use:   "declare",
	Short: "Declare an action before you take it and get the policy's decision",
	Args:  cobra.NoArgs,
	RunE:  runEffectDeclare,
}

var effectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the workspace's effects, newest first",
	Args:  cobra.NoArgs,
	RunE:  runEffectList,
}

var effectUndoCmd = &cobra.Command{
	Use:   "undo <effect-id>",
	Short: "Take back an executed effect inside its undo window",
	Args:  cobra.ExactArgs(1),
	RunE:  runEffectUndo,
}

func runEffectDeclare(cmd *cobra.Command, _ []string) error {
	class, _ := cmd.Flags().GetString("class")
	rev, _ := cmd.Flags().GetString("reversibility")
	target, _ := cmd.Flags().GetString("target")
	summary, _ := cmd.Flags().GetString("summary")
	rationale, _ := cmd.Flags().GetString("rationale")
	if strings.TrimSpace(class) == "" {
		return fmt.Errorf("--class is required: read, write_internal, write, send, share, delete or spend")
	}
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("--target is required: the system and the object, e.g. smtp:customer@example.com")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	body := map[string]any{
		"class": class, "reversibility": rev, "target": target, "summary": summary, "rationale": rationale,
	}
	var out map[string]any
	if err := client.PostJSON(ctx, "/api/effects/declare", body, &out); err != nil {
		return fmt.Errorf("declare effect: %w", err)
	}
	decision, _ := out["decision"].(string)
	if err := cli.PrintJSON(os.Stdout, out); err != nil {
		return err
	}
	if code := effectExitCode(decision); code != effectExitAllow {
		os.Exit(code)
	}
	return nil
}

func runEffectList(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	query := url.Values{}
	for flag, param := range map[string]string{"class": "effect_class", "source": "source", "decision": "decision"} {
		if v, _ := cmd.Flags().GetString(flag); v != "" {
			query.Set(param, v)
		}
	}
	if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	var out struct {
		Effects []map[string]any `json:"effects"`
	}
	path := "/api/effects"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	if err := client.GetJSON(ctx, path, &out); err != nil {
		return fmt.Errorf("list effects: %w", err)
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return cli.PrintJSON(os.Stdout, out.Effects)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCLASS\tDECISION\tSTATUS\tSOURCE\tTARGET\tSUMMARY")
	for _, e := range out.Effects {
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\t%v\n", e["id"], e["effect_class"], e["decision"], e["status"], e["source"], e["target"], e["summary"])
	}
	return w.Flush()
}

func runEffectUndo(cmd *cobra.Command, args []string) error {
	id := strings.TrimSpace(args[0])
	if !uuidPattern.MatchString(id) {
		return fmt.Errorf("%q is not an effect id; run `multica effect list`", id)
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var out map[string]any
	if err := client.PostJSON(ctx, "/api/effects/"+id+"/undo", map[string]any{}, &out); err != nil {
		return fmt.Errorf("undo effect: %w", err)
	}
	return cli.PrintJSON(os.Stdout, out)
}

func init() {
	effectDeclareCmd.Flags().String("class", "", "effect class: read, write_internal, write, send, share, delete or spend")
	effectDeclareCmd.Flags().String("reversibility", "", "reversible, compensable or irreversible (default from the class)")
	effectDeclareCmd.Flags().String("target", "", "the system and the object the action touches")
	effectDeclareCmd.Flags().String("summary", "", "one line for the person: what the action does")
	effectDeclareCmd.Flags().String("rationale", "", "why the action is needed (kept with the declaration)")
	effectListCmd.Flags().String("class", "", "filter by effect class")
	effectListCmd.Flags().String("source", "", "filter by source: connected_app, plugin_action, cli, channel, webhook, publish_channel, declared")
	effectListCmd.Flags().String("decision", "", "filter by decision: allow, confirm, rehearse, block")
	effectListCmd.Flags().Int("limit", 50, "number of effects to show (1 to 200)")
	effectListCmd.Flags().Bool("json", false, "print JSON")
	effectCmd.AddCommand(effectDeclareCmd, effectListCmd, effectUndoCmd)
}
