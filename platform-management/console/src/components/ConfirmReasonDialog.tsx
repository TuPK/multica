import { useState, useEffect } from "react";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Field, FieldLabel } from "@multica/ui/components/ui/field";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { AlertCircle } from "@multica/ui/icons";
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
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>

          {children}

          <Field>
            <FieldLabel htmlFor="confirm-reason-input" className="text-label">
              {t("Reason")} <span className="text-destructive">*</span>
            </FieldLabel>
            <Textarea
              id="confirm-reason-input"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("Provide a reason for this action...")}
              rows={3}
              required
              disabled={isPending}
            />
          </Field>

          {error && (
            <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              <AlertDescription className="text-caption">{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" disabled={isPending} />}>
              {t("Cancel")}
            </DialogClose>
            <Button
              type="submit"
              variant={confirmVariant}
              disabled={!reason.trim() || isPending}
              aria-busy={isPending}
            >
              {isPending && <Spinner className="size-3.5" />}
              {confirmLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
