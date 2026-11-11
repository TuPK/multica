"use client";

import { useQuery } from "@tanstack/react-query";
import { canGovern, type AutonomyLevel } from "@multica/core/governance";
import { squadListOptions } from "@multica/core/workspace/queries";
import type { Squad } from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentMember } from "@multica/core/permissions";
import { cn } from "@multica/ui/lib/utils";
import { PAGE_GUTTER } from "../layout/page-header";
import { useT } from "../i18n";
import { GovernancePoliciesSection } from "./governance-policies-section";
import { AUTONOMY_LEVEL_KEYS } from "./autonomy-levels";
import { SquadAutonomyEditor } from "./squad-autonomy-editor";

/**
 * The i18n key of the autonomy note (spec 4.3). The note must stay on this tab:
 * it is what stops L3 being read as "unsupervised".
 */
export const AUTONOMY_NOTE_KEY = "autonomy.note";

const LEVELS: readonly AutonomyLevel[] = [0, 1, 2, 3];

/**
 * The Autonomy tab (spec 4.3): what each level allows, each squad's level with
 * the reason of its last change, and the standing approvals (spec 4.4).
 * Owners and admins change levels and policies; everyone else reads them.
 */
export function GovernanceAutonomyTab() {
  const { t } = useT("governance");
  const wsId = useWorkspaceId();
  const { role } = useCurrentMember(wsId);
  const editable = canGovern(role);
  const { data: squads = [] } = useQuery(squadListOptions(wsId));
  const live = squads.filter((squad) => !squad.archived_at);

  return (
    <div className={cn(PAGE_GUTTER, "flex flex-col gap-8 py-4")}>
      <p className="max-w-prose text-body text-muted-foreground">{t(($) => $.autonomy.note)}</p>

      <section aria-label={t(($) => $.autonomy.levels_heading)} className="flex flex-col gap-2">
        <h2 className="text-heading">{t(($) => $.autonomy.levels_heading)}</h2>
        <ol className="flex max-w-prose flex-col gap-2">
          {LEVELS.map((level) => (
            <li key={level} className="flex flex-col gap-0.5">
              <span className="text-body font-medium">
                {`L${level} ${t(($) => $.autonomy.levels[AUTONOMY_LEVEL_KEYS[level]].name)}`}
              </span>
              <span className="text-body text-muted-foreground">
                {t(($) => $.autonomy.levels[AUTONOMY_LEVEL_KEYS[level]].body)}
              </span>
            </li>
          ))}
        </ol>
        <p className="max-w-prose text-caption text-muted-foreground">{t(($) => $.autonomy.boss_only_note)}</p>
      </section>

      <section aria-label={t(($) => $.autonomy.squads_heading)} className="flex flex-col gap-2">
        <h2 className="text-heading">{t(($) => $.autonomy.squads_heading)}</h2>
        {!editable ? <p className="text-caption text-muted-foreground">{t(($) => $.autonomy.read_only)}</p> : null}
        {live.length === 0 ? (
          <p className="text-body text-muted-foreground">{t(($) => $.autonomy.squads_empty)}</p>
        ) : (
          <ul className="flex flex-col divide-y rounded-md border">
            {live.map((squad) => (
              <SquadAutonomyRow key={squad.id} wsId={wsId} squad={squad} editable={editable} />
            ))}
          </ul>
        )}
      </section>

      <GovernancePoliciesSection wsId={wsId} editable={editable} squads={live} />
    </div>
  );
}

/** One squad's row on the Autonomy tab: its name, then its level and change form. */
function SquadAutonomyRow({ wsId, squad, editable }: { wsId: string; squad: Squad; editable: boolean }) {
  return (
    <li className="flex flex-col gap-3 px-4 py-3">
      <span className="min-w-0 truncate text-body font-medium">{squad.name}</span>
      <SquadAutonomyEditor wsId={wsId} squad={squad} editable={editable} />
    </li>
  );
}
