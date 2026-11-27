"use client";

import { useOutcomeJury, type JuryPanel, type JuryState, type JuryVoteResult } from "@multica/core/durable/jury";
import type { OutcomeVerdict } from "@multica/core/durable/outcomes";
import { AlertTriangle } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { VerdictChip } from "./verdict-chip";
import { useOutcomesEnabled } from "./use-outcomes-enabled";

const EXPANDED_STATES: ReadonlySet<JuryState> = new Set(["split", "escalated", "timed_out"]);

function toOutcomeVerdict(result: JuryVoteResult): OutcomeVerdict {
  return result === "unable" ? "unverified" : result;
}

/**
 * Disagreement jury panels for an issue's outcome (durable spec 8.3). Shows side-by-side juror
 * votes for split, escalated, or timed-out panels, and a single state line for open, unanimous,
 * or resolved panels. Renders nothing while the outcomes flag is off.
 */
export function JurySplitCard({ wsId, outcomeId }: { wsId: string; outcomeId: string }) {
  const enabled = useOutcomesEnabled();
  if (!enabled || !outcomeId) return null;
  return <JuryPanelsList wsId={wsId} outcomeId={outcomeId} />;
}

function JuryPanelsList({ wsId, outcomeId }: { wsId: string; outcomeId: string }) {
  const { t } = useT("durable");
  const jury = useOutcomeJury(wsId, outcomeId);

  if (jury.isError) {
    return (
      <p role="alert" className="text-caption text-destructive">
        {t(($) => $.outcomes.jury.load_failed)}
      </p>
    );
  }

  const panels = jury.data ?? [];
  if (panels.length === 0) return null;

  return (
    <div className="flex flex-col gap-2" aria-label={t(($) => $.outcomes.jury.title)}>
      {panels.map((panel) => (
        <JuryPanelItem key={panel.id} panel={panel} />
      ))}
    </div>
  );
}

function JuryPanelItem({ panel }: { panel: JuryPanel }) {
  const { t } = useT("durable");
  const expanded = EXPANDED_STATES.has(panel.state);

  return (
    <div className="flex flex-col gap-2 rounded-md border bg-card p-2.5">
      <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-caption">
        <span className="font-medium text-foreground">{t(($) => $.outcomes.jury.state[panel.state])}</span>
        <span className="text-muted-foreground">{t(($) => $.outcomes.jury.diversity[panel.diversity])}</span>
      </div>

      {panel.diversity === "none" ? (
        <div
          role="note"
          className="flex items-start gap-1.5 rounded-sm border border-warning/40 bg-warning/10 px-2 py-1 text-caption text-foreground"
        >
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" aria-hidden="true" />
          <p className="break-words">{t(($) => $.outcomes.jury.same_model_warning)}</p>
        </div>
      ) : null}

      {expanded ? (
        panel.votes.length === 0 ? (
          <p className="text-caption text-muted-foreground">{t(($) => $.outcomes.jury.no_votes)}</p>
        ) : (
          <div className="grid grid-cols-2 gap-2">
            {panel.votes.map((vote, index) => (
              <div key={vote.id} className="flex min-w-0 flex-col gap-1 rounded-sm border bg-muted/30 p-2">
                <div className="flex flex-wrap items-center justify-between gap-1">
                  <span className="text-caption font-medium text-muted-foreground">
                    {t(($) => $.outcomes.jury.juror_label, { number: index + 1 })}
                  </span>
                  <VerdictChip
                    verdict={toOutcomeVerdict(vote.result)}
                    label={t(($) => $.outcomes.result[vote.result])}
                  />
                </div>
                <p className="break-words text-caption text-foreground">
                  {vote.note || t(($) => $.outcomes.jury.no_note)}
                </p>
              </div>
            ))}
          </div>
        )
      ) : null}
    </div>
  );
}
