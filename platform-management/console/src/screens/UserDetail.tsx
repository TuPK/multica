import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
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
  TableRow } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { Selector } from "@astryxdesign/core/Selector";
import { TextArea } from "@astryxdesign/core/TextArea";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { NumberInput } from "@astryxdesign/core/NumberInput";
import { RowActions } from "../components/RowActions";
import {
  Activity,
  Building2,
  UserMinus } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { getMemberChangeErrorMessage } from "../api/client";
import {
  useMe,
  useUser,
  useTenantUser,
  useDeactivateTenantUser,
  useRemoveWorkspaceMember,
  useChangeWorkspaceMemberRole,
  useCreateQuotaRequest,
  type MemberRole,
  type TenantUserAllocationItem } from "../api/hooks";
import { MEMBER_ROLES, type AuditEvent } from "../api/schemas";
import type { Membership, StaffRole } from "../components/types";
import { t } from "../i18n";

interface UserDetailProps {
  userId: string;
  tenantId?: string;
  onBack: () => void;
  backLabel?: string;
  onSelectWorkspace?: (workspaceId: string) => void;
  userRole?: StaffRole;
}


const MEMBER_ROLE_ITEMS: Array<{ value: MemberRole; label: string }> = MEMBER_ROLES.map((role) => ({
  value: role,
  label: role.charAt(0).toUpperCase() + role.slice(1) }));

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
      const limitLabel = limit === null ? t("Unlimited") : limit.toLocaleString();
      return used !== undefined ? `${gate}: ${used.toLocaleString()} / ${limitLabel}` : `${gate}: ${limitLabel}`;
    })
    .join(" · ");
}

