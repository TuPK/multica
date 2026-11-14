"use client";

import type { FlowIssue, PackCommitResponse } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Check } from "@multica/ui/icons";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";

export interface PackResultProps {
  result: PackCommitResponse;
  /** Where the design of the workflow goes on: the draft the install made, else the squad. */
  designHref: string | null;
  squadHref: string | null;
  onDone: () => void;
}

/** What the install did and where to go on. */
export function PackResult({ result, designHref, squadHref, onDone }: PackResultProps) {
  const { t } = useT("squads");
  const created = {
    agents: result.agents.filter((a) => a.created).length,
    skills: result.skills.filter((s) => s.created).length,
  };
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 text-body font-semibold text-foreground">
        <Check className="size-4 text-success" aria-hidden />
        <span>{t(($) => $.packs.result.title)}</span>
      </div>
      {result.already_installed ? (
        <p className="text-caption text-muted-foreground">{t(($) => $.packs.result.already)}</p>
      ) : (
        <p className="text-caption text-muted-foreground">
          {t(($) => $.packs.result.agents, { count: created.agents })} · {t(($) => $.packs.result.skills, { count: created.skills })}
        </p>
      )}
      {result.flow_issues.length > 0 && (
        <div className="space-y-1 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-caption text-amber-700 dark:text-amber-400">
          <div className="font-medium">{t(($) => $.packs.result.flow_issues)}</div>
          <ul className="list-disc space-y-0.5 pl-5 text-micro">
            {result.flow_issues.slice(0, 3).map((issue: FlowIssue, i) => (
              <li key={i}>{issue.message || issue.code}</li>
            ))}
          </ul>
        </div>
      )}
      <div className="flex flex-wrap items-center justify-end gap-2 border-t border-border pt-4">
        {squadHref && (
          <AppLink href={squadHref} className="text-caption text-muted-foreground underline-offset-2 hover:underline">
            {t(($) => $.packs.open_squad)}
          </AppLink>
        )}
        {designHref && (
          <AppLink href={designHref} className="inline-flex h-8 items-center rounded-md bg-primary px-3 text-caption font-medium text-primary-foreground">
            {t(($) => $.packs.design_workflow)}
          </AppLink>
        )}
        <Button type="button" variant="outline" size="sm" onClick={onDone}>
          {t(($) => $.packs.done)}
        </Button>
      </div>
    </div>
  );
}
