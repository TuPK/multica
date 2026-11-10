"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Loader2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { useForkHubItem, type HubItemKind } from "@multica/core/hub";
import { useT } from "../i18n";
import { errorMessage } from "./hub-import-request";

/**
 * Detaches a tracked item from its source. The server keeps the files as they are;
 * the item stops receiving updates. Nothing changes until the server confirms.
 */
export function HubForkDialog({
  wsId,
  kind,
  refId,
  name,
  sourceName,
  open,
  onClose,
}: {
  wsId: string;
  kind: HubItemKind;
  refId: string;
  name: string;
  sourceName: string;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useT("hub");
  const fork = useForkHubItem();
  const [error, setError] = useState("");

  const confirm = async () => {
    setError("");
    try {
      await fork.mutateAsync({ wsId, kind, refId });
      toast.success(t(($) => $.fork.toast, { name }));
      onClose();
    } catch (err) {
      setError(errorMessage(err, t(($) => $.fork.failed)));
    }
  };

  return (
    <AlertDialog open={open} onOpenChange={(next) => !next && !fork.isPending && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t(($) => $.fork.title, { name })}</AlertDialogTitle>
          <AlertDialogDescription>{t(($) => $.fork.description, { source: sourceName })}</AlertDialogDescription>
        </AlertDialogHeader>
        {error ? (
          <p role="alert" className="text-caption text-destructive">
            {error}
          </p>
        ) : null}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={fork.isPending}>{t(($) => $.fork.cancel)}</AlertDialogCancel>
          <Button type="button" disabled={fork.isPending} aria-busy={fork.isPending} onClick={() => void confirm()}>
            {fork.isPending ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
            {t(($) => $.fork.confirm)}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
