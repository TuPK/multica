"use client";

import { useState } from "react";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { ArrowUpCircle } from "@multica/ui/icons";
import type { HubItemKind, HubItemMeta } from "@multica/core/hub";
import { useT } from "../i18n";
import { HubUpdateDialog } from "./hub-update-dialog";

/** Compact marker for a Hub row or card whose tracked source has a newer version. Text, not colour alone. */
export function UpdateAvailableBadge() {
  const { t } = useT("hub");
  return <Badge variant="secondary">{t(($) => $.update.available)}</Badge>;
}

/**
 * Item-page banner while a tracked item has an update. Only a user who may edit
 * the item gets the review action; everyone else sees the notice.
 */
export function HubUpdateBanner({
  wsId,
  kind,
  refId,
  name,
  meta,
  canEdit,
}: {
  wsId: string;
  kind: HubItemKind;
  refId: string;
  name: string;
  meta: HubItemMeta | undefined;
  canEdit: boolean;
}) {
  const { t } = useT("hub");
  const [reviewing, setReviewing] = useState(false);
  if (!meta || meta.tracking !== "tracking" || !meta.update_available) return null;

  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-info/30 bg-info/10 px-3 py-2 text-label"
    >
      <ArrowUpCircle className="size-4 shrink-0 text-info" aria-hidden="true" />
      <span className="min-w-0 flex-1">{t(($) => $.update.banner)}</span>
      {canEdit ? (
        <Button type="button" variant="outline" size="sm" onClick={() => setReviewing(true)}>
          {t(($) => $.update.review)}
        </Button>
      ) : null}
      {canEdit ? (
        <HubUpdateDialog
          wsId={wsId}
          kind={kind}
          refId={refId}
          name={name}
          open={reviewing}
          onClose={() => setReviewing(false)}
        />
      ) : null}
    </div>
  );
}
