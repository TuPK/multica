import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
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
import { RowActions } from "../components/RowActions";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@multica/ui/components/ui/dialog";
import { Field, FieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  Ban,
  Building2,
  Info,
  LogOut,
  RotateCcw,
  Shield,
  Trash2,
  UserMinus,
} from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { TypeToConfirmDialog } from "../components/TypeToConfirmDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { getApiErrorMessage, getMemberChangeErrorMessage } from "../api/client";
import {
  useMe,
  useUser,
  useTenantUser,
  useChangeUserStatus,
  useRevokeUserSessions,
  useResetUserMfa,
  useAccountDeletePreview,
  useDeleteAccountNow,
  useDeactivateTenantUser,
  useRemoveWorkspaceMember,
  useChangeWorkspaceMemberRole,
  useCreateQuotaRequest,
  type DeleteAccountResolution,
  type MemberRole,
  type TenantUserAllocationItem,
} from "../api/hooks";
import { MEMBER_ROLES, type AuditEvent } from "../api/schemas";
import type { Membership, StaffRole } from "../components/types";

interface UserDetailProps {
  userId: string;
  tenantId?: string;
  onBack: () => void;
  backLabel?: string;
  onSelectWorkspace?: (workspaceId: string) => void;
  userRole?: StaffRole;
}

interface WorkspaceResolutionChoice {
  action?: "transfer" | "delete";
  successor_user_id?: string;
}

const MEMBER_ROLE_ITEMS: Array<{ value: MemberRole; label: string }> = MEMBER_ROLES.map((role) => ({
  value: role,
  label: role.charAt(0).toUpperCase() + role.slice(1),
}));

const LIMIT_GATE_ITEMS: Array<{ value: string; label: string }> = [
  { value: "issue_count", label: "Issues" },
  { value: "autopilot_runs", label: "Autopilot runs" },
  { value: "agents", label: "Agents" },
  { value: "runtimes", label: "Runtimes" },
  { value: "storage_bytes", label: "Storage (bytes)" },
  { value: "api_rate_per_minute", label: "API rate (/min)" },
  { value: "audit_retention_days", label: "Audit retention (days)" },
  { value: "member_groups", label: "Member groups" },
  { value: "max_workspaces", label: "Max workspaces" },
];

function isMemberRole(role: string): role is MemberRole {
  return (MEMBER_ROLES as readonly string[]).includes(role);
}

function formatLimitsSummary(allocation: TenantUserAllocationItem): string {
  const entries = Object.entries(allocation.limits);
  if (entries.length === 0) return "-";
  return entries
    .map(([gate, limit]) => {
      const used = allocation.usage[gate];
      const limitLabel = limit === null ? "Unlimited" : limit.toLocaleString();
      return used !== undefined ? `${gate}: ${used.toLocaleString()} / ${limitLabel}` : `${gate}: ${limitLabel}`;
    })
    .join(" · ");
}

