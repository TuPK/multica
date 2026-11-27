"use client";

import {
  usePrincipalPrediction,
  type PrincipalPredictionSubjectType,
} from "@multica/core/durable/principal";
import { useWorkspaceId } from "@multica/core/hooks";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { predictedDecisionLabel } from "./error-copy";
import { usePrincipalEnabled } from "./use-principal-enabled";

/**
 * One-line prediction of what the signed-in person would decide for a card's subject
 * (durable spec 6.4). Fetches only when `durable_principal` is on, and renders nothing
 * while loading, on 404, or on any error.
 */
export function PrincipalPredictionLine({
  subjectType,
  subjectId,
  className,
}: {
  subjectType: PrincipalPredictionSubjectType;
  subjectId: string;
  className?: string;
}) {
  const enabled = usePrincipalEnabled();
  if (!enabled || !subjectId) return null;
  return <PredictionLineBody subjectType={subjectType} subjectId={subjectId} className={className} />;
}

function PredictionLineBody({
  subjectType,
  subjectId,
  className,
}: {
  subjectType: PrincipalPredictionSubjectType;
  subjectId: string;
  className?: string;
}) {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const query = usePrincipalPrediction(wsId, subjectType, subjectId);

  if (query.isPending || query.isError || !query.data) return null;

  const { predicted, confidence, why } = query.data;
  return (
    <span
      title={why || undefined}
      className={cn("block text-caption tabular-nums text-muted-foreground", className)}
    >
      {t(($) => $.principal.prediction.line, {
        predicted: predictedDecisionLabel(predicted, t),
        confidence: Math.round(confidence * 100),
      })}
    </span>
  );
}
