"use client";

import { useState } from "react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { useBatchSetIssueQueueOrder, useSetIssueQueueOrder } from "@multica/core/issues/mutations";
import type { QueueOrderPreview } from "@multica/core/types";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { useT } from "../../i18n";

interface Pending {
  ids: string[];
  order: number | null;
  preview: QueueOrderPreview;
  onDone?: () => void;
  onCancel?: () => void;
}

interface ApplyCallbacks {
  /** Runs after the write succeeded. */
  onDone?: () => void;
  /** Runs when the write failed or the person declined the confirmation. */
  onCancel?: () => void;
}

/**
 * Numbers backlog issues. When the number would start agent runs at once, it
 * asks first, the way a manual move to Todo does; clearing a number or
 * numbering behind unfinished work applies directly.
 */
export function useQueueOrderApply() {
  const { t } = useT("issues");
  const setOne = useSetIssueQueueOrder();
  const setMany = useBatchSetIssueQueueOrder();
  const [pending, setPending] = useState<Pending | null>(null);

  const write = async (ids: string[], order: number | null, cb: ApplyCallbacks) => {
    try {
      if (ids.length === 1) await setOne.mutateAsync({ id: ids[0]!, queueOrder: order });
      else await setMany.mutateAsync({ ids, queueOrder: order });
      cb.onDone?.();
    } catch (err) {
      toast.error(err instanceof Error && err.message ? err.message : t(($) => $.batch.update_failed));
      cb.onCancel?.();
    }
  };

  const apply = async (ids: string[], order: number | null, cb: ApplyCallbacks = {}) => {
    if (ids.length === 0) return;
    if (order !== null) {
      // A failed preview must not block the write; the person still chose the number.
      const preview = await api.previewQueueOrder(ids, order).catch(() => null);
      if (preview && preview.run_count > 0) {
        setPending({ ids, order, preview, ...cb });
        return;
      }
    }
    await write(ids, order, cb);
  };

  const dialog = (
    <AlertDialog
      open={pending !== null}
      onOpenChange={(open) => {
        if (open) return;
        pending?.onCancel?.();
        setPending(null);
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t(($) => $.queue_order.confirm_title)}</AlertDialogTitle>
          <AlertDialogDescription>
            {t(($) => $.queue_order.confirm_desc, {
              count: pending?.preview.release_now ?? 0,
              runs: pending?.preview.run_count ?? 0,
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t(($) => $.batch.cancel)}</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              const current = pending;
              setPending(null);
              if (current) void write(current.ids, current.order, current);
            }}
          >
            {t(($) => $.queue_order.confirm_action)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );

  return { apply, dialog, isPending: setOne.isPending || setMany.isPending };
}
