import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
import { RadioList, RadioListItem } from "@astryxdesign/core/RadioList";
import { Section } from "@astryxdesign/core/Section";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Alert, AlertDescription, AlertTitle } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@multica/ui/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { AlertCircle, AlertTriangle, Ban, Building2, Info, LogOut, RotateCcw, Shield, Trash2 } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { TypeToConfirmDialog } from "../components/TypeToConfirmDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { getApiErrorMessage } from "../api/client";
import {
  useMe,
  useUser,
  useChangeUserStatus,
  useRevokeUserSessions,
  useResetUserMfa,
  useAccountDeletePreview,
  useDeleteAccountNow,
  type DeleteAccountResolution,
} from "../api/hooks";
import type { Membership, StaffRole } from "../components/types";

interface UserDetailProps {
  userId: string;
  onBack: () => void;
  userRole?: StaffRole;
}

interface WorkspaceResolutionChoice {
  action?: "transfer" | "delete";
  successor_user_id?: string;
}

export function UserDetail({ userId, onBack, userRole }: UserDetailProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;

  const {
    data: user,
    isLoading,
    isError,
    error,
    refetch,
  } = useUser(userId);

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const {
    data: preview,
    isLoading: previewLoading,
    isError: previewError,
    error: previewErrorObj,
    refetch: refetchPreview,
  } = useAccountDeletePreview(userId, deleteDialogOpen);

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
  const [deleteNotice, setDeleteNotice] = useState<string | null>(null);

  if (isLoading) {
    return <LoadingState message="Loading user details..." />;
  }

  if (isError || !user) {
    return (
      <ErrorState
        title="Failed to load user"
        message={
          error instanceof Error
            ? error.message
            : "The user could not be found."
        }
        onRetry={refetch}
      />
    );
  }

  const accountStatus = user.account?.status || user.status;
  const isSuspended = accountStatus === "suspended";
  const canControlAccount =
    !effectiveRole || effectiveRole === "system_admin" || effectiveRole === "support";
  const canDeleteAccount =
    effectiveRole === "system_admin" && accountStatus !== "deleted";

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
    confirm,
  }: {
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
          successor_user_id: choice.successor_user_id,
        };
      }
      return {
        workspace_id: ws.workspace_id,
        action: "delete",
      };
    });
    try {
      const res = await deleteAccountNow.mutateAsync({
        userId,
        reason,
        confirm,
        resolutions: payloadResolutions,
      });
      setDeleteDialogOpen(false);
      setDeleteNotice(`Immediate deletion requested (${res.request_id}).`);
    } catch (err: unknown) {
      setDeleteError(getApiErrorMessage(err, "Failed to start account deletion"));
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
        reason,
      });
      setStatusDialogOpen(false);
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : "Failed to change account status";
      setStatusError(msg);
    }
  };

  const handleConfirmRevokeSessions = async (reason: string) => {
    setRevokeError(null);
    try {
      await revokeSessions.mutateAsync({
        userId,
        reason,
      });
      setRevokeDialogOpen(false);
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : "Failed to revoke sessions";
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
        err instanceof Error ? err.message : "Failed to reset two-step sign-in",
      );
    }
  };

  return (
    <div className="space-y-8">
      <PageHeader
        title={user.name || user.email}
        badge={<StatusBadge status={accountStatus} />}
        meta={
          <span className="font-mono">
            {user.email} &bull; ID: {user.id}
          </span>
        }
        backAction={{ label: "Back to users", onClick: onBack }}
        actions={
          <>
            {canControlAccount && (
              <>
                {isSuspended ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleOpenStatusDialog("active")}
                    className="gap-1.5"
                  >
                    <RotateCcw className="size-4" aria-hidden="true" />
                    <span>Reinstate account</span>
                  </Button>
                ) : (
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => handleOpenStatusDialog("suspended")}
                    className="gap-1.5"
                  >
                    <Ban className="size-4" aria-hidden="true" />
                    <span>Suspend account</span>
                  </Button>
                )}

                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setRevokeError(null);
                    setRevokeDialogOpen(true);
                  }}
                  className="gap-1.5"
                >
                  <LogOut className="size-4" aria-hidden="true" />
                  <span>Sign out everywhere</span>
                </Button>

                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setMfaError(null);
                    setMfaDialogOpen(true);
                  }}
                  className="gap-1.5"
                >
                  <Shield className="size-4" aria-hidden="true" />
                  <span>Reset two-step sign-in</span>
                </Button>
              </>
            )}

            {canDeleteAccount && (
              <Button
                variant="destructive"
                size="sm"
                onClick={handleOpenDeleteDialog}
                className="gap-1.5"
              >
                <Trash2 className="size-4" aria-hidden="true" />
                <span>Delete account</span>
              </Button>
            )}
          </>
        }
      />

      {deleteNotice && (
        <Alert role="status">
          <Info className="size-4" aria-hidden="true" />
          <AlertDescription>{deleteNotice}</AlertDescription>
        </Alert>
      )}

      {(isSuspended || user.account?.reason) && (
        <Alert
          variant={isSuspended ? "destructive" : "default"}
          className={
            isSuspended
              ? "border-destructive/30 bg-destructive/5"
              : "border-surface-border bg-surface"
          }
        >
          <AlertTriangle className="size-4 shrink-0" aria-hidden="true" />
          <AlertTitle className="text-label font-semibold">
            {isSuspended ? "Account suspended" : "Status Reason"}
          </AlertTitle>
          {user.account?.reason && (
            <AlertDescription className="text-body">
              {user.account.reason}
            </AlertDescription>
          )}
        </Alert>
      )}

      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>Account Record</Heading>
        <MetadataList columns={3}>
          <MetadataListItem label="Account Status">
            <StatusBadge status={accountStatus} />
          </MetadataListItem>
          <MetadataListItem label="Registered" tabular>
            {user.created_at ? new Date(user.created_at).toLocaleString() : "-"}
          </MetadataListItem>
          {user.account?.changed_at && (
            <MetadataListItem label="Status Changed At" tabular>
              {new Date(user.account.changed_at).toLocaleString()}
            </MetadataListItem>
          )}
          {user.account?.sessions_valid_after && (
            <MetadataListItem label="Sessions Valid After" mono tabular>
              {new Date(user.account.sessions_valid_after).toLocaleString()}
            </MetadataListItem>
          )}
        </MetadataList>
      </Section>

      <Divider />

      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>Workspace Memberships</Heading>
        {!user.memberships || user.memberships.length === 0 ? (
          <EmptyState
            icon={<Building2 className="size-5" aria-hidden="true" />}
            title="No memberships"
            description="This user does not belong to any workspaces."
            isCompact
          />
        ) : (
          <Card padding={0} className="overflow-x-auto">
            <Table density="balanced" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>Workspace Name</TableHeaderCell>
                  <TableHeaderCell>Role</TableHeaderCell>
                  <TableHeaderCell>Workspace ID</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {user.memberships.map((membership: Membership) => (
                  <TableRow key={membership.workspace_id}>
                    <TableCell>
                      <Text type="body" weight="medium">
                        {membership.name}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={membership.role} />
                    </TableCell>
                    <TableCell>
                      <Text type="code" color="secondary">
                        {membership.workspace_id}
                      </Text>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>
        )}
      </Section>

      <ConfirmReasonDialog
        open={statusDialogOpen}
        onOpenChange={setStatusDialogOpen}
        title={
          statusDialogTarget === "suspended"
            ? "Suspend user account"
            : "Reinstate user account"
        }
        description={
          statusDialogTarget === "suspended"
            ? "Suspending this account will block the user from accessing Multica."
            : "Reinstating this account will allow the user to sign in again."
        }
        confirmLabel={
          statusDialogTarget === "suspended"
            ? "Suspend account"
            : "Reinstate account"
        }
        confirmVariant={statusDialogTarget === "suspended" ? "destructive" : "default"}
        isPending={changeStatus.isPending}
        error={statusError}
        onConfirm={handleConfirmStatusChange}
      />

      <ConfirmReasonDialog
        open={revokeDialogOpen}
        onOpenChange={setRevokeDialogOpen}
        title="Sign out everywhere"
        description="Revoke all active sessions for this user. Every session issued before now will stop working."
        confirmLabel="Revoke all sessions"
        confirmVariant="destructive"
        isPending={revokeSessions.isPending}
        error={revokeError}
        onConfirm={handleConfirmRevokeSessions}
      />

      <Dialog open={mfaDialogOpen} onOpenChange={setMfaDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reset two-step sign-in</DialogTitle>
            <DialogDescription>
              The user signs in without a second step until they set it up again. They are not signed out, and they are told by email.
            </DialogDescription>
          </DialogHeader>
          {mfaError && (
            <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              <AlertDescription className="text-caption">{mfaError}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>
              Cancel
            </DialogClose>
            <Button
              type="button"
              variant="destructive"
              disabled={resetMfa.isPending}
              aria-busy={resetMfa.isPending}
              onClick={handleConfirmResetMfa}
            >
              {resetMfa.isPending && <Spinner className="size-3.5" />}
              Reset and email the user
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {canDeleteAccount && (
        <TypeToConfirmDialog
          open={deleteDialogOpen}
          onOpenChange={(open) => {
            setDeleteDialogOpen(open);
            if (!open) setDeleteError(null);
          }}
          title="Delete account"
          description="Immediate deletion permanently removes this account and queues ownership resolution for blocking workspaces."
          expectedText={user.email}
          confirmInputLabel="Account email"
          caseSensitive={false}
          confirmLabel="Delete account"
          confirmVariant="destructive"
          error={deleteError}
          isConfirmDisabled={hasUnresolvedWorkspaces}
          onConfirm={handleConfirmDeleteAccount}
        >
          {previewLoading && <LoadingState message="Checking workspace ownership..." />}
          {previewError && (
            <ErrorState
              title="Failed to load ownership preview"
              message={getApiErrorMessage(previewErrorObj, "Could not check workspace ownership.")}
              onRetry={refetchPreview}
            />
          )}
          {preview && (
            <div className="space-y-4">
              {preview.deleted_with.length > 0 && (
                <div className="space-y-2">
                  <Heading level={3}>Deleted with account</Heading>
                  <div className="space-y-1.5">
                    {preview.deleted_with.map((ws) => (
                      <Card key={ws.workspace_id} padding={3}>
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-mono text-body font-medium text-foreground">
                            {ws.slug || ws.workspace_id}
                          </span>
                          <Text type="supporting">
                            Sole member ({ws.member_count} member)
                          </Text>
                        </div>
                      </Card>
                    ))}
                  </div>
                </div>
              )}

              {preview.need_choice.length > 0 && (
                <div className="space-y-3">
                  <Heading level={3}>Blocking sole-owner workspaces</Heading>
                  {preview.need_choice.map((ws) => {
                    const wsLabel = ws.slug || ws.workspace_id;
                    const choice = resolutions[ws.workspace_id];
                    const eligibleSuccessors = ws.successors.filter(
                      (s) => s.status === "active" && (s.role === "admin" || s.role === "member"),
                    );
                    const successorItems = eligibleSuccessors.map((s) => ({
                      value: s.user_id,
                      label: s.name ? `${s.name} (${s.email})` : s.email,
                    }));

                    return (
                      <Card key={ws.workspace_id} padding={3} className="space-y-3">
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-mono text-body font-medium text-foreground">
                            {wsLabel}
                          </span>
                          <Text type="supporting">
                            {ws.member_count} members &bull; {ws.owner_count} owner
                          </Text>
                        </div>

                        <RadioList
                          label={`Resolution for ${wsLabel}`}
                          value={choice?.action ?? ""}
                          onChange={(val) =>
                            setResolutions((prev) => ({
                              ...prev,
                              [ws.workspace_id]: {
                                action: val as "transfer" | "delete",
                                successor_user_id:
                                  val === "transfer" ? prev[ws.workspace_id]?.successor_user_id : undefined,
                              },
                            }))
                          }
                        >
                          <RadioListItem value="transfer" label="Transfer ownership" />
                          <RadioListItem value="delete" label="Delete workspace now" />
                        </RadioList>

                        {choice?.action === "transfer" && (
                          eligibleSuccessors.length > 0 ? (
                            <Select
                              items={successorItems}
                              value={choice.successor_user_id ?? ""}
                              onValueChange={(val) =>
                                setResolutions((prev) => ({
                                  ...prev,
                                  [ws.workspace_id]: {
                                    action: "transfer",
                                    successor_user_id: val ?? undefined,
                                  },
                                }))
                              }
                            >
                              <SelectTrigger
                                aria-label={`Successor for ${wsLabel}`}
                                className="w-full"
                              >
                                <SelectValue placeholder="Select successor">
                                  {successorItems.find((item) => item.value === choice.successor_user_id)?.label}
                                </SelectValue>
                              </SelectTrigger>
                              <SelectContent>
                                {eligibleSuccessors.map((s) => (
                                  <SelectItem key={s.user_id} value={s.user_id}>
                                    {s.name ? `${s.name} (${s.email})` : s.email} ({s.role})
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          ) : (
                            <Text type="supporting">
                              No eligible admin or member successors were returned for this workspace. Choose &ldquo;Delete workspace now&rdquo; or transfer ownership first.
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
    </div>
  );
}
