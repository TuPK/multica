import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Search, Users as UsersIcon } from "@multica/ui/icons";
import { UserAccountMenu } from "./UserAccountMenu";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { useMe, useUsers } from "../api/hooks";
import type { User } from "../api/schemas";
import { t } from "../i18n";

interface UsersProps {
  onSelectUser: (id: string) => void;
}

export function Users({ onSelectUser }: UsersProps) {
  const [search, setSearch] = useState("");
  const { data: me } = useMe();
  const canControlAccount = me?.role === "system_admin" || me?.role === "support";
  const canDeleteAccount = me?.role === "system_admin";
  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useUsers({
    q: search || undefined,
  });

  const users: User[] = Array.isArray(data)
    ? data
    : (data as unknown as { users?: User[] })?.users || [];

  return (
    <div className="space-y-6">
      <PageHeader title={t("Users")} />

      <div className="flex flex-wrap items-center gap-2">
        <div className="w-full max-w-sm">
        <TextInput
          type="search"
          label={t("Search users")}
          isLabelHidden
          width="100%"
          startIcon={<Search className="size-4" aria-hidden="true" />}
          hasClear
          value={search}
          onChange={(value) => setSearch(value)}
          placeholder={t("Search by email or name...")}
        />
        </div>
      </div>

      {isLoading && <LoadingState message={t("Loading users...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load users")}
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && users.length === 0 && (
        <EmptyState
          icon={<UsersIcon className="size-5" aria-hidden="true" />}
          title={t("No users found")}
          description={
            search
              ? t("No users matched your search query.")
              : t("There are currently no registered users.")
          }
          actions={
            search ? (
              <Button label={t("Clear search")} variant="secondary" size="sm" onClick={() => setSearch("")} />
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && users.length > 0 && (
        <div className="space-y-4">
          <Card padding={0} className="overflow-x-auto">
            <Table density="balanced" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>{t("Email")}</TableHeaderCell>
                  <TableHeaderCell>{t("Name")}</TableHeaderCell>
                  <TableHeaderCell>{t("Status")}</TableHeaderCell>
                  <TableHeaderCell className="text-right">{t("Workspaces")}</TableHeaderCell>
                  <TableHeaderCell>{t("Created")}</TableHeaderCell>
                  <TableHeaderCell className="text-right">{t("Action")}</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((user: User) => (
                  <TableRow key={user.id} onClick={() => onSelectUser(user.id)}>
                    <TableCell>
                      <div className="max-w-60 truncate font-medium text-foreground" title={user.email}>
                        {user.email}
                      </div>
                      <div
                        className="max-w-44 truncate font-mono text-caption text-muted-foreground"
                        title={user.id}
                      >
                        {user.id}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Text type="body">{user.name || "-"}</Text>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={user.status} />
                    </TableCell>
                    <TableCell className="text-right">
                      <Text type="body" hasTabularNumbers>
                        {user.memberships_count}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" hasTabularNumbers>
                        {user.created_at ? new Date(user.created_at).toLocaleString() : "-"}
                      </Text>
                    </TableCell>
                    <TableCell className="text-right" onClick={(event) => event.stopPropagation()}>
                      <UserAccountMenu
                        userId={user.id}
                        email={user.email}
                        accountStatus={user.status}
                        canControlAccount={canControlAccount}
                        canDeleteAccount={canDeleteAccount && user.status !== "deleted"}
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
                label={t("Load more")}
                variant="secondary"
                onClick={() => fetchNextPage()}
                isDisabled={isFetchingNextPage}
                isLoading={isFetchingNextPage}
                aria-busy={isFetchingNextPage}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
