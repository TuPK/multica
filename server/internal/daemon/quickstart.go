package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/multica-ai/multica/server/internal/quickstart"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// quickstartRunTimeout stays under the server's ten-minute running window so
// the daemon reports its own result before the server times the request out.
const quickstartRunTimeout = 9 * time.Minute

// handleQuickstart installs the claimed bundle on this machine and reports
// the per-item steps. Items already present are reported, not reinstalled.
func (d *Daemon) handleQuickstart(ctx context.Context, rt Runtime, pending PendingQuickstart) {
	runCtx, cancel := context.WithTimeout(ctx, quickstartRunTimeout)
	defer cancel()

	env, err := quickstart.DefaultEnv()
	if err != nil {
		d.reportQuickstartFailure(ctx, rt, pending.ID, err.Error(), nil)
		return
	}

	steps := quickstart.Run(runCtx, env, pending.Bundle, d.quickstartSkillSource(rt, pending.ID))
	d.reportRuntimeResultWithRetry(ctx, "quickstart", rt.ID, pending.ID, func(ctx context.Context) error {
		return d.client.ReportRuntimeQuickstartResult(ctx, rt.ID, pending.ID, map[string]any{
			"status": "completed",
			"steps":  steps,
		})
	})
	d.logger.Info("quickstart finished", "runtime_id", rt.ID, "request_id", pending.ID, "failed", quickstart.Failed(steps))
}

// quickstartSkillSource fetches the workspace skills once per run and returns
// only the names the install asked for. Names missing from the response
// are reported as missing so the step fails with a clear reason.
func (d *Daemon) quickstartSkillSource(rt Runtime, requestID string) quickstart.SkillSource {
	return func(ctx context.Context, names []string) (protocol.QuickstartSkillBundles, error) {
		all, err := d.client.GetRuntimeQuickstartSkills(ctx, rt.ID, requestID)
		if err != nil {
			return protocol.QuickstartSkillBundles{}, fmt.Errorf("fetch skills: %w", err)
		}
		byName := make(map[string]protocol.QuickstartSkill, len(all.Skills))
		for _, s := range all.Skills {
			byName[s.Name] = s
		}
		var out protocol.QuickstartSkillBundles
		for _, name := range names {
			if skill, ok := byName[name]; ok {
				out.Skills = append(out.Skills, skill)
			} else {
				out.Missing = append(out.Missing, name)
			}
		}
		return out, nil
	}
}

func (d *Daemon) reportQuickstartFailure(ctx context.Context, rt Runtime, requestID, message string, steps []protocol.QuickstartStep) {
	d.logger.Warn("quickstart failed", "runtime_id", rt.ID, "request_id", requestID, "error", message)
	d.reportRuntimeResultWithRetry(ctx, "quickstart", rt.ID, requestID, func(ctx context.Context) error {
		return d.client.ReportRuntimeQuickstartResult(ctx, rt.ID, requestID, map[string]any{
			"status": "failed",
			"steps":  steps,
			"error":  message,
		})
	})
}