export function UserDetail({
  userId,
  tenantId,
  onBack,
  backLabel = "Back to users",
  onSelectWorkspace,
  userRole,
}: UserDetailProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;

  const {
    data: user,
    isLoading,
    isError,
    error,
    refetch,
  } = useUser(userId);

  const effectiveTenantId = tenantId ?? me?.tenant_id ?? user?.tenant_id ?? undefined;

  const { data: tenantUser } = useTenantUser(
    effectiveTenantId,
    userId,
    Boolean(effectiveTenantId),
  );

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
  const deactivateTenantUser = useDeactivateTenantUser();
  const removeWorkspaceMember = useRemoveWorkspaceMember();
  const changeWorkspaceMemberRole = useChangeWorkspaceMemberRole();
  const createQuotaRequest = useCreateQuotaRequest();

  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [statusDialogTarget, setStatusDialogTarget] = useState<"suspended" | "active">("suspended");
  const [statusError, setStatusError] = useState<string | null>(null);

  const [revokeDialogOpen, setRevokeDialogOpen] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const [mfaDialogOpen, setMfaDialogOpen] = useState(false);
  const [mfaError, setMfaError] = useState<string | null>(null);

  const [resolutions, setResolutions] = useState<Record<string, WorkspaceResolutionChoice>>({});
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [actionNotice, setActionNotice] = useState<string | null>(null);

  // Tenant deactivation dialog state
  const [deactivateDialogOpen, setDeactivateDialogOpen] = useState(false);
  const [deactivateError, setDeactivateError] = useState<string | null>(null);

  // Change member role dialog state
  const [roleDialogWorkspace, setRoleDialogWorkspace] = useState<Membership | null>(null);
  const [selectedRole, setSelectedRole] = useState<MemberRole>("member");
  const [roleError, setRoleError] = useState<string | null>(null);

  // Remove member from workspace dialog state
  const [removeDialogWorkspace, setRemoveDialogWorkspace] = useState<Membership | null>(null);
  const [removeError, setRemoveError] = useState<string | null>(null);

  // Workspace quota increase request dialog state (for admin / system_admin)
  const [quotaDialogWorkspace, setQuotaDialogWorkspace] = useState<Membership | null>(null);
  const [quotaGate, setQuotaGate] = useState("agents");
  const [quotaRequestedValue, setQuotaRequestedValue] = useState("");
  const [quotaReason, setQuotaReason] = useState("");
  const [quotaError, setQuotaError] = useState<string | null>(null);

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

  // Role matrix permissions (platform-management/internal/staff/access.go)
  const canControlAccount =
    effectiveRole === "system_admin" || effectiveRole === "support";
  const canDeleteAccount =
    effectiveRole === "system_admin" && accountStatus !== "deleted";
  const canManageMembers =
    effectiveRole === "system_admin" || effectiveRole === "admin";
  const canEditWorkspaceAllocation =
    effectiveRole === "system_admin" || effectiveRole === "billing";
  const canRequestWorkspaceQuota =
    effectiveRole === "system_admin" || effectiveRole === "admin";

  const isStaffTarget = Boolean(tenantUser?.is_staff);
  const isSelfTarget = Boolean(me?.user_id && me.user_id === user.id);

  // Prefer tenant-scoped memberships when viewing in a tenant context so
  // deactivate/member actions accurately reflect that tenant's workspaces.
  const memberships: Membership[] = tenantUser?.workspaces
    ? tenantUser.workspaces.map((w) => ({
        workspace_id: w.workspace_id,
        name: w.name,
        role: w.member_role,
      }))
    : user.memberships;

  const canDeactivateInTenant =
    canManageMembers && Boolean(effectiveTenantId) && memberships.length > 0 && !isStaffTarget;

  const allocationItems: TenantUserAllocationItem[] =
    tenantUser?.allocation?.length
      ? tenantUser.allocation
      : (user.allocation ?? []);

  const auditEvents: AuditEvent[] =
    tenantUser?.audit_events?.length
      ? tenantUser.audit_events
      : (user.audit_events ?? []);

  const hasMembershipActions =
    canManageMembers || canEditWorkspaceAllocation || canRequestWorkspaceQuota;

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
      setActionNotice(`Immediate deletion requested (${res.request_id}).`);
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

  const handleConfirmDeactivateInTenant = async (reason: string) => {
    if (!effectiveTenantId) return;
    setDeactivateError(null);
    try {
      const res = await deactivateTenantUser.mutateAsync({
        tenantId: effectiveTenantId,
        userId,
        reason,
      });
      setDeactivateDialogOpen(false);
      setActionNotice(
        `Tenant deactivation requested (${res.request_ids.length} workspace removal request${res.request_ids.length === 1 ? "" : "s"}).`,
      );
    } catch (err: unknown) {
      setDeactivateError(
        getMemberChangeErrorMessage(err, "Failed to deactivate user in tenant"),
      );
    }
  };

  const handleOpenRoleDialog = (membership: Membership) => {
    setRoleDialogWorkspace(membership);
    setSelectedRole(isMemberRole(membership.role) ? membership.role : "member");
    setRoleError(null);
  };

  const handleSubmitRoleChange = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!roleDialogWorkspace || changeWorkspaceMemberRole.isPending) return;
    setRoleError(null);
    try {
      const res = await changeWorkspaceMemberRole.mutateAsync({
        workspaceId: roleDialogWorkspace.workspace_id,
        userId,
        role: selectedRole,
        tenantId: effectiveTenantId,
      });
      setRoleDialogWorkspace(null);
      setActionNotice(
        res.changed === false
          ? "Member role is already up to date."
          : `Role change requested (${res.request_id}).`,
      );
    } catch (err: unknown) {
      setRoleError(
        getMemberChangeErrorMessage(err, "Failed to change member role"),
      );
    }
  };

  const handleConfirmRemoveMember = async () => {
    if (!removeDialogWorkspace || removeWorkspaceMember.isPending) return;
    setRemoveError(null);
    try {
      const res = await removeWorkspaceMember.mutateAsync({
        workspaceId: removeDialogWorkspace.workspace_id,
        userId,
        tenantId: effectiveTenantId,
      });
      setRemoveDialogWorkspace(null);
      setActionNotice(
        res.request_id
          ? `Member removal requested (${res.request_id}).`
          : "Member removal requested.",
      );
    } catch (err: unknown) {
      setRemoveError(
        getMemberChangeErrorMessage(err, "Failed to remove member from workspace"),
      );
    }
  };

  const handleOpenQuotaDialog = (membership: Membership) => {
    setQuotaDialogWorkspace(membership);
    setQuotaGate("agents");
    setQuotaRequestedValue("");
    setQuotaReason("");
    setQuotaError(null);
  };

  const handleSubmitQuotaRequest = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!quotaDialogWorkspace || createQuotaRequest.isPending) return;
    const parsedValue = Number(quotaRequestedValue);
    if (!Number.isFinite(parsedValue) || parsedValue < 1 || !quotaReason.trim()) {
      return;
    }
    setQuotaError(null);
    try {
      const res = await createQuotaRequest.mutateAsync({
        workspaceId: quotaDialogWorkspace.workspace_id,
        gate: quotaGate,
        requested_value: parsedValue,
        reason: quotaReason.trim(),
      });
      const wsLabel = quotaDialogWorkspace.name || quotaDialogWorkspace.workspace_id;
      setQuotaDialogWorkspace(null);
      setActionNotice(`Quota increase requested for ${wsLabel} (${res.id}).`);
    } catch (err: unknown) {
      setQuotaError(
        getMemberChangeErrorMessage(err, "Failed to submit quota request"),
      );
    }
  };

  const hasOwnerWorkspace = memberships.some((m) => m.role === "owner");

  return (
    <div className="space-y-8">
      <PageHeader
        title={user.name || user.email}
        badge={
          <div className="flex flex-wrap items-center gap-1.5">
            <StatusBadge status={accountStatus} />
            {effectiveRole === "system_admin" && isStaffTarget && (
              <Badge variant="purple" label="Platform staff" />
            )}
          </div>
        }
        meta={
          <span className="font-mono">
            {user.email} &bull; ID: {user.id}
          </span>
        }
        backAction={{ label: backLabel, onClick: onBack }}
        actions={
          <>
            {canDeactivateInTenant && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setDeactivateError(null);
                  setDeactivateDialogOpen(true);
                }}
                className="gap-1.5"
              >
                <UserMinus className="size-4" aria-hidden="true" />
                <span>Deactivate in tenant</span>
              </Button>
            )}

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

      {actionNotice && (
        <Alert role="status">
          <Info className="size-4" aria-hidden="true" />
          <AlertDescription>{actionNotice}</AlertDescription>
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
          {tenantUser && (
            <>
              <MetadataListItem label="Tenant seats" tabular>
                {tenantUser.seats.toLocaleString()}
              </MetadataListItem>
              <MetadataListItem label="Last login" tabular>
                {tenantUser.last_login_at
                  ? new Date(tenantUser.last_login_at).toLocaleString()
                  : "-"}
              </MetadataListItem>
            </>
          )}
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
        {memberships.length === 0 ? (
          <EmptyState
            icon={<Building2 className="size-5" aria-hidden="true" />}
            title="No memberships"
            description="This user does not belong to any workspaces."
            isCompact
          />
        ) : (
          <div className="overflow-x-auto">
            <Table density="balanced" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>Workspace Name</TableHeaderCell>
                  <TableHeaderCell>Role</TableHeaderCell>
                  <TableHeaderCell>Workspace ID</TableHeaderCell>
                  {hasMembershipActions && (
                    <TableHeaderCell className="text-right">Actions</TableHeaderCell>
                  )}
                </TableRow>
              </TableHeader>
              <TableBody>
                {memberships.map((membership: Membership) => {
                  const isOwner = membership.role === "owner";
                  return (
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
                      {hasMembershipActions && (
                        <TableCell className="text-right">
                          <RowActions
                            label={`workspace ${membership.name}`}
                            actions={[
                              ...(canManageMembers
                                ? [
                                    {
                                      label: "Change role",
                                      disabled: isOwner || isSelfTarget || isStaffTarget,
                                      title: isOwner
                                        ? "Owner role cannot be changed by staff"
                                        : isSelfTarget
                                          ? "You cannot change your own role"
                                          : isStaffTarget
                                            ? "Active staff members cannot be changed here"
                                            : undefined,
                                      onSelect: () => handleOpenRoleDialog(membership),
                                    },
                                    {
                                      label: "Remove",
                                      disabled: isOwner || isStaffTarget,
                                      title: isOwner
                                        ? "Owner cannot be removed by staff"
                                        : isStaffTarget
                                          ? "Active staff members cannot be changed here"
                                          : undefined,
                                      onSelect: () => {
                                        setRemoveError(null);
                                        setRemoveDialogWorkspace(membership);
                                      },
                                    },
                                  ]
                                : []),
                              ...(canEditWorkspaceAllocation
                                ? [
                                    {
                                      label: "Workspace allocation",
                                      onSelect: () => onSelectWorkspace?.(membership.workspace_id),
                                    },
                                  ]
                                : []),
                              ...(canRequestWorkspaceQuota
                                ? [
                                    {
                                      label: "Request quota increase",
                                      onSelect: () => handleOpenQuotaDialog(membership),
                                    },
                                  ]
                                : []),
                            ]}
                          />
                        </TableCell>
                      )}
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </Section>

      {allocationItems.length > 0 && (
        <>
          <Divider />
          <Section variant="transparent" padding={0} className="space-y-4">
            <Heading level={3}>Workspace Resource Allocation</Heading>
            <div className="overflow-x-auto">
              <Table density="compact" dividers="rows" hasHover>
                <TableHeader>
                  <TableRow isHeaderRow>
                    <TableHeaderCell>Workspace</TableHeaderCell>
                    <TableHeaderCell>Role</TableHeaderCell>
                    <TableHeaderCell>Seat</TableHeaderCell>
                    <TableHeaderCell>Workspace seats</TableHeaderCell>
                    <TableHeaderCell>Limits &amp; usage</TableHeaderCell>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {allocationItems.map((item) => (
                    <TableRow key={item.workspace_id}>
                      <TableCell>
                        <Text type="body" weight="medium">
                          {item.workspace_name || item.workspace_id}
                        </Text>
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={item.member_role} />
                      </TableCell>
                      <TableCell>
                        <Text type="supporting">
                          {item.occupies_seat ? "Occupied" : "Exempt"}
                        </Text>
                      </TableCell>
                      <TableCell>
                        <Text type="body" hasTabularNumbers>
                          {item.seats_used !== null && item.seats_purchased !== null
                            ? `${item.seats_used} / ${item.seats_purchased}`
                            : "-"}
                        </Text>
                      </TableCell>
                      <TableCell>
                        <Text type="supporting" hasTabularNumbers>
                          {formatLimitsSummary(item)}
                        </Text>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </Section>
        </>
      )}

      {effectiveTenantId && (
        <>
          <Divider />
          <Section variant="transparent" padding={0} className="space-y-4">
            <Heading level={3}>Recent Tenant Activity</Heading>
            {auditEvents.length === 0 ? (
              <EmptyState
                icon={<Activity className="size-5" aria-hidden="true" />}
                title="No recent tenant activity"
                isCompact
              />
            ) : (
              <div className="overflow-x-auto">
                <Table density="compact" dividers="rows" hasHover>
                  <TableHeader>
                    <TableRow isHeaderRow>
                      <TableHeaderCell>Time</TableHeaderCell>
                      <TableHeaderCell>Action</TableHeaderCell>
                      <TableHeaderCell>Actor</TableHeaderCell>
                      <TableHeaderCell>Target</TableHeaderCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {auditEvents.map((ev) => (
                      <TableRow key={ev.id}>
                        <TableCell>
                          <Text type="supporting" hasTabularNumbers>
                            {ev.created_at ? new Date(ev.created_at).toLocaleString() : "-"}
                          </Text>
                        </TableCell>
                        <TableCell>
                          <Text type="code">{ev.action}</Text>
                        </TableCell>
                        <TableCell>
                          <Text type="body">{ev.actor_label || ev.actor_id || "-"}</Text>
                        </TableCell>
                        <TableCell>
                          <Text type="body">{ev.target_label || ev.target_id || "-"}</Text>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </Section>
        </>
      )}

      <ConfirmReasonDialog
        open={deactivateDialogOpen}
        onOpenChange={(open) => {
          setDeactivateDialogOpen(open);
          if (!open) setDeactivateError(null);
        }}
        title="Deactivate user in tenant"
        description="Removes this user from the tenant workspaces below without changing their global account or other tenants."
        confirmLabel="Deactivate in tenant"
        confirmVariant="destructive"
        isPending={deactivateTenantUser.isPending}
        error={deactivateError}
        onConfirm={handleConfirmDeactivateInTenant}
      >
        <div className="space-y-2">
          <Text type="supporting" weight="medium" as="div">
            Workspaces affected ({memberships.length})
          </Text>
          <div className="space-y-1.5">
            {memberships.map((ws) => (
              <Card key={ws.workspace_id} padding={2}>
                <div className="flex items-center justify-between gap-2">
                  <Text type="body" weight="medium">
                    {ws.name || ws.workspace_id}
                  </Text>
                  <StatusBadge status={ws.role} />
                </div>
              </Card>
            ))}
          </div>
          {hasOwnerWorkspace && (
            <Alert>
              <Info className="size-4 shrink-0" aria-hidden="true" />
              <AlertDescription className="text-caption">
                This user is an owner in at least one workspace. Staff cannot remove an owner until ownership is transferred.
              </AlertDescription>
            </Alert>
          )}
        </div>
      </ConfirmReasonDialog>

      <Dialog
        open={roleDialogWorkspace !== null}
        onOpenChange={(open) => {
          if (!open && !changeWorkspaceMemberRole.isPending) {
            setRoleDialogWorkspace(null);
            setRoleError(null);
          }
        }}
      >
        <DialogContent>
          <form onSubmit={handleSubmitRoleChange} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Change member role</DialogTitle>
              <DialogDescription>
                {roleDialogWorkspace
                  ? `Update workspace role in ${roleDialogWorkspace.name || roleDialogWorkspace.workspace_id}.`
                  : ""}
              </DialogDescription>
            </DialogHeader>

            <Field>
              <FieldLabel htmlFor="change-member-role-select" className="text-label">
                Member role
              </FieldLabel>
              <Select
                items={MEMBER_ROLE_ITEMS}
                value={selectedRole}
                onValueChange={(value) => {
                  if (value && isMemberRole(value)) setSelectedRole(value);
                }}
                disabled={changeWorkspaceMemberRole.isPending}
              >
                <SelectTrigger
                  id="change-member-role-select"
                  aria-label="Member role"
                  className="w-full"
                >
                  <SelectValue>
                    {MEMBER_ROLE_ITEMS.find((item) => item.value === selectedRole)?.label ??
                      selectedRole}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {MEMBER_ROLE_ITEMS.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            {roleError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{roleError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={
                  <Button
                    type="button"
                    variant="outline"
                    disabled={changeWorkspaceMemberRole.isPending}
                  />
                }
              >
                Cancel
              </DialogClose>
              <Button
                type="submit"
                variant="default"
                disabled={changeWorkspaceMemberRole.isPending}
                aria-busy={changeWorkspaceMemberRole.isPending}
              >
                {changeWorkspaceMemberRole.isPending && <Spinner className="size-3.5" />}
                Save role
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={removeDialogWorkspace !== null}
        onOpenChange={(open) => {
          if (!open && !removeWorkspaceMember.isPending) {
            setRemoveDialogWorkspace(null);
            setRemoveError(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove from workspace</DialogTitle>
            <DialogDescription>
              {removeDialogWorkspace
                ? `Queue removal of ${user.email} from ${removeDialogWorkspace.name || removeDialogWorkspace.workspace_id} and release their workspace seat.`
                : ""}
            </DialogDescription>
          </DialogHeader>

          {removeError && (
            <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              <AlertDescription className="text-caption">{removeError}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <DialogClose
              render={
                <Button
                  type="button"
                  variant="outline"
                  disabled={removeWorkspaceMember.isPending}
                />
              }
            >
              Cancel
            </DialogClose>
            <Button
              type="button"
              variant="destructive"
              disabled={removeWorkspaceMember.isPending}
              aria-busy={removeWorkspaceMember.isPending}
              onClick={handleConfirmRemoveMember}
            >
              {removeWorkspaceMember.isPending && <Spinner className="size-3.5" />}
              Remove member
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={quotaDialogWorkspace !== null}
        onOpenChange={(open) => {
          if (!open && !createQuotaRequest.isPending) {
            setQuotaDialogWorkspace(null);
            setQuotaError(null);
          }
        }}
      >
        <DialogContent>
          <form onSubmit={handleSubmitQuotaRequest} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Request workspace quota increase</DialogTitle>
              <DialogDescription>
                {quotaDialogWorkspace
                  ? `Quotas apply per workspace. Submit a request for ${quotaDialogWorkspace.name || quotaDialogWorkspace.workspace_id} (${quotaDialogWorkspace.workspace_id}).`
                  : ""}
              </DialogDescription>
            </DialogHeader>

            <Field>
              <FieldLabel htmlFor="quota-gate-select" className="text-label">
                Resource gate
              </FieldLabel>
              <Select
                items={LIMIT_GATE_ITEMS}
                value={quotaGate}
                onValueChange={(value) => {
                  if (value) setQuotaGate(value);
                }}
                disabled={createQuotaRequest.isPending}
              >
                <SelectTrigger
                  id="quota-gate-select"
                  aria-label="Resource gate"
                  className="w-full"
                >
                  <SelectValue>
                    {LIMIT_GATE_ITEMS.find((item) => item.value === quotaGate)?.label ??
                      quotaGate}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {LIMIT_GATE_ITEMS.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel htmlFor="quota-requested-value" className="text-label">
                Requested limit <span className="text-destructive">*</span>
              </FieldLabel>
              <Input
                id="quota-requested-value"
                type="number"
                min={1}
                value={quotaRequestedValue}
                onChange={(e) => setQuotaRequestedValue(e.target.value)}
                required
                disabled={createQuotaRequest.isPending}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="quota-reason-input" className="text-label">
                Reason <span className="text-destructive">*</span>
              </FieldLabel>
              <Textarea
                id="quota-reason-input"
                value={quotaReason}
                onChange={(e) => setQuotaReason(e.target.value)}
                placeholder="Explain why this workspace limit should be raised..."
                rows={3}
                maxLength={500}
                required
                disabled={createQuotaRequest.isPending}
              />
            </Field>

            {quotaError && (
              <Alert variant="destructive" className="border-destructive/30 bg-destructive/5 py-2">
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                <AlertDescription className="text-caption">{quotaError}</AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <DialogClose
                render={
                  <Button
                    type="button"
                    variant="outline"
                    disabled={createQuotaRequest.isPending}
                  />
                }
              >
                Cancel
              </DialogClose>
              <Button
                type="submit"
                variant="default"
                disabled={
                  !quotaReason.trim() ||
                  !quotaRequestedValue ||
                  Number(quotaRequestedValue) < 1 ||
                  createQuotaRequest.isPending
                }
                aria-busy={createQuotaRequest.isPending}
              >
                {createQuotaRequest.isPending && <Spinner className="size-3.5" />}
                Submit request
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

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
