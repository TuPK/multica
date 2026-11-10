package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/salescollect"
)

// `multica sales` reads the sources of a Sales Loop squad without a model. The Scout runs it
// and reads its JSON; the collector never writes to the knowledge base itself, so the Scout
// records the outcome in the source's document (spec 4.11, 7.5).

var salesCmd = &cobra.Command{
	Use:   "sales",
	Short: "Read the sources of a Sales Loop squad",
	Long:  "Probe a link or handle to find how it can be read, and collect a source's items from its recipe. Both print JSON.",
}

var salesProbeCmd = &cobra.Command{
	Use:   "probe <input>",
	Short: "Find how an input can be read, and sample it",
	Long: "Names the platform of a link, fetches it once, tries each open format it suggests (feed, JSON, JSON-LD), and runs each on a sample. " +
		"A social platform is sent to its connection with the sentence that unlocks it. Uses no model, no paid provider, and no login.",
	Args: cobra.ExactArgs(1),
	RunE: runSalesProbe,
}

var salesCollectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Run a source's recipe and print its signals",
	Long: "Reads one source from its recipe file: fetches it, filters the items, normalises them to signals with a family and an entity key, " +
		"and reports the coverage of the four fields. Exits non-zero when the source cannot be read.",
	Args: cobra.NoArgs,
	RunE: runSalesCollect,
}

func init() {
	salesProbeCmd.Flags().Int("sample", 20, "Items to keep in each candidate's sample")

	salesCollectCmd.Flags().String("recipe-file", "", "A JSON file with the source's recipe (the Recipe section of its document) — required")
	salesCollectCmd.Flags().String("csv", "", "For a csv_drop recipe: the CSV file to read")
	salesCollectCmd.Flags().String("source-id", "", "The sales_source document this recipe belongs to, copied to each signal")
	salesCollectCmd.Flags().String("region", "", "The lane's region, copied to each signal")
	salesCollectCmd.Flags().String("sector", "", "The lane's sector hint, copied to each signal")
	salesCollectCmd.Flags().Int("limit", 0, "Most signals to keep (default: the recipe's max_items)")
	salesCollectCmd.Flags().Bool("dry-run", false, "Read a sample of 5 items and write nothing")

	salesCmd.AddCommand(salesProbeCmd)
	salesCmd.AddCommand(salesCollectCmd)
	rootCmd.AddCommand(salesCmd)
}

func runSalesProbe(cmd *cobra.Command, args []string) error {
	sample, _ := cmd.Flags().GetInt("sample")
	if sample <= 0 {
		return errors.New("--sample must be a positive number")
	}
	report := salescollect.Probe(commandContext(cmd), args[0], salescollect.Options{Limit: sample})
	return writeJSON(cmd.OutOrStdout(), report)
}

func runSalesCollect(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("recipe-file")
	if path == "" {
		return errors.New("--recipe-file is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read recipe: %w", err)
	}
	recipe, err := salescollect.ParseRecipe(raw)
	if err != nil {
		return err
	}
	csvPath, _ := cmd.Flags().GetString("csv")
	sourceID, _ := cmd.Flags().GetString("source-id")
	region, _ := cmd.Flags().GetString("region")
	sector, _ := cmd.Flags().GetString("sector")
	limit, _ := cmd.Flags().GetInt("limit")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun && (limit == 0 || limit > 5) {
		limit = 5
	}
	result := salescollect.Run(commandContext(cmd), recipe, salescollect.Options{
		CSVPath: csvPath, SourceID: sourceID, Region: region, Sector: sector, Limit: limit,
	})
	if err := writeJSON(cmd.OutOrStdout(), result); err != nil {
		return err
	}
	if result.Status == "error" {
		cmd.SilenceUsage = true
		return fmt.Errorf("the source could not be read: %s", result.Error)
	}
	return nil
}

// commandContext is the command's context, or a background one when the command is run
// outside cobra (in tests).
func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
