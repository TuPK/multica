package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/quickstart"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

var quickstartCmd = &cobra.Command{
	Use:   "quickstart",
	Short: "Install Claude Code, team plugins and skills on this machine",
	Long: `Prepare this machine to run Claude Code tasks the same way as the
team's reference machine. Anything already present is left alone:

  1. Install the Claude Code CLI through npm if it is missing.
  2. Add the plugin marketplaces and install the plugins that are missing.
  3. Write the workspace skills that are missing into ~/.claude/skills.

Skills come from the workspace you are logged in to, so run "multica login"
(or "multica setup") first. The same install can be started from the Multica
web app on a connected machine.`,
	Args: cobra.NoArgs,
	RunE: runQuickstart,
}

func init() {
	quickstartCmd.Flags().String("output", "table", "Output format: table or json")
}

func runQuickstart(cmd *cobra.Command, _ []string) error {
	if inDaemonTaskIdentityContext() {
		return errors.New("quickstart changes this machine's Claude Code setup and cannot run inside an agent task")
	}
	bundle, err := quickstart.LoadBundle()
	if err != nil {
		return err
	}
	env, err := quickstart.DefaultEnv()
	if err != nil {
		return err
	}
	client, err := newSkillAPIClient(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := cli.StallAwareContext(context.Background())
	defer cancel()
	steps := quickstart.Run(ctx, env, bundle, workspaceSkillSource(client))

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		if err := cli.PrintJSON(os.Stdout, steps); err != nil {
			return err
		}
	} else {
		for _, step := range steps {
			line := fmt.Sprintf("[%s] %s", step.Status, step.Name)
			if step.Detail != "" {
				line += ": " + step.Detail
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	}
	if quickstart.Failed(steps) {
		return errors.New("quickstart finished with failures")
	}
	return nil
}

// workspaceSkillSource reads skills from the workspace through the API.
func workspaceSkillSource(client *cli.APIClient) quickstart.SkillSource {
	return func(ctx context.Context, names []string) (protocol.QuickstartSkillBundles, error) {
		var list []map[string]any
		if err := client.GetJSON(ctx, "/api/skills", &list); err != nil {
			return protocol.QuickstartSkillBundles{}, fmt.Errorf("list workspace skills: %w", err)
		}
		ids := make(map[string]string, len(list))
		for _, s := range list {
			if name := strVal(s, "name"); name != "" {
				if _, seen := ids[name]; !seen {
					ids[name] = strVal(s, "id")
				}
			}
		}

		var out protocol.QuickstartSkillBundles
		for _, name := range names {
			id, ok := ids[name]
			if !ok {
				out.Missing = append(out.Missing, name)
				continue
			}
			var skill protocol.QuickstartSkill
			var raw struct {
				Name    string                         `json:"name"`
				Content string                         `json:"content"`
				Files   []protocol.QuickstartSkillFile `json:"files"`
			}
			if err := client.GetJSON(ctx, "/api/skills/"+id+"?include=content", &raw); err != nil {
				return protocol.QuickstartSkillBundles{}, fmt.Errorf("fetch skill %s: %w", name, err)
			}
			skill.Name = name
			skill.Content = raw.Content
			skill.Files = raw.Files
			out.Skills = append(out.Skills, skill)
		}
		return out, nil
	}
}
