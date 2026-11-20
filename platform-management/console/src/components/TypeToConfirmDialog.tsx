import { useEffect, useId, useState, type FormEvent, type ReactNode } from "react";
import { Field, Text } from "@astryxdesign/core";
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
import { Switch } from "@multica/ui/components/ui/switch";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { getApiErrorMessage } from "../api/client";
import { t } from "../i18n";

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
  showExportFirst?: boolean;
  ownersNotice?: string;
  onConfirm: (values: {
    reason: string;
    confirm: string;
    export_first?: boolean;
  }) => Promise<void> | void;
  children?: ReactNode;
}

const DEFAULT_OWNERS_NOTICE =
  "Workspace owners will be notified by email before deletion and receive any exported archive link (valid for 7 days).";

export function TypeToConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  expectedText,
  confirmInputLabel = t("Confirmation"),
  confirmInputPlaceholder,
  reasonLabel = t("Reason"),
  reasonPlaceholder = t("Explain why this action is being taken..."),
  caseSensitive = true,
  confirmLabel = t("Confirm"),
  confirmVariant = "destructive",
  error = null,
  isConfirmDisabled = false,
  showExportFirst = false,
  ownersNotice = t(DEFAULT_OWNERS_NOTICE),
  onConfirm,
  children,
}: TypeToConfirmDialogProps) {
  const [reason, setReason] = useState("");
  const [confirmText, setConfirmText] = useState("");
  const [exportFirst, setExportFirst] = useState(false);
  const [pending, setPending] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const reasonId = useId();
  const confirmId = useId();
  const exportFirstId = useId();
  const exportFirstLabelId = useId();

  useEffect(() => {
    if (open) {
      setReason("");
      setConfirmText("");
      setExportFirst(false);
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
        ...(showExportFirst ? { export_first: exportFirst } : {}),
      });
    } catch (err) {
      setLocalError(getApiErrorMessage(err, t("Request failed")));
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

          {showExportFirst && (
            <>
              <div className="rounded-md border border-border bg-secondary/40 p-3">
                <Text type="supporting" color="primary">
                  {ownersNotice}
                </Text>
              </div>
              <div className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
                <div className="min-w-0">
                  <label
                    id={exportFirstLabelId}
                    htmlFor={exportFirstId}
                    className="text-body font-medium text-foreground"
                  >
                    {t("Export data before deleting")}
                  </label>
                  <Text type="supporting" as="div">
                    {t("Queues a pre-deletion workspace archive before purge starts.")}
                  </Text>
                </div>
                <Switch
                  id={exportFirstId}
                  aria-labelledby={exportFirstLabelId}
                  checked={exportFirst}
                  onCheckedChange={setExportFirst}
                  disabled={pending}
                />
              </div>
            </>
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
            description={expectedText ? t("Type \"{0}\" to confirm.", expectedText) : undefined}
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
              {t("Cancel")}
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
