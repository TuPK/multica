import { useMemo, useState } from "react";
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
import { Button } from "@multica/ui/components/ui/button";
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
import { Activity } from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { DatePickerField } from "../components/DatePickerField";
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

  const hasFilters = Boolean(action || actorId || from || to);

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

  const actionItems = useMemo(
    () => [
      { value: "all", label: "All actions" },
      ...CATALOG_ACTIONS.map((code) => ({ value: code, label: code })),
    ],
    [],
  );

  const clearFilters = () => {
    setAction("");
    setActorId("");
    setFrom("");
    setTo("");
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Audit Log"
        actions={
          hasFilters ? (
            <Button variant="outline" size="sm" onClick={clearFilters}>
              Clear filters
            </Button>
          ) : undefined
        }
      />

      <Card padding={4}>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Field>
            <FieldLabel htmlFor="audit-action-select" className="text-label">
              Action
            </FieldLabel>
            <Select
              items={actionItems}
              value={action || "all"}
              onValueChange={(val) => setAction(!val || val === "all" ? "" : val)}
            >
              <SelectTrigger id="audit-action-select" aria-label="Action" className="w-full">
                <SelectValue>
                  {actionItems.find((item) => item.value === (action || "all"))?.label ??
                    "All actions"}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {actionItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field>
            <FieldLabel htmlFor="audit-actor-input" className="text-label">
              Actor ID
            </FieldLabel>
            <Input
              id="audit-actor-input"
              type="text"
              value={actorId}
              onChange={(e) => setActorId(e.target.value)}
              placeholder="Filter by actor ID..."
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="audit-from-input" className="text-label">
              From Date
            </FieldLabel>
            <DatePickerField
              id="audit-from-input"
              value={from}
              onChange={setFrom}
              placeholder="Select start date"
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="audit-to-input" className="text-label">
              To Date
            </FieldLabel>
            <DatePickerField
              id="audit-to-input"
              value={to}
              onChange={setTo}
              placeholder="Select end date"
            />
          </Field>
        </div>
      </Card>

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
          icon={<Activity className="size-5" aria-hidden="true" />}
          title="No audit events found"
          description="Try adjusting your filter criteria."
          actions={
            hasFilters ? (
              <Button variant="outline" size="sm" onClick={clearFilters}>
                Clear filters
              </Button>
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && events.length > 0 && (
        <div className="space-y-4">
          <Card padding={0} className="overflow-x-auto">
            <Table density="compact" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>Time</TableHeaderCell>
                  <TableHeaderCell>Action</TableHeaderCell>
                  <TableHeaderCell>Actor</TableHeaderCell>
                  <TableHeaderCell>Target</TableHeaderCell>
                  <TableHeaderCell>Details</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((event: AuditEvent) => (
                  <TableRow key={event.id}>
                    <TableCell className="whitespace-nowrap">
                      <Text type="supporting" hasTabularNumbers>
                        {event.created_at ? new Date(event.created_at).toLocaleString() : "-"}
                      </Text>
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      <Text type="code" weight="medium">
                        {event.action}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col">
                        <Text type="body">{event.actor_label}</Text>
                        <Text type="supporting">{event.actor_type}</Text>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col">
                        <Text type="body">{event.target_label || event.target_id}</Text>
                        <Text type="supporting">{event.target_type}</Text>
                      </div>
                    </TableCell>
                    <TableCell className="max-w-xs">
                      <span
                        className="block truncate font-mono text-caption text-muted-foreground"
                        title={event.detail ? JSON.stringify(event.detail) : undefined}
                      >
                        {event.detail ? JSON.stringify(event.detail) : "-"}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>

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