export function UserDetail({
  userId,
  tenantId,
  onBack,
  backLabel = t("Back to users"),
  onSelectWorkspace,
  userRole }: UserDetailProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;

  const {
    data: user,
    isLoading,
    isError,
    error,
    refetch } = useUser(userId);

  const effectiveTenantId = tenantId ?? me?.tenant_id ?? user?.tenant_id ?? undefined;

  const { data: tenantUser } = useTenantUser(
    effectiveTenantId,
    userId,
    Boolean(effectiveTenantId),
  );

  const deactivateTenantUser = useDeactivateTenantUser();
  const removeWorkspaceMember = useRemoveWorkspaceMember();
  const changeWorkspaceMemberRole = useChangeWorkspaceMemberRole();
  const createQuotaRequest = useCreateQuotaRequest();

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
    return <LoadingState message={t("Loading user details...")} />;
  }

  if (isError || !user) {
    return (
      <ErrorState
        title={t("Failed to load user")}
        message={
          error instanceof Error
            ? error.message
            : t("The user could not be found.")
        }
        onRetry={refetch}
      />
    );
  }

  const accountStatus = user.account?.status || user.status;
  const isSuspended = accountStatus === "suspended";

  // Role matrix permissions (platform-management/internal/staff/access.go)
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
        role: w.member_role }))
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

  const handleConfirmDeactivateInTenant = async (reason: string) => {
    if (!effectiveTenantId) return;
    setDeactivateError(null);
    try {
      const res = await deactivateTenantUser.mutateAsync({
        tenantId: effectiveTenantId,
        userId,
        reason });
      setDeactivateDialogOpen(false);
      setActionNotice(
        t("Tenant deactivation requested ({0} workspace removal requests).", res.request_ids.length),
      );
    } catch (err: unknown) {
      setDeactivateError(
        getMemberChangeErrorMessage(err, t("Failed to deactivate user in tenant")),
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
        tenantId: effectiveTenantId });
      setRoleDialogWorkspace(null);
      setActionNotice(
        res.changed === false
          ? t("Member role is already up to date.")
          : t("Role change requested ({0}).", res.request_id),
      );
    } catch (err: unknown) {
      setRoleError(
        getMemberChangeErrorMessage(err, t("Failed to change member role")),
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
        tenantId: effectiveTenantId });
      setRemoveDialogWorkspace(null);
      setActionNotice(
        res.request_id
          ? t("Member removal requested ({0}).", res.request_id)
          : t("Member removal requested."),
      );
    } catch (err: unknown) {
      setRemoveError(
        getMemberChangeErrorMessage(err, t("Failed to remove member from workspace")),
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
        reason: quotaReason.trim() });
      const wsLabel = quotaDialogWorkspace.name || quotaDialogWorkspace.workspace_id;
      setQuotaDialogWorkspace(null);
      setActionNotice(t("Quota increase requested for {0} ({1}).", wsLabel, res.id));
    } catch (err: unknown) {
      setQuotaError(
        getMemberChangeErrorMessage(err, t("Failed to submit quota request")),
      );
    }
  };

  const hasOwnerWorkspace = memberships.some((m) => m.role === "owner");

  return (
    <div className="space-y-8">
      <PageHeader
        title={user.name || user.email}
        details={
          <>
            <StatusBadge status={accountStatus} />
            {effectiveRole === "system_admin" && isStaffTarget && (
              <Badge variant="purple" label={t("Platform staff")} />
            )}
            <span className="font-mono text-caption text-muted-foreground">
              {user.email} {t("• ID:")} {user.id}
            </span>
          </>
        }
        backAction={{ label: backLabel, onClick: onBack }}
        actions={
          <>
            {canDeactivateInTenant && (
              <Button
                label={t("Deactivate in tenant")}
                variant="secondary"
                size="sm"
                icon={<UserMinus className="size-4" aria-hidden="true" />}
                onClick={() => {
                  setDeactivateError(null);
                  setDeactivateDialogOpen(true);
                }}
                className="gap-1.5"
              />
            )}


          </>
        }
      />

      {actionNotice && (
        <Banner
          status="info"
          role="status"
          title={actionNotice}
        />
      )}

      {(isSuspended || user.account?.reason) && (
        <Banner
          status="info"
          title={isSuspended ? t("Account suspended") : t("Status Reason")}
          description={user.account.reason}
        />
      )}

      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>{t("Account Record")}</Heading>
        <MetadataList columns={3}>
          <MetadataListItem label={t("Account Status")}>
            <StatusBadge status={accountStatus} />
          </MetadataListItem>
          <MetadataListItem label={t("Registered")} tabular>
            {user.created_at ? new Date(user.created_at).toLocaleString() : "-"}
          </MetadataListItem>
          {tenantUser && (
            <>
              <MetadataListItem label={t("Tenant seats")} tabular>
                {tenantUser.seats.toLocaleString()}
              </MetadataListItem>
              <MetadataListItem label={t("Last login")} tabular>
                {tenantUser.last_login_at
                  ? new Date(tenantUser.last_login_at).toLocaleString()
                  : "-"}
              </MetadataListItem>
            </>
          )}
          {user.account?.changed_at && (
            <MetadataListItem label={t("Status Changed At")} tabular>
              {new Date(user.account.changed_at).toLocaleString()}
            </MetadataListItem>
          )}
          {user.account?.sessions_valid_after && (
            <MetadataListItem label={t("Sessions Valid After")} mono tabular>
              {new Date(user.account.sessions_valid_after).toLocaleString()}
            </MetadataListItem>
          )}
        </MetadataList>
      </Section>

      <Divider />

      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>{t("Workspace Memberships")}</Heading>
        {memberships.length === 0 ? (
          <EmptyState
            icon={<Building2 className="size-5" aria-hidden="true" />}
            title={t("No memberships")}
            description={t("This user does not belong to any workspaces.")}
            isCompact
          />
        ) : (
          <Card padding={0} className="overflow-x-auto">
            <Table density="compact" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>{t("Workspace Name")}</TableHeaderCell>
                  <TableHeaderCell>{t("Role")}</TableHeaderCell>
                  <TableHeaderCell>{t("Workspace ID")}</TableHeaderCell>
                  {hasMembershipActions && (
                    <TableHeaderCell className="text-right">{t("Actions")}</TableHeaderCell>
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
                            label={t("workspace {0}", membership.name)}
                            actions={[
                              ...(canManageMembers
                                ? [
                                    {
                                      label: t("Change role"),
                                      disabled: isOwner || isSelfTarget || isStaffTarget,
                                      title: isOwner
                                        ? t("Owner role cannot be changed by staff")
                                        : isSelfTarget
                                          ? t("You cannot change your own role")
                                          : isStaffTarget
                                            ? t("Active staff members cannot be changed here")
                                            : undefined,
                                      onSelect: () => handleOpenRoleDialog(membership) },
                                    {
                                      label: t("Remove"),
                                      disabled: isOwner || isStaffTarget,
                                      title: isOwner
                                        ? t("Owner cannot be removed by staff")
                                        : isStaffTarget
                                          ? t("Active staff members cannot be changed here")
                                          : undefined,
                                      onSelect: () => {
                                        setRemoveError(null);
                                        setRemoveDialogWorkspace(membership);
                                      } },
                                  ]
                                : []),
                              ...(canEditWorkspaceAllocation
                                ? [
                                    {
                                      label: t("Workspace allocation"),
                                      onSelect: () => onSelectWorkspace?.(membership.workspace_id) },
                                  ]
                                : []),
                              ...(canRequestWorkspaceQuota
                                ? [
                                    {
                                      label: t("Request quota increase"),
                                      onSelect: () => handleOpenQuotaDialog(membership) },
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
          </Card>
        )}
      </Section>

      {allocationItems.length > 0 && (
        <>
          <Divider />
          <Section variant="transparent" padding={0} className="space-y-4">
            <Heading level={3}>{t("Workspace Resource Allocation")}</Heading>
            <Card padding={0} className="overflow-x-auto">
              <Table density="compact" dividers="rows" hasHover>
                <TableHeader>
                  <TableRow isHeaderRow>
                    <TableHeaderCell>{t("Workspace")}</TableHeaderCell>
                    <TableHeaderCell>{t("Role")}</TableHeaderCell>
                    <TableHeaderCell>{t("Seat")}</TableHeaderCell>
                    <TableHeaderCell>{t("Workspace seats")}</TableHeaderCell>
                    <TableHeaderCell>{t("Limits & usage")}</TableHeaderCell>
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
                          {item.occupies_seat ? t("Occupied") : t("Exempt")}
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
            </Card>
          </Section>
        </>
      )}

      {effectiveTenantId && (
        <>
          <Divider />
          <Section variant="transparent" padding={0} className="space-y-4">
            <Heading level={3}>{t("Recent Tenant Activity")}</Heading>
            {auditEvents.length === 0 ? (
              <EmptyState
                icon={<Activity className="size-5" aria-hidden="true" />}
                title={t("No recent tenant activity")}
                isCompact
              />
            ) : (
              <Card padding={0} className="overflow-x-auto">
                <Table density="compact" dividers="rows" hasHover>
                  <TableHeader>
                    <TableRow isHeaderRow>
                      <TableHeaderCell>{t("Time")}</TableHeaderCell>
                      <TableHeaderCell>{t("Action")}</TableHeaderCell>
                      <TableHeaderCell>{t("Actor")}</TableHeaderCell>
                      <TableHeaderCell>{t("Target")}</TableHeaderCell>
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
              </Card>
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
        title={t("Deactivate user in tenant")}
        description={t("Removes this user from the tenant workspaces below without changing their global account or other tenants.")}
        confirmLabel={t("Deactivate in tenant")}
        confirmVariant="destructive"
        isPending={deactivateTenantUser.isPending}
        error={deactivateError}
        onConfirm={handleConfirmDeactivateInTenant}
      >
        <div className="space-y-2">
          <Text type="supporting" weight="medium" as="div">
            {t("Workspaces affected (")}{memberships.length})
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
            <Banner
              status="info"
              title={t("This user is an owner in at least one workspace. Staff cannot remove an owner until ownership is transferred.")}
            />
          )}
        </div>
      </ConfirmReasonDialog>

      <Dialog
        isOpen={roleDialogWorkspace !== null}
        onOpenChange={(open) => {
          if (!open && !changeWorkspaceMemberRole.isPending) {
            setRoleDialogWorkspace(null);
            setRoleError(null);
          }
        }}
        purpose="form"
      >

          <form onSubmit={handleSubmitRoleChange} className="flex flex-col gap-4">
            <DialogHeader title={t("Change member role")} subtitle={roleDialogWorkspace
                  ? t("Update workspace role in {0}.", roleDialogWorkspace.name || roleDialogWorkspace.workspace_id)
                  : ""} />

            <Selector
              label={t("Member role")}
              width="100%"
              options={MEMBER_ROLE_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
              value={selectedRole}
              onChange={(value) => {
                  if (value && isMemberRole(value)) setSelectedRole(value);
                }}
            />

            {roleError && (
              <Banner
                status="error"
                role="alert"
                title={roleError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="secondary"
                isDisabled={changeWorkspaceMemberRole.isPending}
                label={t("Cancel")}
                onClick={() => ((open) => {
          if (!open && !changeWorkspaceMemberRole.isPending) {
            setRoleDialogWorkspace(null);
            setRoleError(null);
          }
        })(false)}
              />
              <Button
                type="submit"
                label={t("Save role")}
                variant="primary"
                isLoading={changeWorkspaceMemberRole.isPending}
                aria-busy={changeWorkspaceMemberRole.isPending}
                isDisabled={changeWorkspaceMemberRole.isPending}
              />
            </div>
          </form>

      </Dialog>

      <Dialog
        isOpen={removeDialogWorkspace !== null}
        onOpenChange={(open) => {
          if (!open && !removeWorkspaceMember.isPending) {
            setRemoveDialogWorkspace(null);
            setRemoveError(null);
          }
        }}
        purpose="info"
      >

          <DialogHeader title={t("Remove from workspace")} subtitle={removeDialogWorkspace
                ? t("Queue removal of {0} from {1} and release their workspace seat.", user.email, removeDialogWorkspace.name || removeDialogWorkspace.workspace_id)
                : ""} />

          {removeError && (
            <Banner
              status="error"
              role="alert"
              title={removeError}
            />
          )}

          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="secondary"
              isDisabled={removeWorkspaceMember.isPending}
              label={t("Cancel")}
              onClick={() => ((open) => {
          if (!open && !removeWorkspaceMember.isPending) {
            setRemoveDialogWorkspace(null);
            setRemoveError(null);
          }
        })(false)}
            />
            <Button
              type="button"
              label={t("Remove member")}
              variant="destructive"
              isLoading={removeWorkspaceMember.isPending}
              aria-busy={removeWorkspaceMember.isPending}
              onClick={handleConfirmRemoveMember}
              isDisabled={removeWorkspaceMember.isPending}
            />
          </div>

      </Dialog>

      <Dialog
        isOpen={quotaDialogWorkspace !== null}
        onOpenChange={(open) => {
          if (!open && !createQuotaRequest.isPending) {
            setQuotaDialogWorkspace(null);
            setQuotaError(null);
          }
        }}
        purpose="form"
      >

          <form onSubmit={handleSubmitQuotaRequest} className="flex flex-col gap-4">
            <DialogHeader title={t("Request workspace quota increase")} subtitle={quotaDialogWorkspace
                  ? t("Quotas apply per workspace. Submit a request for {0} ({1}).", quotaDialogWorkspace.name || quotaDialogWorkspace.workspace_id, quotaDialogWorkspace.workspace_id)
                  : ""} />

            <Selector
              label={t("Resource gate")}
              width="100%"
              options={LIMIT_GATE_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
              value={quotaGate}
              onChange={(value) => {
                  if (value) setQuotaGate(value);
                }}
            />

            <NumberInput
              label={t("Requested limit")}
              id={"quota-requested-value"}
              isRequired
              min={1}
              value={quotaRequestedValue.trim() === "" ? null : Number(quotaRequestedValue)}
              onChange={(value) => setQuotaRequestedValue(value === null ? "" : String(value))}
              isDisabled={createQuotaRequest.isPending}
              width="100%"
            />

            <TextArea
              label={t("Reason")}
              id={"quota-reason-input"}
              isRequired
              value={quotaReason}
              onChange={(value) => setQuotaReason(value.slice(0, 500))}
              placeholder={t("Explain why this workspace limit should be raised...")}
              rows={3}
              isDisabled={createQuotaRequest.isPending}
              width="100%"
            />

            {quotaError && (
              <Banner
                status="error"
                role="alert"
                title={quotaError}
              />
            )}

            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="secondary"
                isDisabled={createQuotaRequest.isPending}
                label={t("Cancel")}
                onClick={() => ((open) => {
          if (!open && !createQuotaRequest.isPending) {
            setQuotaDialogWorkspace(null);
            setQuotaError(null);
          }
        })(false)}
              />
              <Button
                type="submit"
                label={t("Submit request")}
                variant="primary"
                isLoading={createQuotaRequest.isPending}
                aria-busy={createQuotaRequest.isPending}
                isDisabled={
                  !quotaReason.trim() ||
                  !quotaRequestedValue ||
                  Number(quotaRequestedValue) < 1 ||
                  createQuotaRequest.isPending
                }
              />
            </div>
          </form>

      </Dialog>

    </div>
  );
}
