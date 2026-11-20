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
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Search, Users as UsersIcon, X } from "@multica/ui/icons";
import { RowActions } from "../components/RowActions";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { StatusBadge } from "../components/StatusBadge";
import { useMe, useTenantUsers, type TenantUser } from "../api/hooks";
import {
  ACCOUNT_STATUSES,
  TENANT_MEMBER_ROLES,
  type StaffRole,
} from "../api/schemas";

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
        ? "Pending deletion"
        : status.charAt(0).toUpperCase() + status.slice(1),
  })),
];

const MEMBER_ROLE_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "all", label: "All roles" },
  ...TENANT_MEMBER_ROLES.map((role) => ({
    value: role,
    label: role.charAt(0).toUpperCase() + role.slice(1),
  })),
];

export function TenantUsers({
  tenantId,
  workspaces = [],
  onSelectUser,
  userRole,
}: TenantUsersProps) {
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
    isFetchingNextPage,
  } = useTenantUsers(tenantId, {
    q: search.trim() || undefined,
    account_status: statusFilter || undefined,
    workspace_id: workspaceFilter || undefined,
    member_role: roleFilter || undefined,
  });

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
      { value: "all", label: "All workspaces" },
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
        <Heading level={3}>Users</Heading>
        {!isLoading && !isError && (
          <Text type="supporting" hasTabularNumbers>
            {filteredUsers.length.toLocaleString()} shown
          </Text>
        )}
      </div>

      <HStack wrap="wrap" gap={2} align="center">
        <div className="relative w-full max-w-sm">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            type="search"
            aria-label="Search users"
            placeholder="Search by email or name..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pr-8 pl-8"
          />
          {search && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label="Clear search"
              onClick={() => setSearch("")}
              className="absolute top-1/2 right-1.5 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <X className="size-3.5" aria-hidden="true" />
            </Button>
          )}
        </div>

        <Select
          items={ACCOUNT_STATUS_OPTIONS}
          value={statusFilter || "all"}
          onValueChange={(value) =>
            setStatusFilter(value === "all" ? "" : (value ?? ""))
          }
        >
          <SelectTrigger aria-label="Filter by status" className="w-44">
            <SelectValue placeholder="All statuses" />
          </SelectTrigger>
          <SelectContent>
            {ACCOUNT_STATUS_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          items={workspaceOptions}
          value={workspaceFilter || "all"}
          onValueChange={(value) =>
            setWorkspaceFilter(value === "all" ? "" : (value ?? ""))
          }
        >
          <SelectTrigger aria-label="Filter by workspace" className="w-48">
            <SelectValue placeholder="All workspaces" />
          </SelectTrigger>
          <SelectContent>
            {workspaceOptions.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          items={MEMBER_ROLE_OPTIONS}
          value={roleFilter || "all"}
          onValueChange={(value) =>
            setRoleFilter(value === "all" ? "" : (value ?? ""))
          }
        >
          <SelectTrigger aria-label="Filter by role" className="w-40">
            <SelectValue placeholder="All roles" />
          </SelectTrigger>
          <SelectContent>
            {MEMBER_ROLE_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {hasActiveFilters && (
          <Button type="button" variant="ghost" size="sm" onClick={clearFilters}>
            Clear filters
          </Button>
        )}
      </HStack>

      {isLoading && <LoadingState message="Loading tenant users..." />}

      {isError && (
        <ErrorState
          title="Failed to load tenant users"
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && filteredUsers.length === 0 && (
        <EmptyState
          icon={<UsersIcon className="size-5" aria-hidden="true" />}
          title="No users found"
          description={
            hasActiveFilters
              ? "No tenant users matched your current filters."
              : "This tenant has no workspace members yet."
          }
          actions={
            hasActiveFilters ? (
              <Button variant="outline" size="sm" onClick={clearFilters}>
                Clear filters
              </Button>
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && filteredUsers.length > 0 && (
        <div className="space-y-4">
          <div className="overflow-x-auto">
            <Table density="balanced" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>User</TableHeaderCell>
                  <TableHeaderCell>Workspaces &amp; Roles</TableHeaderCell>
                  <TableHeaderCell>Status</TableHeaderCell>
                  <TableHeaderCell className="text-right">Seats</TableHeaderCell>
                  <TableHeaderCell>Last login</TableHeaderCell>
                  <TableHeaderCell className="text-right">Action</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredUsers.map((user: TenantUser) => (
                  <TableRow key={user.user_id}>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-1.5">
                        <span
                          className="max-w-52 truncate font-medium text-foreground"
                          title={user.name || user.email}
                        >
                          {user.name || user.email || user.user_id}
                        </span>
                        {isSystemAdmin && user.is_staff && (
                          <Badge variant="purple" label="Staff" />
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
                    <TableCell className="text-right">
                      <RowActions label="user" actions={[{ label: "View", onSelect: () => onSelectUser(user.user_id) }]} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {hasNextPage && (
            <div className="flex justify-center pt-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={isFetchingNextPage}
                aria-busy={isFetchingNextPage || undefined}
                onClick={() => void fetchNextPage()}
              >
                {isFetchingNextPage && (
                  <Spinner className="size-4" aria-hidden="true" />
                )}
                {isFetchingNextPage ? "Loading..." : "Load more"}
              </Button>
            </div>
          )}
        </div>
      )}
    </Section>
  );
}
