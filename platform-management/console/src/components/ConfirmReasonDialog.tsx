import { useState, useEffect } from "react";
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
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Spinner } from "@multica/ui/components/ui/spinner";

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
  confirmLabel = "Confirm",
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

          <div className="flex flex-col gap-1.5">
            <label htmlFor="confirm-reason-input" className="text-caption font-medium text-foreground">
              Reason <span className="text-destructive">*</span>
            </label>
            <Textarea
              id="confirm-reason-input"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Provide a reason for this action..."
              rows={3}
              required
              disabled={isPending}
            />
          </div>

          {error && (
            <div role="alert" className="text-caption text-destructive">
              {error}
            </div>
          )}

          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" disabled={isPending} />}>
              Cancel
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
