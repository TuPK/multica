package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

var principalPredictCmd = &cobra.Command{
	Use:   "predict",
	Short: "Record the Secretary's prediction of what a person will decide on a question",
	Long: `The Secretary predicts a person's decision on a pending question. The prediction shows
next to the question, and the person's decision resolves it.

  multica principal predict --member <user-id> --subject proposal:<id> --kind proposal:hire --predicted approve --confidence 0.8 --why "Same as the last two hires" --traits <trait-id>,<trait-id>`,
	Args: cobra.NoArgs,
	RunE: runPrincipalPredict,
}

// parsePredictSubject reads --subject values of the form type:id, where type is one of the questions
// a prediction is made for.
func parsePredictSubject(value string) (string, string, error) {
	typ, id, ok := strings.Cut(strings.TrimSpace(value), ":")
	typ, id = strings.TrimSpace(typ), strings.TrimSpace(id)
	if !ok || typ == "" || !uuidPattern.MatchString(id) {
		return "", "", fmt.Errorf("--subject %q must be type:id, for example proposal:<id>", value)
	}
	return typ, id, nil
}

func runPrincipalPredict(cmd *cobra.Command, _ []string) error {
	member, _ := cmd.Flags().GetString("member")
	subject, _ := cmd.Flags().GetString("subject")
	kind, _ := cmd.Flags().GetString("kind")
	predicted, _ := cmd.Flags().GetString("predicted")
	confidence, _ := cmd.Flags().GetFloat64("confidence")
	why, _ := cmd.Flags().GetString("why")
	traits, _ := cmd.Flags().GetStringSlice("traits")
	if !uuidPattern.MatchString(strings.TrimSpace(member)) {
		return fmt.Errorf("--member is required: the person's user id")
	}
	subjectType, subjectID, err := parsePredictSubject(subject)
	if err != nil {
		return err
	}
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(predicted) == "" {
		return fmt.Errorf("--kind and --predicted are required")
	}
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("--confidence must be between 0 and 1")
	}
	for i, id := range traits {
		traits[i] = strings.TrimSpace(id)
		if !uuidPattern.MatchString(traits[i]) {
			return fmt.Errorf("--traits %q is not a trait id", id)
		}
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	body := map[string]any{
		"member_id": strings.TrimSpace(member), "subject_type": subjectType, "subject_id": subjectID,
		"decision_kind": kind, "predicted": predicted, "confidence": confidence, "why": why, "trait_ids": traits,
	}
	if task := strings.TrimSpace(os.Getenv("MULTICA_TASK_ID")); uuidPattern.MatchString(task) {
		body["task_id"] = task
	}
	var out map[string]any
	if err := client.PostJSON(ctx, "/api/principal/predictions", body, &out); err != nil {
		return fmt.Errorf("record prediction: %w", err)
	}
	return cli.PrintJSON(os.Stdout, out)
}

func init() {
	principalPredictCmd.Flags().String("member", "", "the person's user id")
	principalPredictCmd.Flags().String("subject", "", "the question as type:id, for example proposal:<id>")
	principalPredictCmd.Flags().String("kind", "", "the decision kind, for example proposal:hire")
	principalPredictCmd.Flags().String("predicted", "", "what the person would decide, for example approve or reject")
	principalPredictCmd.Flags().Float64("confidence", 0, "confidence between 0 and 1")
	principalPredictCmd.Flags().String("why", "", "why, in one or two sentences (1000 characters at most)")
	principalPredictCmd.Flags().StringSlice("traits", nil, "the trait ids the prediction rests on")
	principalCmd.AddCommand(principalPredictCmd)
}
