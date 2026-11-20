import { Card } from "@astryxdesign/core/Card";
import { useMemo, useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { HStack } from "@astryxdesign/core/HStack";
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
import { Selector } from "@astryxdesign/core/Selector";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Search, Users as UsersIcon } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { StatusBadge } from "../components/StatusBadge";
import { useMe, useTenantUsers, type TenantUser } from "../api/hooks";
import { TenantUserDeactivateMenu } from "./TenantUserDeactivateMenu";
import {
  ACCOUNT_STATUSES,
  TENANT_MEMBER_ROLES,
  type StaffRole } from "../api/schemas";
import { t } from "../i18n";

interface TenantUsersProps {
  tenantId: string;
  workspaces?: Array<{ id: string; name: string }>;
  onSelectUser: (userId: string) => void;
  userRole?: StaffRole;
}

const ACCOUNT_STATUS_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "all", label: "All statuses" },
  ...ACCOUNT_STATUSES.map((status) => ({
    value: status,
    label:
      status === "pending_deletion"
        ? t("Pending deletion")
        : status.charAt(0).toUpperCase() + status.slice(1) })),
];

const MEMBER_ROLE_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "all", label: "All roles" },
  ...TENANT_MEMBER_ROLES.map((role) => ({
    value: role,
    label: role.charAt(0).toUpperCase() + role.slice(1) })),
];

