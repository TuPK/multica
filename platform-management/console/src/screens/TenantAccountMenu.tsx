import { useState } from "react";
import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { Selector } from "@astryxdesign/core/Selector";
import { TextArea } from "@astryxdesign/core/TextArea";
import { Lock, Trash2 } from "@multica/ui/icons";
import { RowActions, type RowAction } from "../components/RowActions";
import { TypeToConfirmDialog } from "../components/TypeToConfirmDialog";
import { getApiErrorMessage } from "../api/client";
import { useDeleteTenantNow, useSetTenantStatus } from "../api/hooks";
import { t } from "../i18n";

interface TenantAccountMenuProps {
  tenantId: string;
  tenantName: string;
  tenantStatus: string;
  canManageTenantStatus: boolean;
  canDeleteTenant: boolean;
}

// The lock, unlock and delete actions of one tenant, shown in its row of the
// tenants table. Success needs no notice: the row's status or the tenant
// list itself changes once the mutation invalidates the queries.
export function TenantAccountMenu({
  tenantId,
  tenantName,
  tenantStatus,
  canManageTenantStatus,
  canDeleteTenant,
}: TenantAccountMenuProps) {
  const setTenantStatus = useSetTenantStatus();
  const deleteTenantNow = useDeleteTenantNow();

  const [statusDialogMode, setStatusDialogMode] = useState<"lock" | "unlock" | null>(null);
  const [lockTargetStatus, setLockTargetStatus] = useState<"suspended" | "read_only">("suspended");
  const [statusReason, setStatusReason] = useState("");
  const [statusDialogError, setStatusDialogError] = useState<string | null>(null);

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const openStatusDialog = (mode: "lock" | "unlock") => {
    setStatusDialogMode(mode);
    setLockTargetStatus("suspended");
    setStatusReason("");
    setStatusDialogError(null);
  };

  const handleSubmitTenantStatus = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!statusDialogMode || setTenantStatus.isPending) return;
    const trimmedReason = statusReason.trim();
    if (!trimmedReason) {
      setStatusDialogError(t("A reason is required."));
      return;
    }

    const nextStatus = statusDialogMode === "unlock" ? "active" : lockTargetStatus;
    setStatusDialogError(null);
    try {
      await setTenantStatus.mutateAsync({
        tenantId,
        status: nextStatus,
        reason: trimmedReason,
      });
      setStatusDialogMode(null);
    } catch (err) {
      setStatusDialogError(getApiErrorMessage(err, t("Failed to update tenant status")));
    }
  };

  const handleConfirmDeleteTenant = async ({
    reason,
    confirm,
    export_first,
  }: {
    reason: string;
    confirm: string;
    export_first?: boolean;
  }) => {
    setDeleteError(null);
    try {
      await deleteTenantNow.mutateAsync({
        tenantId,
        reason,
        confirm,
        ...(export_first !== undefined ? { export_first } : {}),
      });
      setDeleteDialogOpen(false);
    } catch (err) {
      setDeleteError(getApiErrorMessage(err, t("Failed to start tenant deletion")));
    }
  };

  const actions: RowAction[] = [];
  if (canManageTenantStatus && tenantStatus !== "suspended") {
    actions.push({
      label: t("Lock tenant"),
      icon: Lock,
      onSelect: () => openStatusDialog("lock"),
    });
  }
  if (canManageTenantStatus && (tenantStatus === "suspended" || tenantStatus === "read_only")) {
    actions.push({
      label: t("Unlock tenant"),
      onSelect: () => openStatusDialog("unlock"),
    });
  }
  if (canDeleteTenant) {
    actions.push({
      label: t("Delete tenant"),
      icon: Trash2,
      destructive: true,
      onSelect: () => {
        setDeleteError(null);
        setDeleteDialogOpen(true);
      },
    });
  }

  return (
    <>
      <RowActions label={t("tenant {0}", tenantName)} actions={actions} />

      {canManageTenantStatus && (
        <Dialog
          isOpen={statusDialogMode !== null}
          onOpenChange={(open) => {
            if (!open && !setTenantStatus.isPending) {
              setStatusDialogMode(null);
              setStatusDialogError(null);
            }
          }}
          purpose="form"
          width={448}
        >
          <form onSubmit={handleSubmitTenantStatus} className="space-y-4">
            <DialogHeader
              title={statusDialogMode === "unlock" ? t("Unlock tenant") : t("Lock tenant")}
              subtitle={
                statusDialogMode === "unlock"
                  ? t("Restore all locked workspaces in this tenant back to active status.")
                  : t("Cascade a status lock across every active workspace belonging to this tenant.")
              }
            />

            {statusDialogError && <Banner status="error" role="alert" title={statusDialogError} />}

            {statusDialogMode === "lock" && (
              <Selector
                label={t("Target status")}
                width="100%"
                options={[
                  { value: "suspended", label: t("Suspended") },
                  { value: "read_only", label: t("Read-only") },
                ]}
                value={lockTargetStatus}
                onChange={(val) => {
                  if (val === "suspended" || val === "read_only") {
                    setLockTargetStatus(val);
                  }
                }}
              />
            )}

            <TextArea
              id={`tenant-status-reason-${tenantId}`}
              label={t("Reason")}
              value={statusReason}
              onChange={(value) => setStatusReason(value)}
              placeholder={t("Explain why this tenant is being updated...")}
              isDisabled={setTenantStatus.isPending}
              rows={3}
              width="100%"
            />

            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="secondary"
                isDisabled={setTenantStatus.isPending}
                label={t("Cancel")}
                onClick={() => {
                  if (!setTenantStatus.isPending) {
                    setStatusDialogMode(null);
                    setStatusDialogError(null);
                  }
                }}
              />
              <Button
                type="submit"
                label={statusDialogMode === "unlock" ? t("Unlock tenant") : t("Apply lock")}
                variant={statusDialogMode === "unlock" ? "primary" : "destructive"}
                isDisabled={!statusReason.trim() || setTenantStatus.isPending}
                isLoading={setTenantStatus.isPending}
                aria-busy={setTenantStatus.isPending || undefined}
              />
            </div>
          </form>
        </Dialog>
      )}

      {canDeleteTenant && (
        <TypeToConfirmDialog
          open={deleteDialogOpen}
          onOpenChange={(open) => {
            setDeleteDialogOpen(open);
            if (!open) setDeleteError(null);
          }}
          title={t("Delete tenant")}
          description={t("Immediate deletion permanently removes this tenant and all of its workspaces.")}
          expectedText={tenantName}
          confirmInputLabel={t("Tenant name")}
          confirmLabel={t("Delete tenant")}
          confirmVariant="destructive"
          showExportFirst
          error={deleteError}
          onConfirm={handleConfirmDeleteTenant}
        />
      )}
    </>
  );
}
