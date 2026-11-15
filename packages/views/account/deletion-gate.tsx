"use client";

import { useAuthStore } from "@multica/core/auth";
import { useCancelAccountDeletion } from "@multica/core/account";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Loader2 } from "@multica/ui/icons";
import { toast } from "sonner";
import { useLogout } from "../auth";
import { useLocale, useT } from "../i18n";

export function DeletionGate() {
  const { t } = useT("settings");
  const locale = useLocale();
  const user = useAuthStore((s) => s.user);
  const setUser = useAuthStore((s) => s.setUser);
  const logout = useLogout();

  const cancelAccountDeletion = useCancelAccountDeletion();
  const isPending = Boolean(cancelAccountDeletion?.isPending);

  const deletionScheduledAt = (
    user as { deletion_scheduled_at?: string | null } | null
  )?.deletion_scheduled_at;

  if (!deletionScheduledAt) {
    return null;
  }

  const dateObj = new Date(deletionScheduledAt);
  const formattedDate = Number.isNaN(dateObj.getTime())
    ? deletionScheduledAt
    : new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(dateObj);

  const handleCancel = async () => {
    if (isPending) return;
    try {
      await cancelAccountDeletion.mutateAsync();
      if (user) {
        setUser({ ...user, deletion_scheduled_at: null } as unknown as typeof user);
      }
      toast.success(t(($) => $.deletion_gate.cancel_success));
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t(($) => $.deletion_gate.cancel_failed),
      );
    }
  };

  return (
    <Dialog open disablePointerDismissal>
      <DialogContent
        showCloseButton={false}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.preventDefault();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>{t(($) => $.deletion_gate.title)}</DialogTitle>
          <DialogDescription>
            {t(($) => $.deletion_gate.description, { date: formattedDate })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={logout}
            disabled={isPending}
          >
            {t(($) => $.deletion_gate.sign_out_button)}
          </Button>
          <Button
            type="button"
            onClick={handleCancel}
            disabled={isPending}
            aria-busy={isPending || undefined}
          >
            {isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            {isPending
              ? t(($) => $.deletion_gate.canceling_button)
              : t(($) => $.deletion_gate.cancel_button)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
