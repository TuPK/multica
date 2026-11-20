import { useState, useEffect } from "react";
import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { TextArea } from "@astryxdesign/core/TextArea";
import { t } from "../i18n";

interface ConfirmReasonDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  confirmLabel?: string;
  confirmVariant?: "default" | "destructive";
  isPending?: boolean;
  error?: string | null;
  children?: React.ReactNode;
  onConfirm: (reason: string) => Promise<void> | void;
}

export function ConfirmReasonDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = t("Confirm"),
  confirmVariant = "default",
  isPending = false,
  error = null,
  children,
  onConfirm,
}: ConfirmReasonDialogProps) {
  const [reason, setReason] = useState("");

  useEffect(() => {
    if (open) {
      setReason("");
    }
  }, [open]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!reason.trim() || isPending) return;
    await onConfirm(reason.trim());
  };

  return (
    <Dialog isOpen={open} onOpenChange={onOpenChange} purpose="form">
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <DialogHeader title={title} subtitle={description} />

        {children}

        <TextArea
          label={t("Reason")}
          isRequired
          value={reason}
          onChange={(value) => setReason(value)}
          placeholder={t("Provide a reason for this action...")}
          rows={3}
          isDisabled={isPending}
          width="100%"
        />

        {error && <Banner role="alert" status="error" title={error} />}

        <div className="flex justify-end gap-2">
          <Button
            label={t("Cancel")}
            type="button"
            variant="secondary"
            isDisabled={isPending}
            onClick={() => onOpenChange(false)}
          />
          <Button
            label={confirmLabel}
            type="submit"
            variant={confirmVariant === "destructive" ? "destructive" : "primary"}
            isDisabled={!reason.trim() || isPending}
            isLoading={isPending}
            aria-busy={isPending}
          />
        </div>
      </form>
    </Dialog>
  );
}