export function TenantUsers({
  tenantId,
  workspaces = [],
  onSelectUser,
  userRole }: TenantUsersProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const isSystemAdmin = effectiveRole === "system_admin";

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [workspaceFilter, setWorkspaceFilter] = useState("");
  const [roleFilter, setRoleFilter] = useState("");

  const {
    data: users = [],
    isLoading,
    isError,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage } = useTenantUsers(tenantId, {
    q: search.trim() || undefined,
    account_status: statusFilter || undefined,
    workspace_id: workspaceFilter || undefined,
    member_role: roleFilter || undefined });

  const workspaceOptions = useMemo(() => {
    const map = new Map<string, string>();
    for (const ws of workspaces) {
      map.set(ws.id, ws.name || ws.id);
    }
    for (const user of users) {
      for (const ws of user.workspaces) {
        if (!map.has(ws.workspace_id)) {
          map.set(ws.workspace_id, ws.name || ws.workspace_id);
        }
      }
    }
    return [
      { value: "all", label: t("All workspaces") },
      ...[...map.entries()]
        .sort((a, b) => a[1].localeCompare(b[1]))
        .map(([id, name]) => ({ value: id, label: name })),
    ];
  }, [workspaces, users]);

  // Client-side filter fallback for status, workspace, and role until the
  // backend listTenantUsers endpoint implements those query parameters.
  const filteredUsers = useMemo(() => {
    return users.filter((u: TenantUser) => {
      if (statusFilter && u.account_status !== statusFilter) return false;
      if (workspaceFilter && roleFilter) {
        return u.workspaces.some(
          (w) => w.workspace_id === workspaceFilter && w.member_role === roleFilter,
        );
      }
      if (workspaceFilter && !u.workspaces.some((w) => w.workspace_id === workspaceFilter)) {
        return false;
      }
      if (roleFilter && !u.workspaces.some((w) => w.member_role === roleFilter)) {
        return false;
      }
      return true;
    });
  }, [users, statusFilter, workspaceFilter, roleFilter]);

  const hasActiveFilters = Boolean(
    search.trim() || statusFilter || workspaceFilter || roleFilter,
  );

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("");
    setWorkspaceFilter("");
    setRoleFilter("");
  };

  return (
    <Section variant="transparent" padding={0} className="space-y-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <Heading level={3}>{t("Users")}</Heading>
        {!isLoading && !isError && (
          <Text type="supporting" hasTabularNumbers>
            {filteredUsers.length.toLocaleString()} {t("shown")}
          </Text>
        )}
      </div>

      <HStack wrap="wrap" gap={2} align="center">
        <div className="w-full max-w-sm">
  <TextInput
    type="search"
    label={t("Search users")}
    isLabelHidden
    width="100%"
    startIcon={<Search className="size-4" aria-hidden="true" />}
    hasClear
    placeholder={t("Search by email or name...")}
    value={search}
    onChange={(value) => setSearch(value)}
  />
</div>

        <Selector
          label={t("Filter by status")}
          isLabelHidden
          width={176}
          options={ACCOUNT_STATUS_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
          value={statusFilter || "all"}
          onChange={(value) =>
            setStatusFilter(value === "all" ? "" : (value ?? ""))}
        />

        <Selector
          label={t("Filter by workspace")}
          isLabelHidden
          width={192}
          options={workspaceOptions.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
          value={workspaceFilter || "all"}
          onChange={(value) =>
            setWorkspaceFilter(value === "all" ? "" : (value ?? ""))}
        />

        <Selector
          label={t("Filter by role")}
          isLabelHidden
          width={160}
          options={MEMBER_ROLE_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
          value={roleFilter || "all"}
          onChange={(value) =>
            setRoleFilter(value === "all" ? "" : (value ?? ""))}
        />

        {hasActiveFilters && (
          <Button
            type="button"
            label={t("Clear filters")}
            variant="ghost"
            size="sm"
            onClick={clearFilters}
          />
        )}
      </HStack>

      {isLoading && <LoadingState message={t("Loading tenant users...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load tenant users")}
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && filteredUsers.length === 0 && (
        <EmptyState
          icon={<UsersIcon className="size-5" aria-hidden="true" />}
          title={t("No users found")}
          description={
            hasActiveFilters
              ? t("No tenant users matched your current filters.")
              : t("This tenant has no workspace members yet.")
          }
          actions={
            hasActiveFilters ? (
              <Button
                label={t("Clear filters")}
                variant="secondary"
                size="sm"
                onClick={clearFilters}
              />
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && filteredUsers.length > 0 && (
        <div className="space-y-4">
          <Card padding={0} className="overflow-x-auto">
            <Table density="compact" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>{t("User")}</TableHeaderCell>
                  <TableHeaderCell>{t("Workspaces & Roles")}</TableHeaderCell>
                  <TableHeaderCell>{t("Status")}</TableHeaderCell>
                  <TableHeaderCell className="text-right">{t("Seats")}</TableHeaderCell>
                  <TableHeaderCell>{t("Last login")}</TableHeaderCell>
                  <TableHeaderCell className="text-right">{t("Action")}</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredUsers.map((user: TenantUser) => (
                  <TableRow key={user.user_id} onClick={() => onSelectUser(user.user_id)}>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-1.5">
                        <span
                          className="max-w-52 truncate font-medium text-foreground"
                          title={user.name || user.email}
                        >
                          {user.name || user.email || user.user_id}
                        </span>
                        {isSystemAdmin && user.is_staff && (
                          <Badge variant="purple" label={t("Staff")} />
                        )}
                      </div>
                      <div
                        className="max-w-56 truncate text-caption text-muted-foreground"
                        title={user.email}
                      >
                        {user.email || "-"}
                      </div>
                    </TableCell>
                    <TableCell>
                      {user.workspaces.length === 0 ? (
                        <Text type="supporting">-</Text>
                      ) : (
                        <div className="flex flex-col gap-1">
                          {user.workspaces.map((ws) => (
                            <div
                              key={ws.workspace_id}
                              className="flex flex-wrap items-center gap-1.5"
                            >
                              <Text type="body">{ws.name || ws.workspace_id}</Text>
                              <StatusBadge status={ws.member_role} />
                            </div>
                          ))}
                        </div>
                      )}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={user.account_status} />
                    </TableCell>
                    <TableCell className="text-right">
                      <Text type="body" hasTabularNumbers>
                        {user.seats.toLocaleString()}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" hasTabularNumbers>
                        {user.last_login_at
                          ? new Date(user.last_login_at).toLocaleString()
                          : "-"}
                      </Text>
                    </TableCell>
                    <TableCell className="text-right" onClick={(event) => event.stopPropagation()}>
                      <TenantUserDeactivateMenu
                        tenantId={tenantId}
                        user={user}
                        canManageMembers={effectiveRole === "system_admin" || effectiveRole === "admin"}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>

          {hasNextPage && (
            <div className="flex justify-center pt-2">
              <Button
                type="button"
                label={isFetchingNextPage ? t("Loading...") : t("Load more")}
                variant="secondary"
                size="sm"
                isLoading={isFetchingNextPage}
                aria-busy={isFetchingNextPage || undefined}
                onClick={() => void fetchNextPage()}
                isDisabled={isFetchingNextPage}
              />
            </div>
          )}
        </div>
      )}
    </Section>
  );
}
