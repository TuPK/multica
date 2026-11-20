import { useState } from "react";
import { Ban, CheckCircle2, Trash2 } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { RowActions, type RowAction } from "../components/RowActions";
import { TypeToConfirmDialog } from "../components/TypeToConfirmDialog";
import { getApiErrorMessage } from "../api/client";
import { useSetRuntimeStatus, useUnregisterRuntime } from "../api/hooks";
import { t } from "../i18n";

interface RuntimeAccountMenuProps {
  runtimeId: string;
  runtimeName: string;
  disabled: boolean;
  canControlRuntime: boolean;
}

// Enable/disable and unregister for one runtime, shown in its row of the
// runtimes table. Success needs no notice: the list refreshes once the
// mutation invalidates its query.
export function RuntimeAccountMenu({
  runtimeId,
  runtimeName,
  disabled,
  canControlRuntime,
}: RuntimeAccountMenuProps) {
  const setRuntimeStatus = useSetRuntimeStatus();
  const unregisterRuntime = useUnregisterRuntime();

  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [statusError, setStatusError] = useState<string | null>(null);
  const [unregisterDialogOpen, setUnregisterDialogOpen] = useState(false);
  const [unregisterError, setUnregisterError] = useState<string | null>(null);

  const nextStatus: "disabled" | "enabled" = disabled ? "enabled" : "disabled";

  const handleConfirmStatusChange = async (reason: string) => {
    setStatusError(null);
    try {
      await setRuntimeStatus.mutateAsync({ runtimeId, status: nextStatus, reason });
      setStatusDialogOpen(false);
    } catch (err) {
      setStatusError(getApiErrorMessage(err, t("Failed to update runtime status")));
    }
  };

  const handleConfirmUnregister = async ({ reason, confirm }: { reason: string; confirm: string }) => {
    setUnregisterError(null);
    try {
      await unregisterRuntime.mutateAsync({ runtimeId, reason, confirm });
      setUnregisterDialogOpen(false);
    } catch (err) {
      setUnregisterError(getApiErrorMessage(err, t("Failed to unregister runtime")));
    }
  };

  const actions: RowAction[] = canControlRuntime
    ? [
        {
          label: disabled ? t("Enable runtime") : t("Disable runtime"),
          icon: disabled ? CheckCircle2 : Ban,
          destructive: !disabled,
          onSelect: () => {
            setStatusError(null);
            setStatusDialogOpen(true);
          },
        },
        {
          label: t("Unregister runtime"),
          icon: Trash2,
          destructive: true,
          onSelect: () => {
            setUnregisterError(null);
            setUnregisterDialogOpen(true);
          },
        },
      ]
    : [];

  return (
    <>
      <RowActions label={t("runtime {0}", runtimeName)} actions={actions} />

      {canControlRuntime && (
        <>
          <ConfirmReasonDialog
            open={statusDialogOpen}
            onOpenChange={(open) => {
              if (!setRuntimeStatus.isPending) {
                setStatusDialogOpen(open);
                if (!open) setStatusError(null);
              }
            }}
            title={disabled ? t("Enable runtime") : t("Disable runtime")}
            description={
              disabled
                ? t("Re-enabling allows this daemon to reconnect and claim queued tasks.")
                : t("Disabling rejects daemon routes and blocks task claims until re-enabled.")
            }
            confirmLabel={disabled ? t("Enable runtime") : t("Disable runtime")}
            confirmVariant={disabled ? "default" : "destructive"}
            isPending={setRuntimeStatus.isPending}
            error={statusError}
            onConfirm={handleConfirmStatusChange}
          />

          <TypeToConfirmDialog
            open={unregisterDialogOpen}
            onOpenChange={(open) => {
              setUnregisterDialogOpen(open);
              if (!open) setUnregisterError(null);
            }}
            title={t("Unregister runtime")}
            description={t("Unregistering queues permanent removal of this runtime and cannot be undone.")}
            expectedText={runtimeName || runtimeId}
            confirmInputLabel={t("Runtime name")}
            confirmLabel={t("Unregister runtime")}
            confirmVariant="destructive"
            error={unregisterError}
            onConfirm={handleConfirmUnregister}
          />
        </>
      )}
    </>
  );
}
