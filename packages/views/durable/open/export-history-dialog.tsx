"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { OCF_EXPORT_FILENAME, useExportOCF } from "@multica/core/durable/ocf";
import { usePrincipalProfile } from "@multica/core/durable/principal";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@multica/ui/components/ui/dialog";
import { Label } from "@multica/ui/components/ui/label";
import { Download, Loader2 } from "@multica/ui/icons";
import { saveBlob } from "../../governance/save-blob";
import { useT } from "../../i18n";
import { useReachEnabled } from "../reach/use-reach-enabled";

/**
 * Dialog for downloading the workspace in Open Company Format v1 (company.ocf.zip)
 * with optional history and consented principal profile (durable spec 12.2, F11).
 * Hidden while durable_open is off.
 */
export function ExportHistoryDialog({
  open,
  onOpenChange,
  trigger,
}: {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  trigger?: ReactNode;
}) {
  const enabled = useReachEnabled();
  if (!enabled) return null;
  return (
    <ExportHistoryDialogInner
      open={open}
      onOpenChange={onOpenChange}
      trigger={trigger}
    />
  );
}

function ExportHistoryDialogInner({
  open: controlledOpen,
  onOpenChange,
  trigger,
}: {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  trigger?: ReactNode;
}) {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
  const open = controlledOpen ?? uncontrolledOpen;
  const setOpen = (next: boolean) => {
    setUncontrolledOpen(next);
    onOpenChange?.(next);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {trigger ? (
        <DialogTrigger render={<span className="inline-flex" />}>{trigger}</DialogTrigger>
      ) : null}
      <DialogContent>
        <ExportHistoryForm
          wsId={wsId}
          onExported={() => setOpen(false)}
          cancelLabel={t(($) => $.open_export.cancel)}
        />
      </DialogContent>
    </Dialog>
  );
}

function ExportHistoryForm({
  wsId,
  onExported,
  cancelLabel,
}: {
  wsId: string;
  onExported: () => void;
  cancelLabel: string;
}) {
  const { t } = useT("durable");
  const principalProfile = usePrincipalProfile(wsId);
  const exportOcf = useExportOCF(wsId);
  const [history, setHistory] = useState(false);
  const [includePrincipal, setIncludePrincipal] = useState(false);

  const hasPrincipalConsent = Boolean(principalProfile.data);
  const showPrincipalOption = history && hasPrincipalConsent;

  const handleHistoryChange = (checked: boolean) => {
    setHistory(checked);
    if (!checked) setIncludePrincipal(false);
  };

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (exportOcf.isPending) return;
    exportOcf
      .mutateAsync({
        history,
        include_principal: showPrincipalOption && includePrincipal,
      })
      .then(
        (blob) => {
          saveBlob(blob, OCF_EXPORT_FILENAME);
          toast.success(t(($) => $.open_export.exported));
          onExported();
        },
        () => {
          toast.error(t(($) => $.open_export.export_failed));
        },
      );
  };

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{t(($) => $.open_export.title)}</DialogTitle>
        <DialogDescription>{t(($) => $.open_export.description)}</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3">
        <div className="flex items-start gap-2.5 rounded-md border border-surface-border p-3">
          <Checkbox
            id="ocf-export-history"
            checked={history}
            disabled={exportOcf.isPending}
            onCheckedChange={(checked) => handleHistoryChange(Boolean(checked))}
            className="mt-0.5"
          />
          <div className="flex flex-col gap-0.5">
            <Label htmlFor="ocf-export-history" className="text-body font-medium leading-snug">
              {t(($) => $.open_export.include_history)}
            </Label>
            <p className="text-caption text-muted-foreground">
              {t(($) => $.open_export.include_history_hint)}
            </p>
          </div>
        </div>

        {showPrincipalOption ? (
          <div className="flex items-start gap-2.5 rounded-md border border-surface-border p-3">
            <Checkbox
              id="ocf-export-include-principal"
              checked={includePrincipal}
              disabled={exportOcf.isPending}
              onCheckedChange={(checked) => setIncludePrincipal(Boolean(checked))}
              className="mt-0.5"
            />
            <div className="flex flex-col gap-0.5">
              <Label
                htmlFor="ocf-export-include-principal"
                className="text-body font-medium leading-snug"
              >
                {t(($) => $.open_export.include_principal)}
              </Label>
              <p className="text-caption text-muted-foreground">
                {t(($) => $.open_export.include_principal_hint)}
              </p>
            </div>
          </div>
        ) : null}
      </div>

      <DialogFooter>
        <DialogClose
          render={<Button type="button" variant="outline" disabled={exportOcf.isPending} />}
        >
          {cancelLabel}
        </DialogClose>
        <Button
          type="submit"
          disabled={exportOcf.isPending}
          aria-busy={exportOcf.isPending || undefined}
        >
          {exportOcf.isPending ? (
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          ) : (
            <Download className="size-4" aria-hidden="true" />
          )}
          {t(($) => $.open_export.export)}
        </Button>
      </DialogFooter>
    </form>
  );
}
