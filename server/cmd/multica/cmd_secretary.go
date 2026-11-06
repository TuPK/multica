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

var secretaryCmd = &cobra.Command{
	Use:   "secretary",
	Short: "The Secretary's own actions (Secretary only)",
}

var secretaryAskCmd = &cobra.Command{
	Use:   "ask",
	Short: "File a Secretary request for a judgment question, in one call",
	Long: `File the request, one child issue per addressee, the request's metadata, and its
deadline, in one call. Run it from the Secretary's own turn in the boss's chat.

--question-file is the boss's question in full, as Markdown.
--addressees-file is a JSON array; each entry names one addressee with exactly one of
"squad_id" or "agent_id", and gives a "title" and a "description" for its child issue.
At most 20 addressees. The child's description is the ask: the question, why it
matters now, the period, what you already know, and the answer format you want back.

--due-date is the day the answers are due (YYYY-MM-DD); --deadline is the RFC3339
time the deadline wakeup fires, which starts the assessment even if a leader is silent.
If any step fails, the call cancels what it already filed.`,
	Args: cobra.NoArgs,
	RunE: runSecretaryAsk,
}

func init() {
	secretaryAskCmd.Flags().String("chat-session", "", "The chat session of the boss's message you are answering (required)")
	secretaryAskCmd.Flags().String("question-file", "", "Markdown file with the boss's question (required)")
	secretaryAskCmd.Flags().String("addressees-file", "", "JSON file with the addressees (required)")
	secretaryAskCmd.Flags().String("due-date", "", "Day the answers are due, YYYY-MM-DD (required)")
	secretaryAskCmd.Flags().String("deadline", "", "Deadline as an RFC3339 time (required)")
	secretaryCmd.AddCommand(secretaryAskCmd)
}

type secretaryAskAddressee struct {
	SquadID     string `json:"squad_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func runSecretaryAsk(cmd *cobra.Command, _ []string) error {
	session, _ := cmd.Flags().GetString("chat-session")
	questionPath, _ := cmd.Flags().GetString("question-file")
	addresseesPath, _ := cmd.Flags().GetString("addressees-file")
	dueDate, _ := cmd.Flags().GetString("due-date")
	deadline, _ := cmd.Flags().GetString("deadline")
	for name, v := range map[string]string{"--chat-session": session, "--question-file": questionPath, "--addressees-file": addresseesPath, "--due-date": dueDate, "--deadline": deadline} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if err := ensureFileFlagWithinWorkdir(cmd, "question-file", "content", questionPath); err != nil {
		return err
	}
	if err := ensureFileFlagWithinWorkdir(cmd, "addressees-file", "content", addresseesPath); err != nil {
		return err
	}
	question, err := os.ReadFile(questionPath)
	if err != nil {
		return fmt.Errorf("read --question-file: %w", err)
	}
	raw, err := os.ReadFile(addresseesPath)
	if err != nil {
		return fmt.Errorf("read --addressees-file: %w", err)
	}
	var addressees []secretaryAskAddressee
	if err := json.Unmarshal(raw, &addressees); err != nil {
		return fmt.Errorf("--addressees-file must be a JSON array of addressees: %w", err)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	body := map[string]any{
		"question":        strings.TrimSpace(string(question)),
		"chat_session_id": strings.TrimSpace(session),
		"due_date":        dueDate,
		"deadline":        deadline,
		"addressees":      addressees,
	}
	var resp map[string]any
	if err := client.PostJSON(ctx, "/api/secretary/requests", body, &resp); err != nil {
		return fmt.Errorf("file the Secretary request: %w", err)
	}
	return cli.PrintJSON(os.Stdout, resp)
}
