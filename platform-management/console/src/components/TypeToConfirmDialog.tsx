import { Card } from "@astryxdesign/core/Card";
import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { Switch } from "@astryxdesign/core/Switch";
import { Text } from "@astryxdesign/core/Text";
import { TextArea } from "@astryxdesign/core/TextArea";
import { TextInput } from "@astryxdesign/core/TextInput";
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
    <Dialog
      isOpen={open}
      onOpenChange={(next) => !pending && onOpenChange(next)}
      purpose="form"
      width={512}
      maxHeight="85vh"
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <DialogHeader title={title} subtitle={description} />

        {displayError && <Banner role="alert" status="error" title={displayError} />}

        {showExportFirst && (
          <>
            <Card padding={3} elevation="none">
              <Text type="supporting" color="primary">
                {ownersNotice}
              </Text>
            </Card>
            <Card padding={3} elevation="none">
              <Switch
                label={t("Export data before deleting")}
                description={t("Queues a pre-deletion workspace archive before purge starts.")}
                labelPosition="start"
                labelSpacing="spread"
                width="100%"
                value={exportFirst}
                onChange={(checked) => setExportFirst(checked)}
                isDisabled={pending}
              />
            </Card>
          </>
        )}

        {children}

        <TextArea
          label={reasonLabel}
          isRequired
          width="100%"
          value={reason}
          onChange={(value) => setReason(value)}
          placeholder={reasonPlaceholder}
          isDisabled={pending}
          rows={3}
        />

        <TextInput
          label={confirmInputLabel}
          description={expectedText ? t("Type \"{0}\" to confirm.", expectedText) : undefined}
          isRequired
          width="100%"
          value={confirmText}
          onChange={(value) => setConfirmText(value)}
          placeholder={confirmInputPlaceholder ?? expectedText}
          isDisabled={pending}
          autoComplete="off"
        />

        <div className="flex justify-end gap-2">
          <Button
            label={t("Cancel")}
            type="button"
            variant="secondary"
            onClick={() => onOpenChange(false)}
            isDisabled={pending}
          />
          <Button
            label={confirmLabel}
            type="submit"
            variant={confirmVariant === "destructive" ? "destructive" : "primary"}
            isDisabled={!canSubmit}
            isLoading={pending}
            aria-busy={pending || undefined}
          />
        </div>
      </form>
    </Dialog>
  );
}
