import { useState } from "react";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Search, X } from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { useUsers } from "../api/hooks";
import type { User } from "../api/schemas";

interface UsersProps {
  onSelectUser: (id: string) => void;
}

export function Users({ onSelectUser }: UsersProps) {
  const [search, setSearch] = useState("");
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
      <PageHeader title="Users" />

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-full max-w-sm">
          <Search
            className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            aria-label="Search users"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by email or name..."
            className="pr-8 pl-9"
          />
          {search && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label="Clear search"
              onClick={() => setSearch("")}
              className="absolute top-1 right-1 text-muted-foreground hover:text-foreground"
            >
              <X className="size-3.5" aria-hidden="true" />
            </Button>
          )}
        </div>
      </div>

      {isLoading && <LoadingState message="Loading users..." />}

      {isError && (
        <ErrorState
          title="Failed to load users"
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && users.length === 0 && (
        <EmptyState
          title="No users found"
          description={
            search
              ? "No users matched your search query."
              : "There are currently no registered users."
          }
          actions={
            search ? (
              <Button variant="outline" size="sm" onClick={() => setSearch("")}>
                Clear search
              </Button>
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && users.length > 0 && (
        <div className="space-y-4">
          <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Email</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Workspaces</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((user: User) => (
                  <TableRow key={user.id}>
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
                    <TableCell>{user.name || "-"}</TableCell>
                    <TableCell>
                      <StatusBadge status={user.status} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {user.memberships_count}
                    </TableCell>
                    <TableCell className="text-muted-foreground tabular-nums">
                      {user.created_at ? new Date(user.created_at).toLocaleString() : "-"}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => onSelectUser(user.id)}
                      >
                        View
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {hasNextPage && (
            <div className="flex justify-center pt-2">
              <Button
                variant="outline"
                onClick={() => fetchNextPage()}
                disabled={isFetchingNextPage}
                aria-busy={isFetchingNextPage}
              >
                {isFetchingNextPage && <Spinner className="size-3.5" />}
                Load more
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
