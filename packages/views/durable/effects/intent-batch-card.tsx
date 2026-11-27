"use client";

import { toast } from "sonner";
import { useApproveIntent, useApproveIntentBatch, type Intent } from "@multica/core/durable/intents";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";
import { PrincipalPredictionLine } from "../principal/prediction-line";

/**
 * Pending intents of one batch key, as one card (durable spec 5.5). "Approve all" decides them
 * in one request and reports how many went through; each row can still be approved alone.
 */
export function IntentBatchCard({ intents }: { intents: Intent[] }) {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const approveBatch = useApproveIntentBatch(wsId);
  const approveOne = useApproveIntent(wsId);
  const busy = approveBatch.isPending || approveOne.isPending;

  const approveAll = () => {
    approveBatch
      .mutateAsync({ ids: intents.map((intent) => intent.id) })
      .then(
        (results) => {
          if (results.length === 0) {
            toast.error(t(($) => $.batch.approve_all_failed));
            return;
          }
          const approved = results.filter((r) => r.status === 200).length;
          const failed = results.length - approved;
          if (failed === 0) toast.success(t(($) => $.batch.approved_all, { count: approved }));
          else toast.success(t(($) => $.batch.approved_partial, { approved, failed }));
        },
        () => {
          toast.error(t(($) => $.batch.approve_all_failed));
        },
      );
  };

  const approveRow = (intent: Intent) => {
    approveOne.mutateAsync({ id: intent.id }).then(
      () => toast.success(t(($) => $.intent.approved)),
      () => toast.error(t(($) => $.intent.approve_failed)),
    );
  };

  return (
    <section aria-label={t(($) => $.batch.title, { count: intents.length })} className="flex flex-col gap-3 rounded-lg border bg-card p-4">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-title-sm font-medium">{t(($) => $.batch.title, { count: intents.length })}</h3>
        <Button type="button" size="sm" disabled={busy} aria-busy={busy} onClick={approveAll}>
          {t(($) => $.batch.approve_all)}
        </Button>
      </header>
      <ul className="flex flex-col divide-y">
        {intents.map((intent) => (
          <li key={intent.id} className="flex items-center justify-between gap-3 py-2">
            <div className="flex min-w-0 flex-col gap-0.5">
              <span className="min-w-0 break-words text-body">{intent.preview.title || intent.target}</span>
              <PrincipalPredictionLine subjectType="effect_intent" subjectId={intent.id} />
            </div>
            <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => approveRow(intent)}>
              {t(($) => $.intent.approve)}
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}
