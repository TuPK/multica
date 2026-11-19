import { useEffect, useId, useState, type FormEvent, type ReactNode } from "react";
import { Field } from "@astryxdesign/core";
import { AlertCircle } from "@multica/ui/icons";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { getApiErrorMessage } from "../api/client";

export interface TypeToConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  expectedText: string;
  confirmInputLabel?: string;
  confirmInputPlaceholder?: string;
  reasonLabel?: string;
  reasonPlaceholder?: string;
  caseSensitive?: boolean;
  confirmLabel?: string;
  confirmVariant?: "default" | "destructive";
  error?: string | null;
  isConfirmDisabled?: boolean;
  onConfirm: (values: { reason: string; confirm: string }) => Promise<void> | void;
  children?: ReactNode;
}

export function TypeToConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  expectedText,
  confirmInputLabel = "Confirmation",
  confirmInputPlaceholder,
  reasonLabel = "Reason",
  reasonPlaceholder = "Explain why this action is being taken...",
  caseSensitive = true,
  confirmLabel = "Confirm",
  confirmVariant = "destructive",
  error = null,
  isConfirmDisabled = false,
  onConfirm,
  children,
}: TypeToConfirmDialogProps) {
  const [reason, setReason] = useState("");
  const [confirmText, setConfirmText] = useState("");
  const [pending, setPending] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const reasonId = useId();
  const confirmId = useId();

  useEffect(() => {
    if (open) {
      setReason("");
      setConfirmText("");
      setLocalError(null);
    }
  }, [open]);

  const trimmedReason = reason.trim();
  const trimmedConfirm = confirmText.trim();
  const normalizedExpected = expectedText.trim();
  const textMatches =
    normalizedExpected !== "" &&
    (caseSensitive
      ? trimmedConfirm === normalizedExpected
      : trimmedConfirm.toLowerCase() === normalizedExpected.toLowerCase());

  const canSubmit =
    trimmedReason.length > 0 && textMatches && !isConfirmDisabled && !pending;

  const displayError = error || localError;

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setPending(true);
    setLocalError(null);
    try {
      await onConfirm({
        reason: trimmedReason,
        confirm: trimmedConfirm,
      });
    } catch (err) {
      setLocalError(getApiErrorMessage(err, "Request failed"));
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !pending && onOpenChange(next)}>
      <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description && <DialogDescription>{description}</DialogDescription>}
          </DialogHeader>

          {displayError && (
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>{displayError}</AlertDescription>
            </Alert>
          )}

          {children}

          <Field
            label={reasonLabel}
            inputID={reasonId}
            isRequired
            width="100%"
          >
            <Textarea
              id={reasonId}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={reasonPlaceholder}
              disabled={pending}
              rows={3}
            />
          </Field>

          <Field
            label={confirmInputLabel}
            inputID={confirmId}
            description={expectedText ? `Type "${expectedText}" to confirm.` : undefined}
            isRequired
            width="100%"
          >
            <Input
              id={confirmId}
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder={confirmInputPlaceholder ?? expectedText}
              disabled={pending}
              autoComplete="off"
            />
          </Field>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={pending}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant={confirmVariant}
              disabled={!canSubmit}
              aria-busy={pending || undefined}
            >
              {pending && <Spinner />}
              {confirmLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
