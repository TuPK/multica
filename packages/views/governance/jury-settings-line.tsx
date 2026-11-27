"use client";

import { parseWorkspaceJurySettings } from "@multica/core/durable/jury";
import { useCurrentWorkspace } from "@multica/core/paths";
import { useT } from "../i18n";

/**
 * Read-only summary of the workspace's disagreement jury settings on Governance → Controls
 * (durable spec 8.2): pool size, configured work-class prefixes, and the spec's activation rule.
 */
export function JurySettingsLine() {
  const { t } = useT("durable");
  const workspace = useCurrentWorkspace();
  const { jury_pool, jury_work_classes } = parseWorkspaceJurySettings(workspace?.settings ?? {});

  return (
    <section className="flex flex-col gap-1.5" aria-labelledby="governance-jury-heading">
      <h2 id="governance-jury-heading" className="text-title-sm font-medium">
        {t(($) => $.outcomes.jury.controls.heading)}
      </h2>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-body text-foreground">
        <span>
          {jury_pool.length === 0
            ? t(($) => $.outcomes.jury.controls.empty)
            : t(($) => $.outcomes.jury.controls.pool, { count: jury_pool.length })}
        </span>
        {jury_work_classes.length > 0 ? (
          <>
            <span aria-hidden="true" className="text-muted-foreground">
              ·
            </span>
            <span className="break-all text-caption text-muted-foreground">
              {t(($) => $.outcomes.jury.controls.work_classes_label, {
                classes: jury_work_classes.join(", "),
              })}
            </span>
          </>
        ) : null}
      </div>
      <p className="text-caption text-muted-foreground">{t(($) => $.outcomes.jury.controls.rule)}</p>
    </section>
  );
}
