"use client";

import { useIntents } from "@multica/core/durable/intents";
import { groupIntents } from "@multica/core/durable/intents";
import { useWorkspaceId } from "@multica/core/hooks";
import { useT } from "../../i18n";
import { IntentBatchCard } from "./intent-batch-card";
import { IntentCard } from "./intent-card";
import { useEffectsEnabled } from "./use-effects-enabled";

/**
 * The decisions waiting for a person, for the inbox (durable spec 5.5). Renders nothing while
 * durable effects is off, and nothing when no decision is open.
 */
export function PendingIntents() {
  const enabled = useEffectsEnabled();
  if (!enabled) return null;
  return <PendingIntentList />;
}

function PendingIntentList() {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const intents = useIntents(wsId, "pending");
  const groups = groupIntents(intents.data ?? []);
  if (groups.length === 0) return null;

  return (
    <section aria-labelledby="pending-intents-heading" className="flex flex-col gap-3 p-4">
      <h2 id="pending-intents-heading" className="text-title-sm font-medium">
        {t(($) => $.intent.title)}
      </h2>
      {groups.map((group) =>
        group.kind === "batch" ? (
          <IntentBatchCard key={`batch-${group.key}`} intents={group.intents} />
        ) : (
          <IntentCard key={group.intent.id} intent={group.intent} />
        ),
      )}
    </section>
  );
}
