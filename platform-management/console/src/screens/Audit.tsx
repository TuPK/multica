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
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { useAuditEvents, type AuditFilters } from "../api/hooks";
import { CATALOG_ACTIONS } from "../components/types";
import type { AuditEvent } from "../api/schemas";

export function Audit() {
  const [action, setAction] = useState("");
  const [actorId, setActorId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  const filters: AuditFilters = {
    action: action || undefined,
    actor_id: actorId || undefined,
    from: from ? new Date(from).toISOString() : undefined,
    to: to ? new Date(to).toISOString() : undefined,
  };

  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useAuditEvents(filters);

  const events: AuditEvent[] = Array.isArray(data)
    ? data
    : (data as unknown as { events?: AuditEvent[] })?.events || [];

  const handleFilterChange =
    (setter: (val: string) => void) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
      setter(e.target.value);
    };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-title-lg font-bold">Audit Log</h1>
        <p className="text-body text-muted-foreground">
          System-wide audit trail of security, administration, and tenant events.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 rounded-lg border border-surface-border bg-surface p-4 sm:grid-cols-2 lg:grid-cols-4">
        <div className="space-y-1.5">
          <label htmlFor="audit-action-select" className="text-caption font-medium text-foreground">
            Action
          </label>
          <select
            id="audit-action-select"
            value={action}
            onChange={handleFilterChange(setAction)}
            className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-body transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            <option value="">All actions</option>
            {CATALOG_ACTIONS.map((code) => (
              <option key={code} value={code}>
                {code}
              </option>
            ))}
          </select>
        </div>

        <div className="space-y-1.5">
          <label htmlFor="audit-actor-input" className="text-caption font-medium text-foreground">
            Actor ID
          </label>
          <Input
            id="audit-actor-input"
            type="text"
            value={actorId}
            onChange={handleFilterChange(setActorId)}
            placeholder="Filter by actor ID..."
          />
        </div>

        <div className="space-y-1.5">
          <label htmlFor="audit-from-input" className="text-caption font-medium text-foreground">
            From Date
          </label>
          <Input
            id="audit-from-input"
            type="date"
            value={from}
            onChange={handleFilterChange(setFrom)}
          />
        </div>

        <div className="space-y-1.5">
          <label htmlFor="audit-to-input" className="text-caption font-medium text-foreground">
            To Date
          </label>
          <Input
            id="audit-to-input"
            type="date"
            value={to}
            onChange={handleFilterChange(setTo)}
          />
        </div>
      </div>

      {isLoading && <LoadingState message="Loading audit log..." />}

      {isError && (
        <ErrorState
          title="Failed to load audit events"
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && events.length === 0 && (
        <EmptyState
          title="No audit events found"
          description="Try adjusting your filter criteria."
        />
      )}

      {!isLoading && !isError && events.length > 0 && (
        <div className="space-y-4">
          <div className="rounded-lg border border-surface-border bg-surface shadow-xs">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Target</TableHead>
                  <TableHead>Details</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((event: AuditEvent) => (
                  <TableRow key={event.id}>
                    <TableCell className="text-muted-foreground whitespace-nowrap">
                      {event.created_at ? new Date(event.created_at).toLocaleString() : "-"}
                    </TableCell>
                    <TableCell className="font-mono text-caption whitespace-nowrap font-medium">
                      {event.action}
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col">
                        <span>{event.actor_label}</span>
                        <span className="text-caption text-muted-foreground">
                          {event.actor_type}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col">
                        <span>{event.target_label || event.target_id}</span>
                        <span className="text-caption text-muted-foreground">
                          {event.target_type}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="max-w-xs truncate font-mono text-caption text-muted-foreground">
                      {event.detail ? JSON.stringify(event.detail) : "-"}
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
