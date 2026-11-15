import { useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@multica/ui/components/ui/card";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import { ArrowLeft, Ban, RotateCcw, LogOut } from "@multica/ui/icons";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import {
  useUser,
  useChangeUserStatus,
  useRevokeUserSessions,
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

  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [statusDialogTarget, setStatusDialogTarget] = useState<"suspended" | "active">("suspended");
  const [statusError, setStatusError] = useState<string | null>(null);

  const [revokeDialogOpen, setRevokeDialogOpen] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

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

  const isSuspended = user.account?.status === "suspended" || user.status === "suspended";

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

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-4">
          <Button
            variant="ghost"
            size="sm"
            onClick={onBack}
            className="gap-1 text-muted-foreground"
          >
            <ArrowLeft className="size-4" />
            <span>Back to users</span>
          </Button>
          <div>
            <h1 className="font-heading text-title-lg font-bold">
              {user.name || user.email}
            </h1>
            <p className="font-mono text-caption text-muted-foreground">
              {user.email} &bull; ID: {user.id}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {isSuspended ? (
            <Button
              variant="outline"
              onClick={() => handleOpenStatusDialog("active")}
              className="gap-1.5"
            >
              <RotateCcw className="size-4" />
              <span>Reinstate account</span>
            </Button>
          ) : (
            <Button
              variant="destructive"
              onClick={() => handleOpenStatusDialog("suspended")}
              className="gap-1.5"
            >
              <Ban className="size-4" />
              <span>Suspend account</span>
            </Button>
          )}

          <Button
            variant="outline"
            onClick={() => {
              setRevokeError(null);
              setRevokeDialogOpen(true);
            }}
            className="gap-1.5"
          >
            <LogOut className="size-4" />
            <span>Sign out everywhere</span>
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
        <Card className="md:col-span-1">
          <CardHeader>
            <CardTitle>Account Record</CardTitle>
            <CardDescription>
              User authentication and security state.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <span className="block text-caption text-muted-foreground">Account Status</span>
              <div className="mt-1">
                <StatusBadge status={user.account?.status || user.status} />
              </div>
            </div>

            <div>
              <span className="block text-caption text-muted-foreground">Registered</span>
              <span className="mt-1 block text-body font-medium">
                {user.created_at ? new Date(user.created_at).toLocaleString() : "-"}
              </span>
            </div>

            {user.account?.changed_at && (
              <div>
                <span className="block text-caption text-muted-foreground">Status Changed At</span>
                <span className="mt-1 block text-body">
                  {new Date(user.account.changed_at).toLocaleString()}
                </span>
              </div>
            )}

            {user.account?.reason && (
              <div className="rounded-lg border border-surface-border bg-muted/40 p-3">
                <span className="block text-caption font-medium text-foreground">
                  Status Reason:
                </span>
                <p className="mt-0.5 text-body text-muted-foreground">
                  {user.account.reason}
                </p>
              </div>
            )}

            {user.account?.sessions_valid_after && (
              <div>
                <span className="block text-caption text-muted-foreground">Sessions Valid After</span>
                <span className="mt-1 block font-mono text-caption text-muted-foreground">
                  {new Date(user.account.sessions_valid_after).toLocaleString()}
                </span>
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="md:col-span-2">
          <CardHeader>
            <CardTitle>Workspace Memberships</CardTitle>
            <CardDescription>
              Workspaces this user belongs to.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {!user.memberships || user.memberships.length === 0 ? (
              <EmptyState
                title="No memberships"
                description="This user does not belong to any workspaces."
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Workspace Name</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead>Workspace ID</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {user.memberships.map((membership: Membership) => (
                    <TableRow key={membership.workspace_id}>
                      <TableCell className="font-medium">
                        {membership.name}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={membership.role} />
                      </TableCell>
                      <TableCell className="font-mono text-caption text-muted-foreground">
                        {membership.workspace_id}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

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
    </div>
  );
}
