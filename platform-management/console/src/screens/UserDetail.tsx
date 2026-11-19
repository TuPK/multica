import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
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
import { Spinner } from "@multica/ui/components/ui/spinner";
import { AlertCircle, AlertTriangle, Ban, Building2, LogOut, RotateCcw, Shield } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import {
  useUser,
  useChangeUserStatus,
  useRevokeUserSessions,
  useResetUserMfa,
} from "../api/hooks";
import type { Membership } from "../components/types";

interface UserDetailProps {
  userId: string;
  onBack: () => void;
}

export function UserDetail({ userId, onBack }: UserDetailProps) {
  const {
    data: user,
    isLoading,
    isError,
    error,
    refetch,
  } = useUser(userId);

  const changeStatus = useChangeUserStatus();
  const revokeSessions = useRevokeUserSessions();
  const resetMfa = useResetUserMfa();

  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [statusDialogTarget, setStatusDialogTarget] = useState<"suspended" | "active">("suspended");
  const [statusError, setStatusError] = useState<string | null>(null);

  const [revokeDialogOpen, setRevokeDialogOpen] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const [mfaDialogOpen, setMfaDialogOpen] = useState(false);
  const [mfaError, setMfaError] = useState<string | null>(null);

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
        }
      />

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
    </div>
  );
}
