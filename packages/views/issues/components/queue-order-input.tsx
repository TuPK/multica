"use client";

import { useEffect, useState } from "react";
import type { Issue } from "@multica/core/types";
import { useQuery } from "@tanstack/react-query";
import { useWorkspaceId } from "@multica/core/hooks";
import { queueFrontiersOptions, queueWaitingOn } from "@multica/core/issues/queries";
import { Input } from "@multica/ui/components/ui/input";
import { useT } from "../../i18n";
import { useQueueOrderApply } from "./use-queue-order-apply";

const MAX_DIGITS = 7;

/** Keeps digits only, so letters and signs never reach the field. */
export function sanitizeQueueOrder(raw: string): string {
  return raw.replace(/\D/g, "").slice(0, MAX_DIGITS);
}

/** Parses the field: empty takes the issue out of the queue, 0 is not a number. */
export function parseQueueOrder(text: string): number | null | undefined {
  if (text === "") return null;
  const n = Number.parseInt(text, 10);
  return n >= 1 ? n : undefined;
}

/**
 * Run-queue number of a backlog issue. Issues with the same number start
 * together once every lower number has finished.
 */
export function QueueOrderInput({
  issue,
  className,
}: {
  issue: Pick<Issue, "id" | "queue_order">;
  className?: string;
}) {
  const { t } = useT("issues");
  const { apply, dialog, isPending } = useQueueOrderApply();
  const stored = issue.queue_order != null ? String(issue.queue_order) : "";
  const [text, setText] = useState(stored);

  useEffect(() => {
    setText(stored);
  }, [stored]);

  const commit = () => {
    const next = parseQueueOrder(text);
    if (next === undefined || (next ?? "") === (issue.queue_order ?? "")) {
      setText(stored);
      return;
    }
    void apply([issue.id], next, { onCancel: () => setText(stored) });
  };

  return (
    <>
      <Input
        value={text}
        inputMode="numeric"
        autoComplete="off"
        aria-label={t(($) => $.queue_order.label)}
        placeholder={t(($) => $.queue_order.placeholder)}
        disabled={isPending}
        className={className}
        onChange={(e) => setText(sanitizeQueueOrder(e.target.value))}
        onBlur={commit}
        onKeyDown={(e) => {
          e.stopPropagation();
          if (e.key === "Enter") e.currentTarget.blur();
          if (e.key === "Escape") setText(stored);
        }}
      />
      {dialog}
    </>
  );
}

/** "Waiting for #N" while lower numbers in the project are unfinished. */
export function QueueWaitingLabel({
  issue,
  className,
}: {
  issue: Pick<Issue, "status" | "queue_order" | "project_id">;
  className?: string;
}) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const { data } = useQuery({
    ...queueFrontiersOptions(wsId),
    enabled: issue.status === "backlog" && issue.queue_order != null,
  });
  const waitingOn = queueWaitingOn(issue, data);
  if (waitingOn === null) return null;
  return <span className={className}>{t(($) => $.queue_order.waiting, { n: waitingOn })}</span>;
}
