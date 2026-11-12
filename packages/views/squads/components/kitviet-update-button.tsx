"use client";

import { useState } from "react";
import {
  useUpgradeKitvietSquads,
  type KitvietWorkspaceUpgradeEntry,
} from "@multica/core/squads/kitviet-update";
import { Loader2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { toast } from "sonner";

import { useT } from "../../i18n";

function countAgents(entry: KitvietWorkspaceUpgradeEntry) {
  const count = (status: string) => entry.result.agents.filter((a) => a.status === status).length;
  return {
    upgraded: count("upgraded"),
    current: count("current"),
    kept: count("kept"),
    failed: count("failed"),
  };
}

/**
 * One button that brings the workspace's Kit Viet squads to the current pack, then shows what it
 * changed. The button is shown to workspace admins only, and only when a Kit Viet squad exists.
 */
export function KitvietUpdateButton({ wsId, visible }: { wsId: string; visible: boolean }) {
  const { t } = useT("squads");
  const [entries, setEntries] = useState<KitvietWorkspaceUpgradeEntry[] | null>(null);
  const update = useUpgradeKitvietSquads(wsId);

  if (!visible) return null;

  const statusText = (status: string): string => {
    switch (status) {
      case "upgraded":
        return t(($) => $.kitviet_update.statuses.upgraded);
      case "current":
        return t(($) => $.kitviet_update.statuses.current);
      case "kept":
        return t(($) => $.kitviet_update.statuses.kept);
      case "missing":
        return t(($) => $.kitviet_update.statuses.missing);
      case "failed":
        return t(($) => $.kitviet_update.statuses.failed);
      case "ready":
        return t(($) => $.kitviet_update.statuses.ready);
      case "partial":
        return t(($) => $.kitviet_update.statuses.partial);
      default:
        return t(($) => $.kitviet_update.statuses.skipped);
    }
  };

  const run = async () => {
    try {
      const done = await update.mutateAsync();
      setEntries(done);
      const problems = done.some(
        (e) => e.result.squad.status === "failed" || e.result.agents.some((a) => a.status === "failed") || e.result.skills.status === "failed",
      );
      if (problems) toast.error(t(($) => $.kitviet_update.toast_problems));
      else toast.success(t(($) => $.kitviet_update.toast_done));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(($) => $.kitviet_update.toast_failed));
    }
  };

  return (
    <>
      <Button type="button" variant="outline" size="sm" disabled={update.isPending} onClick={run}>
        {update.isPending ? (
          <>
            <Loader2 className="mr-1 size-3.5 animate-spin" />
            {t(($) => $.kitviet_update.running)}
          </>
        ) : (
          t(($) => $.kitviet_update.button)
        )}
      </Button>

      <Dialog open={entries !== null} onOpenChange={(open) => { if (!open) setEntries(null); }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t(($) => $.kitviet_update.title)}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 text-body">
            {entries?.length === 0 ? (
              <p>{t(($) => $.kitviet_update.none)}</p>
            ) : (
              entries?.map((entry) => {
                const agents = countAgents(entry);
                return (
                  <section key={entry.templateId} className="space-y-1">
                    <h3 className="font-medium">{entry.name}</h3>
                    <p>
                      {t(($) => $.kitviet_update.agents, agents)}
                    </p>
                    <p>
                      {t(($) => $.kitviet_update.squad, { status: statusText(entry.result.squad.status) })}
                    </p>
                    <p>
                      {t(($) => $.kitviet_update.skills, { status: statusText(entry.result.skills.status) })}
                    </p>
                    {entry.result.squad.detail ? (
                      <p className="text-muted-foreground">{entry.result.squad.detail}</p>
                    ) : null}
                  </section>
                );
              })
            )}
            <p className="text-muted-foreground">{t(($) => $.kitviet_update.kept_note)}</p>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" size="sm" onClick={() => setEntries(null)}>
              {t(($) => $.kitviet_update.close)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
