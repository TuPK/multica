package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// ── Handoff of a request ────────────────────────────────────────────────────

var squadHandoffCmd = &cobra.Command{
	Use:   "handoff <request-id>",
	Short: "Ask for the handoff of a finished request to the squad its rule names",
	Long: `Ask the platform to hand a finished request to another squad.

A squad leader runs this at the very end of a request, after posting its
handoff packet. The platform reads the squad's handoff rule and answers:

  handed_off  the other squad received the request; it is named in the answer
  pending     the handoff waits (for the boss's approval, or for the company to run again)
  off         the squad has no handoff, or it is off; finish the request as usual
  declined    the boss declined the handoff; do not ask again

Asking twice is harmless: a request is handed off at most once. Never create the
other squad's request yourself.`,
	Args: exactArgs(1),
	RunE: runSquadHandoff,
}

func runSquadHandoff(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	issueRef, err := resolveIssueRef(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+issueRef.ID+"/handoff", map[string]any{}, &result); err != nil {
		return fmt.Errorf("hand off %s: %w", issueRef.Display, err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	status := strVal(result, "status")
	switch status {
	case "handed_off":
		handoff, _ := result["handoff"].(map[string]any)
		fmt.Printf("handed_off: %s received the request (issue %s)\n", strVal(handoff, "target_squad_name"), strVal(handoff, "target_issue_id"))
	case "pending":
		fmt.Printf("pending: %s\n", strVal(result, "reason"))
	case "off":
		fmt.Println("off: this squad has no handoff, or it is off. Finish the request as usual.")
	case "declined":
		fmt.Println("declined: the boss declined the handoff. Do not ask again.")
	default:
		fmt.Println(status)
	}
	return nil
}

// ── Handoff rule ────────────────────────────────────────────────────────────

var squadHandoffRuleCmd = &cobra.Command{
	Use:   "handoff-rule",
	Short: "Show or set where a squad hands its finished requests",
}

var squadHandoffRuleGetCmd = &cobra.Command{
	Use:   "get <squad-id>",
	Short: "Show a squad's handoff rule",
	Args:  exactArgs(1),
	RunE:  runSquadHandoffRuleGet,
}

func runSquadHandoffRuleGet(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	squadID, err := resolveSquadRef(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve squad: %w", err)
	}
	var rule map[string]any
	if err := client.GetJSON(ctx, "/api/squads/"+squadID+"/handoff-rule", &rule); err != nil {
		return fmt.Errorf("get handoff rule: %w", err)
	}
	return printHandoffRule(cmd, rule)
}

var squadHandoffRuleSetCmd = &cobra.Command{
	Use:   "set <squad-id>",
	Short: "Set where a squad hands its finished requests (owners and admins)",
	Long: `Set the squad's handoff rule: the squad it hands finished requests to, and how.

Modes:
  off   the squad stops at its finished request; the boss hands off by hand
  ask   each handoff waits for the boss's approval
  auto  the handoff runs as soon as the squad's leader asks for it

Pass no --to to remove the rule.`,
	Args: exactArgs(1),
	RunE: runSquadHandoffRuleSet,
}

func runSquadHandoffRuleSet(cmd *cobra.Command, args []string) error {
	to, _ := cmd.Flags().GetString("to")
	mode, _ := cmd.Flags().GetString("mode")
	if to != "" && mode != "off" && mode != "ask" && mode != "auto" {
		return fmt.Errorf("--mode is required with --to: off, ask or auto")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	squadID, err := resolveSquadRef(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("resolve squad: %w", err)
	}
	body := map[string]any{}
	if to, _ := cmd.Flags().GetString("to"); to != "" {
		targetID, err := resolveSquadRef(ctx, client, to)
		if err != nil {
			return fmt.Errorf("resolve --to: %w", err)
		}
		mode, _ := cmd.Flags().GetString("mode")
		if mode != "off" && mode != "ask" && mode != "auto" {
			return fmt.Errorf("--mode must be off, ask or auto")
		}
		body["target_squad_id"] = targetID
		body["mode"] = mode
	}
	var rule map[string]any
	if err := client.PutJSON(ctx, "/api/squads/"+squadID+"/handoff-rule", body, &rule); err != nil {
		return fmt.Errorf("set handoff rule: %w", err)
	}
	return printHandoffRule(cmd, rule)
}

func printHandoffRule(cmd *cobra.Command, rule map[string]any) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, rule)
	}
	target := strVal(rule, "target_squad_name")
	if target == "" {
		fmt.Println("No handoff target: the squad stops at its finished request.")
		return nil
	}
	fmt.Printf("Hands finished requests to %s, mode %s.\n", target, strVal(rule, "mode"))
	return nil
}

func init() {
	squadHandoffCmd.Flags().String("output", "table", "Output format: table or json")

	squadHandoffRuleGetCmd.Flags().String("output", "table", "Output format: table or json")
	squadHandoffRuleSetCmd.Flags().String("to", "", "The squad to hand finished requests to (name or ID); omit to remove the rule")
	squadHandoffRuleSetCmd.Flags().String("mode", "", "off, ask or auto (required with --to)")
	squadHandoffRuleSetCmd.Flags().String("output", "table", "Output format: table or json")

	squadHandoffRuleCmd.AddCommand(squadHandoffRuleGetCmd)
	squadHandoffRuleCmd.AddCommand(squadHandoffRuleSetCmd)
	squadCmd.AddCommand(squadHandoffCmd)
	squadCmd.AddCommand(squadHandoffRuleCmd)
}
