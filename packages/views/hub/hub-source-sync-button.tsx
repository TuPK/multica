"use client";

import { toast } from "sonner";
import { Loader2, RefreshCw } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { useSyncHubSource, type HubSource } from "@multica/core/hub";
import { useT } from "../i18n";
import { errorMessage } from "./hub-import-request";
import { syncCounts } from "./hub-source-sync";

/** Runs one manual sync and reports the outcome in a toast. The row shows the stored status once the list refreshes. */
export function SyncSourceButton({ wsId, source }: { wsId: string; source: HubSource }) {
  const { t } = useT("hub");
  const sync = useSyncHubSource();

  const run = async () => {
    try {
      const result = await sync.mutateAsync({ wsId, id: source.id });
      if (result.status === "error") {
        toast.error(result.error || t(($) => $.sync.failed));
        return;
      }
      const counts = syncCounts(result);
      toast.success(
        t(($) => $.sync.done, {
          checked: counts.checked,
          updates: counts.updates,
          unchanged: counts.unchanged,
          errors: counts.errors,
        }),
      );
    } catch (err) {
      toast.error(errorMessage(err, t(($) => $.sync.failed)));
    }
  };

  return (
    <Button
      variant="ghost"
      size="icon-xs"
      disabled={sync.isPending || !source.enabled}
      aria-busy={sync.isPending}
      aria-label={t(($) => $.sync.aria, { name: source.name })}
      title={source.enabled ? t(($) => $.sync.now) : t(($) => $.sync.disabled_hint)}
      onClick={() => void run()}
    >
      {sync.isPending ? (
        <Loader2 className="animate-spin" aria-hidden="true" />
      ) : (
        <RefreshCw aria-hidden="true" />
      )}
    </Button>
  );
}
