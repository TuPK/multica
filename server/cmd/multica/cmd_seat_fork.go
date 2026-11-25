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

var seatCmd = &cobra.Command{
	Use:   "seat",
	Short: "Keep what a seat learns: its dossier of lessons, pitfalls and context",
	Long: `A seat is an agent in the company. Its dossier holds what it has learned from past
work. The dossier stays with the seat when the model in the seat changes.

  multica seat lesson add --kind pitfall --text "The export drops the header row" --evidence outcome:<outcome-id>
  multica seat lesson list
  multica seat lesson retire <entry-id> --reason "Duplicates an entry from last week"

Inside an agent's task the seat is the agent itself. Otherwise name it with --agent.`,
}

var seatLessonCmd = &cobra.Command{
	Use:   "lesson",
	Short: "Add, list and retire the entries of a seat's dossier",
}

var seatLessonAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an entry to the seat's dossier",
	Args:  cobra.NoArgs,
	RunE:  runSeatLessonAdd,
}

var seatLessonListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the seat's dossier entries, newest first",
	Args:  cobra.NoArgs,
	RunE:  runSeatLessonList,
}

var seatLessonRetireCmd = &cobra.Command{
	Use:   "retire <entry-id>",
	Short: "Retire an entry that is a duplicate or no longer holds, with the reason",
	Args:  cobra.ExactArgs(1),
	RunE:  runSeatLessonRetire,
}

// seatFromFlags is the seat the command acts on: --agent, else the agent of this task.
func seatFromFlags(cmd *cobra.Command) (string, error) {
	seat, _ := cmd.Flags().GetString("agent")
	if seat == "" {
		seat = strings.TrimSpace(os.Getenv("MULTICA_AGENT_ID"))
	}
	if !uuidPattern.MatchString(seat) {
		return "", fmt.Errorf("name the seat with --agent <agent-id>, or run inside an agent task")
	}
	return seat, nil
}

// parseSeatEvidence reads --evidence values of the form type:ref.
func parseSeatEvidence(values []string) ([]map[string]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("--evidence is required: outcome:<id>, retro:<id>, comment:<id> or document:<id>")
	}
	out := make([]map[string]string, 0, len(values))
	for _, v := range values {
		typ, ref, ok := strings.Cut(strings.TrimSpace(v), ":")
		if !ok || strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("--evidence %q must be type:ref, for example outcome:<id>", v)
		}
		out = append(out, map[string]string{"type": strings.TrimSpace(typ), "ref": strings.TrimSpace(ref)})
	}
	return out, nil
}

func runSeatLessonAdd(cmd *cobra.Command, _ []string) error {
	seat, err := seatFromFlags(cmd)
	if err != nil {
		return err
	}
	kind, _ := cmd.Flags().GetString("kind")
	text, _ := cmd.Flags().GetString("text")
	workClass, _ := cmd.Flags().GetString("work-class")
	evidenceFlags, _ := cmd.Flags().GetStringArray("evidence")
	if strings.TrimSpace(kind) == "" {
		return fmt.Errorf("--kind is required: lesson, pitfall, preference, context or contact")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("--text is required")
	}
	evidence, err := parseSeatEvidence(evidenceFlags)
	if err != nil {
		return err
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	body := map[string]any{"entries": []map[string]any{{
		"kind": kind, "text": text, "work_class": workClass, "evidence": evidence,
	}}}
	var out map[string]any
	if err := client.PostJSON(ctx, "/api/agents/"+seat+"/dossier", body, &out); err != nil {
		return fmt.Errorf("add seat entry: %w", err)
	}
	return cli.PrintJSON(os.Stdout, out)
}

func runSeatLessonList(cmd *cobra.Command, _ []string) error {
	seat, err := seatFromFlags(cmd)
	if err != nil {
		return err
	}
	status, _ := cmd.Flags().GetString("status")
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	query := url.Values{}
	if status != "" {
		query.Set("status", status)
	}
	path := "/api/agents/" + seat + "/dossier"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var out struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := client.GetJSON(ctx, path, &out); err != nil {
		return fmt.Errorf("list seat entries: %w", err)
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return cli.PrintJSON(os.Stdout, out.Entries)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tKIND\tORIGIN\tSTATUS\tWORK CLASS\tTEXT")
	for _, e := range out.Entries {
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\n", e["id"], e["kind"], e["origin"], e["status"], e["work_class"], e["text"])
	}
	return w.Flush()
}

func runSeatLessonRetire(cmd *cobra.Command, args []string) error {
	entry := strings.TrimSpace(args[0])
	if !uuidPattern.MatchString(entry) {
		return fmt.Errorf("%q is not an entry id; run `multica seat lesson list`", entry)
	}
	seat, err := seatFromFlags(cmd)
	if err != nil {
		return err
	}
	reason, _ := cmd.Flags().GetString("reason")
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var out map[string]any
	if err := client.PostJSON(ctx, "/api/agents/"+seat+"/dossier/"+entry+"/retire", map[string]any{"reason": reason}, &out); err != nil {
		return fmt.Errorf("retire seat entry: %w", err)
	}
	return cli.PrintJSON(os.Stdout, out)
}

func init() {
	for _, c := range []*cobra.Command{seatLessonAddCmd, seatLessonListCmd, seatLessonRetireCmd} {
		c.Flags().String("agent", "", "the seat (agent id); defaults to this task's agent")
	}
	seatLessonAddCmd.Flags().String("kind", "", "lesson, pitfall, preference, context or contact")
	seatLessonAddCmd.Flags().String("text", "", "one line the seat should know (800 characters at most)")
	seatLessonAddCmd.Flags().String("work-class", "", "apply the entry to one kind of work only; empty applies to all work")
	seatLessonAddCmd.Flags().StringArray("evidence", nil, "what the entry rests on, as type:ref (outcome, retro, comment or document)")
	seatLessonListCmd.Flags().String("status", "active", "active, retired or all")
	seatLessonListCmd.Flags().Bool("json", false, "print JSON")
	seatLessonRetireCmd.Flags().String("reason", "", "why the entry goes (kept in the governance log)")
	seatLessonCmd.AddCommand(seatLessonAddCmd, seatLessonListCmd, seatLessonRetireCmd)
	seatCmd.AddCommand(seatLessonCmd)
	rootCmd.AddCommand(seatCmd)
}
