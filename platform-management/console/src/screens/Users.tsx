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
import { Search } from "@multica/ui/icons";
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
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="font-heading text-title-lg font-bold">Users</h1>
          <p className="text-body text-muted-foreground">
            Search and manage Multica user accounts.
          </p>
        </div>
        <div className="relative w-full max-w-xs">
          <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
          <Input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by email or name..."
            className="pl-9"
          />
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
        />
      )}

      {!isLoading && !isError && users.length > 0 && (
        <div className="space-y-4">
          <div className="rounded-lg border border-surface-border bg-surface shadow-xs">
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
                    <TableCell className="font-medium">{user.email}</TableCell>
                    <TableCell>{user.name || "-"}</TableCell>
                    <TableCell>
                      <StatusBadge status={user.status} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {user.memberships_count}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
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
