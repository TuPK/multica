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

var principalCmd = &cobra.Command{
	Use:   "principal",
	Short: "A person's principal profile: what their decisions say about how they decide",
	Long: `A principal profile is kept only with the person's consent, and the person can delete it
at any time. The Secretary writes learned traits from the person's decisions, and each trait must
cite one of them.

  multica principal show --member <user-id>
  multica principal trait add --member <user-id> --kind proposal:budget --text "Approves spend under 200 when the goal is active" --evidence decision:<proposal-id>
  multica principal trait retire <trait-id> --member <user-id>

Without --member a person reads their own profile.`,
}

var principalShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show a principal profile and its active traits",
	Args:  cobra.NoArgs,
	RunE:  runPrincipalShow,
}

var principalTraitCmd = &cobra.Command{
	Use:   "trait",
	Short: "Add and retire the traits of a principal profile",
}

var principalTraitAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a learned trait that cites the person's own decisions, comments or messages",
	Args:  cobra.NoArgs,
	RunE:  runPrincipalTraitAdd,
}

var principalTraitRetireCmd = &cobra.Command{
	Use:   "retire <trait-id>",
	Short: "Retire a trait; it stays in the profile's history",
	Args:  cobra.ExactArgs(1),
	RunE:  runPrincipalTraitRetire,
}

// parsePrincipalEvidence reads --evidence values of the form type:ref, where type is decision,
// comment or interview.
func parsePrincipalEvidence(values []string) ([]map[string]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("--evidence is required: decision:<id>, comment:<id> or interview:<message-id>")
	}
	out := make([]map[string]string, 0, len(values))
	for _, v := range values {
		typ, ref, ok := strings.Cut(strings.TrimSpace(v), ":")
		typ, ref = strings.TrimSpace(typ), strings.TrimSpace(ref)
		if !ok || typ == "" || ref == "" {
			return nil, fmt.Errorf("--evidence %q must be type:ref, for example decision:<id>", v)
		}
		out = append(out, map[string]string{"type": typ, "ref": ref})
	}
	return out, nil
}

// principalMemberQuery is the member_id query of a request, empty when no --member was given.
func principalMemberQuery(cmd *cobra.Command) (url.Values, error) {
	query := url.Values{}
	member, _ := cmd.Flags().GetString("member")
	if member = strings.TrimSpace(member); member != "" {
		if !uuidPattern.MatchString(member) {
			return nil, fmt.Errorf("--member %q is not a user id", member)
		}
		query.Set("member_id", member)
	}
	return query, nil
}

func principalPath(base string, query url.Values) string {
	if len(query) == 0 {
		return base
	}
	return base + "?" + query.Encode()
}

func runPrincipalShow(cmd *cobra.Command, _ []string) error {
	query, err := principalMemberQuery(cmd)
	if err != nil {
		return err
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var out struct {
		MemberID        string           `json:"member_id"`
		Status          string           `json:"status"`
		ShareWithOwners bool             `json:"share_with_owners"`
		Version         int              `json:"version"`
		Traits          []map[string]any `json:"traits"`
	}
	if err := client.GetJSON(ctx, principalPath("/api/principal/profile", query), &out); err != nil {
		return fmt.Errorf("read principal profile: %w", err)
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return cli.PrintJSON(os.Stdout, out)
	}
	fmt.Fprintf(os.Stdout, "Profile %s: %s, version %d, shared with owners: %v\n", out.MemberID, out.Status, out.Version, out.ShareWithOwners)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tKIND\tORIGIN\tTEXT")
	for _, t := range out.Traits {
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", t["id"], t["decision_kind"], t["origin"], t["statement"])
	}
	return w.Flush()
}

func runPrincipalTraitAdd(cmd *cobra.Command, _ []string) error {
	member, _ := cmd.Flags().GetString("member")
	kind, _ := cmd.Flags().GetString("kind")
	text, _ := cmd.Flags().GetString("text")
	origin, _ := cmd.Flags().GetString("origin")
	evidenceFlags, _ := cmd.Flags().GetStringArray("evidence")
	if strings.TrimSpace(kind) == "" {
		return fmt.Errorf("--kind is required: general, outcome, inquiry, proposal:<kind> or effect:<class>")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("--text is required")
	}
	evidence, err := parsePrincipalEvidence(evidenceFlags)
	if err != nil {
		return err
	}
	body := map[string]any{
		"decision_kind": kind, "statement": text, "origin": origin, "evidence": evidence,
	}
	if member = strings.TrimSpace(member); member != "" {
		if !uuidPattern.MatchString(member) {
			return fmt.Errorf("--member %q is not a user id", member)
		}
		body["member_id"] = member
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var out map[string]any
	if err := client.PostJSON(ctx, "/api/principal/profile/traits", body, &out); err != nil {
		return fmt.Errorf("add principal trait: %w", err)
	}
	return cli.PrintJSON(os.Stdout, out)
}

func runPrincipalTraitRetire(cmd *cobra.Command, args []string) error {
	trait := strings.TrimSpace(args[0])
	if !uuidPattern.MatchString(trait) {
		return fmt.Errorf("%q is not a trait id", trait)
	}
	query, err := principalMemberQuery(cmd)
	if err != nil {
		return err
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	if err := client.DeleteJSON(ctx, principalPath("/api/principal/profile/traits/"+trait, query)); err != nil {
		return fmt.Errorf("retire principal trait: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Retired trait %s\n", trait)
	return nil
}

func init() {
	principalShowCmd.Flags().String("member", "", "the person's user id; defaults to you")
	principalShowCmd.Flags().Bool("json", false, "print JSON")
	principalTraitAddCmd.Flags().String("member", "", "the person's user id (required for the Secretary)")
	principalTraitAddCmd.Flags().String("kind", "", "general, outcome, inquiry, proposal:<kind> or effect:<class>")
	principalTraitAddCmd.Flags().String("text", "", "the trait in one sentence (500 characters at most)")
	principalTraitAddCmd.Flags().String("origin", "learned", "learned (the Secretary) or interview")
	principalTraitAddCmd.Flags().StringArray("evidence", nil, "what the trait rests on, as type:ref (decision, comment or interview)")
	principalTraitRetireCmd.Flags().String("member", "", "the person's user id (required for the Secretary)")
	principalTraitCmd.AddCommand(principalTraitAddCmd, principalTraitRetireCmd)
	principalCmd.AddCommand(principalShowCmd, principalTraitCmd)
	rootCmd.AddCommand(principalCmd)
}
