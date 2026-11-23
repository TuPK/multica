import { useState } from "react";
import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { Card } from "@astryxdesign/core/Card";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { Heading } from "@astryxdesign/core/Heading";
import { RadioList, RadioListItem } from "@astryxdesign/core/RadioList";
import { Selector } from "@astryxdesign/core/Selector";
import { Text } from "@astryxdesign/core/Text";
import { RowActions, type RowAction } from "../components/RowActions";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { TypeToConfirmDialog } from "../components/TypeToConfirmDialog";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { getApiErrorMessage } from "../api/client";
import {
  useAccountDeletePreview,
  useChangeUserStatus,
  useRevokeUserSessions,
  useResetUserMfa,
  useDeleteAccountNow,
  type DeleteAccountResolution } from "../api/hooks";
import { t } from "../i18n";

interface WorkspaceResolutionChoice {
  action?: "transfer" | "delete";
  successor_user_id?: string;
}

interface UserAccountMenuProps {
  userId: string;
  email: string;
  accountStatus: string;
  canControlAccount: boolean;
  canDeleteAccount: boolean;
}

// Account-level actions for one user, shown in the Users table's row menu.
// Each row mounts its own dialogs, which stay closed until an action opens them.
export function UserAccountMenu({
  userId,
  email,
  accountStatus,
  canControlAccount,
  canDeleteAccount }: UserAccountMenuProps) {
  const isSuspended = accountStatus === "suspended";

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const {
    data: preview,
    isLoading: previewLoading,
    isError: previewError,
    error: previewErrorObj,
    refetch: refetchPreview } = useAccountDeletePreview(userId, deleteDialogOpen);

  const changeStatus = useChangeUserStatus();
  const revokeSessions = useRevokeUserSessions();
  const resetMfa = useResetUserMfa();
  const deleteAccountNow = useDeleteAccountNow();
  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [statusDialogTarget, setStatusDialogTarget] = useState<"suspended" | "active">("suspended");
  const [statusError, setStatusError] = useState<string | null>(null);

  const [revokeDialogOpen, setRevokeDialogOpen] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const [mfaDialogOpen, setMfaDialogOpen] = useState(false);
  const [mfaError, setMfaError] = useState<string | null>(null);

  const [resolutions, setResolutions] = useState<Record<string, WorkspaceResolutionChoice>>({});
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const handleOpenDeleteDialog = () => {
    setResolutions({});
    setDeleteError(null);
    setDeleteDialogOpen(true);
  };

  const hasUnresolvedWorkspaces =
    previewLoading ||
    previewError ||
    !preview ||
    preview.need_choice.some((ws) => {
      const choice = resolutions[ws.workspace_id];
      if (choice?.action === "delete") return false;
      if (choice?.action === "transfer" && choice.successor_user_id) return false;
      return true;
    });

  const handleConfirmDeleteAccount = async ({
    reason,
    confirm }: {
    reason: string;
    confirm: string;
  }) => {
    if (!preview) return;
    setDeleteError(null);
    const payloadResolutions: DeleteAccountResolution[] = preview.need_choice.map((ws) => {
      const choice = resolutions[ws.workspace_id];
      if (choice?.action === "transfer" && choice.successor_user_id) {
        return {
          workspace_id: ws.workspace_id,
          action: "transfer",
          successor_user_id: choice.successor_user_id };
      }
      return {
        workspace_id: ws.workspace_id,
        action: "delete" };
    });
    try {
      await deleteAccountNow.mutateAsync({
        userId,
        reason,
        confirm,
        resolutions: payloadResolutions });
      setDeleteDialogOpen(false);
    } catch (err: unknown) {
      setDeleteError(getApiErrorMessage(err, t("Failed to start account deletion")));
    }
  };

  const handleOpenStatusDialog = (target: "suspended" | "active") => {
    setStatusDialogTarget(target);
    setStatusError(null);
    setStatusDialogOpen(true);
  };

  const handleConfirmStatusChange = async (reason: string) => {
    setStatusError(null);
    try {
      await changeStatus.mutateAsync({
        userId,
        status: statusDialogTarget,
        reason });
      setStatusDialogOpen(false);
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : t("Failed to change account status");
      setStatusError(msg);
    }
  };

  const handleConfirmRevokeSessions = async (reason: string) => {
    setRevokeError(null);
    try {
      await revokeSessions.mutateAsync({
        userId,
        reason });
      setRevokeDialogOpen(false);
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : t("Failed to revoke sessions");
      setRevokeError(msg);
    }
  };

  const handleConfirmResetMfa = async () => {
    setMfaError(null);
    try {
      await resetMfa.mutateAsync({ userId });
      setMfaDialogOpen(false);
    } catch (err: unknown) {
      setMfaError(
        err instanceof Error ? err.message : t("Failed to reset two-step sign-in"),
      );
    }
  };

  const actions: RowAction[] = [
    ...(canControlAccount
      ? [
          isSuspended
            ? { label: t("Reinstate account"), onSelect: () => handleOpenStatusDialog("active") }
            : {
                label: t("Suspend account"),
                onSelect: () => handleOpenStatusDialog("suspended"),
                destructive: true },
          {
            label: t("Sign out everywhere"),
            onSelect: () => {
              setRevokeError(null);
              setRevokeDialogOpen(true);
            } },
          {
            label: t("Reset two-step sign-in"),
            onSelect: () => {
              setMfaError(null);
              setMfaDialogOpen(true);
            } },
        ]
      : []),
    ...(canDeleteAccount
      ? [{ label: t("Delete account"), onSelect: handleOpenDeleteDialog, destructive: true }]
      : []),
  ];

  return (
    <>
      <RowActions label={email} actions={actions} />
      <ConfirmReasonDialog
        open={statusDialogOpen}
        onOpenChange={setStatusDialogOpen}
        title={
          statusDialogTarget === "suspended"
            ? t("Suspend user account")
            : t("Reinstate user account")
        }
        description={
          statusDialogTarget === "suspended"
            ? t("Suspending this account will block the user from accessing Delegora.")
            : t("Reinstating this account will allow the user to sign in again.")
        }
        confirmLabel={
          statusDialogTarget === "suspended"
            ? t("Suspend account")
            : t("Reinstate account")
        }
        confirmVariant={statusDialogTarget === "suspended" ? "destructive" : "default"}
        isPending={changeStatus.isPending}
        error={statusError}
        onConfirm={handleConfirmStatusChange}
      />

      <ConfirmReasonDialog
        open={revokeDialogOpen}
        onOpenChange={setRevokeDialogOpen}
        title={t("Sign out everywhere")}
        description={t("Revoke all active sessions for this user. Every session issued before now will stop working.")}
        confirmLabel={t("Revoke all sessions")}
        confirmVariant="destructive"
        isPending={revokeSessions.isPending}
        error={revokeError}
        onConfirm={handleConfirmRevokeSessions}
      />

      <Dialog
        isOpen={mfaDialogOpen}
        onOpenChange={setMfaDialogOpen}
        purpose="info"
      >

          <DialogHeader title={t("Reset two-step sign-in")} subtitle={t("The user signs in without a second step until they set it up again. They are not signed out, and they are told by email.")} />
          {mfaError && (
            <Banner
              status="error"
              role="alert"
              title={mfaError}
            />
          )}
          <div className="flex justify-end gap-2">
            <Button
        type="button"
        label={t("Cancel")}
        variant="secondary"
        onClick={() => setMfaDialogOpen(false)}
      />
            <Button
              type="button"
              label={t("Reset and email the user")}
              variant="destructive"
              isLoading={resetMfa.isPending}
              aria-busy={resetMfa.isPending}
              onClick={handleConfirmResetMfa}
              isDisabled={resetMfa.isPending}
            />
          </div>

      </Dialog>

      {canDeleteAccount && (
        <TypeToConfirmDialog
          open={deleteDialogOpen}
          onOpenChange={(open) => {
            setDeleteDialogOpen(open);
            if (!open) setDeleteError(null);
          }}
          title={t("Delete account")}
          description={t("Immediate deletion permanently removes this account and queues ownership resolution for blocking workspaces.")}
          expectedText={email}
          confirmInputLabel={t("Account email")}
          caseSensitive={false}
          confirmLabel={t("Delete account")}
          confirmVariant="destructive"
          error={deleteError}
          isConfirmDisabled={hasUnresolvedWorkspaces}
          onConfirm={handleConfirmDeleteAccount}
        >
          {previewLoading && <LoadingState message={t("Checking workspace ownership...")} />}
          {previewError && (
            <ErrorState
              title={t("Failed to load ownership preview")}
              message={getApiErrorMessage(previewErrorObj, t("Could not check workspace ownership."))}
              onRetry={refetchPreview}
            />
          )}
          {preview && (
            <div className="space-y-4">
              {preview.deleted_with.length > 0 && (
                <div className="space-y-2">
                  <Heading level={3}>{t("Deleted with account")}</Heading>
                  <div className="space-y-1.5">
                    {preview.deleted_with.map((ws) => (
                      <Card key={ws.workspace_id} padding={3}>
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-mono text-body font-medium text-foreground">
                            {ws.slug || ws.workspace_id}
                          </span>
                          <Text type="supporting">
                            {t("Sole member (")}{ws.member_count} {t("member)")}
                          </Text>
                        </div>
                      </Card>
                    ))}
                  </div>
                </div>
              )}

              {preview.need_choice.length > 0 && (
                <div className="space-y-3">
                  <Heading level={3}>{t("Blocking sole-owner workspaces")}</Heading>
                  {preview.need_choice.map((ws) => {
                    const wsLabel = ws.slug || ws.workspace_id;
                    const choice = resolutions[ws.workspace_id];
                    const eligibleSuccessors = ws.successors.filter(
                      (s) => s.status === "active" && (s.role === "admin" || s.role === "member"),
                    );
                    return (
                      <Card key={ws.workspace_id} padding={3} className="space-y-3">
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-mono text-body font-medium text-foreground">
                            {wsLabel}
                          </span>
                          <Text type="supporting">
                            {ws.member_count} {t("members •")} {ws.owner_count} {t("owner")}
                          </Text>
                        </div>

                        <RadioList
                          label={t("Resolution for {0}", wsLabel)}
                          value={choice?.action ?? ""}
                          onChange={(val) =>
                            setResolutions((prev) => ({
                              ...prev,
                              [ws.workspace_id]: {
                                action: val as "transfer" | "delete",
                                successor_user_id:
                                  val === "transfer" ? prev[ws.workspace_id]?.successor_user_id : undefined } }))
                          }
                        >
                          <RadioListItem value="transfer" label={t("Transfer ownership")} />
                          <RadioListItem value="delete" label={t("Delete workspace now")} />
                        </RadioList>

                        {choice?.action === "transfer" && (
                          eligibleSuccessors.length > 0 ? (
                            <Selector
                              label={t("Successor for {0}", wsLabel)}
                              isLabelHidden
                              width="100%"
                              placeholder={t("Select successor")}
                              options={eligibleSuccessors.map((s) => ({
                                value: s.user_id,
                                label: `${s.name ? `${s.name} (${s.email})` : s.email} (${s.role})`,
                              }))}
                              value={choice.successor_user_id ?? ""}
                              onChange={(val) =>
                                setResolutions((prev) => ({
                                  ...prev,
                                  [ws.workspace_id]: {
                                    action: "transfer",
                                    successor_user_id: val ?? undefined } }))
                              }
                            />
                          ) : (
                            <Text type="supporting">
                              {t("No eligible admin or member successors were returned for this workspace. Choose “Delete workspace now” or transfer ownership first.")}
                            </Text>
                          )
                        )}
                      </Card>
                    );
                  })}
                </div>
              )}
            </div>
          )}
        </TypeToConfirmDialog>
      )}
    </>
  );
}
