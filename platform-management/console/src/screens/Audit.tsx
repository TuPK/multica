import { useMemo, useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Heading } from "@astryxdesign/core/Heading";
import { Section } from "@astryxdesign/core/Section";
import { proportional, Table } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
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
import { Activity, AlertCircle, Download } from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { DatePickerField } from "../components/DatePickerField";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { getApiErrorMessage } from "../api/client";
import { useAuditEvents, useExportAudit, useMe, type AuditFilters } from "../api/hooks";
import { CATALOG_ACTIONS } from "../components/types";
import type { AuditEvent } from "../api/schemas";
import { t } from "../i18n";

const MAX_EXPORT_RANGE_MS = 31 * 24 * 60 * 60 * 1000;

const EXPORT_FORMAT_ITEMS = [
  { value: "ndjson", label: "NDJSON (.ndjson)" },
  { value: "csv", label: "CSV (.csv)" },
] as const;

export function Audit() {
  const { data: me } = useMe();
  const canExport = !me?.role || me.role === "system_admin" || me.role === "admin";

  const [action, setAction] = useState("");
  const [actorId, setActorId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  const [exportFrom, setExportFrom] = useState("");
  const [exportTo, setExportTo] = useState("");
  const [exportFormat, setExportFormat] = useState<"ndjson" | "csv">("ndjson");
  const [exportError, setExportError] = useState<string | null>(null);
  const [exportNotice, setExportNotice] = useState<string | null>(null);

  const exportAudit = useExportAudit();

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
      { value: "all", label: t("All actions") },
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

  const handleExport = async () => {
    setExportError(null);
    setExportNotice(null);

    if (!exportFrom || !exportTo) {
      setExportError(t("Select both Export from and Export to dates."));
      return;
    }

    const startMs = Date.parse(`${exportFrom}T00:00:00Z`);
    const endInclusiveMs = Date.parse(`${exportTo}T00:00:00Z`);
    if (!Number.isFinite(startMs) || !Number.isFinite(endInclusiveMs) || endInclusiveMs < startMs) {
      setExportError(t("Export end date must be on or after the start date."));
      return;
    }

    const endExclusiveMs = endInclusiveMs + 24 * 60 * 60 * 1000;
    if (endExclusiveMs - startMs > MAX_EXPORT_RANGE_MS) {
      setExportError(t("Export range cannot exceed 31 days."));
      return;
    }

    try {
      const result = await exportAudit.mutateAsync({
        from: new Date(startMs).toISOString(),
        to: new Date(endExclusiveMs).toISOString(),
        format: exportFormat,
        action: action || undefined,
        actor_id: actorId || undefined,
      });

      if (typeof URL.createObjectURL === "function") {
        const mimeType =
          exportFormat === "csv" ? "text/csv;charset=utf-8" : "application/x-ndjson;charset=utf-8";
        const blob = new Blob([result.text], { type: mimeType });
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = url;
        link.download = `audit-events-${exportFrom}-to-${exportTo}.${exportFormat}`;
        link.click();
        URL.revokeObjectURL(url);
      }

      setExportNotice(
        result.nextCursor
          ? t("Downloaded export page (more matching events exist; narrow the window for all rows).")
          : t("Audit log export downloaded."),
      );
    } catch (err) {
      setExportError(getApiErrorMessage(err, t("Failed to export audit log")));
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Audit Log")}
        actions={
          hasFilters ? (
            <Button variant="outline" size="sm" onClick={clearFilters}>
              {t("Clear filters")}
            </Button>
          ) : undefined
        }
      />

      <Card padding={4}>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Field>
            <FieldLabel htmlFor="audit-action-select" className="text-label">
              {t("Action")}
            </FieldLabel>
            <Select
              items={actionItems}
              value={action || "all"}
              onValueChange={(val) => setAction(!val || val === "all" ? "" : val)}
            >
              <SelectTrigger id="audit-action-select" aria-label={t("Action")} className="w-full">
                <SelectValue>
                  {t(actionItems.find((item) => item.value === (action || "all"))?.label ??
                    t("All actions"))}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {actionItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {t(item.label)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field>
            <FieldLabel htmlFor="audit-actor-input" className="text-label">
              {t("Actor ID")}
            </FieldLabel>
            <Input
              id="audit-actor-input"
              type="text"
              value={actorId}
              onChange={(e) => setActorId(e.target.value)}
              placeholder={t("Filter by actor ID...")}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="audit-from-input" className="text-label">
              {t("From Date")}
            </FieldLabel>
            <DatePickerField
              id="audit-from-input"
              value={from}
              onChange={setFrom}
              placeholder={t("Select start date")}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="audit-to-input" className="text-label">
              {t("To Date")}
            </FieldLabel>
            <DatePickerField
              id="audit-to-input"
              value={to}
              onChange={setTo}
              placeholder={t("Select end date")}
            />
          </Field>
        </div>
      </Card>

      {canExport && (
        <Section padding={4}>
          <div className="space-y-4">
            <Heading level={3}>{t("Export audit log")}</Heading>
            {exportError && (
              <Alert variant="destructive">
                <AlertCircle className="size-4" />
                <AlertDescription>{exportError}</AlertDescription>
              </Alert>
            )}
            {exportNotice && (
              <Text type="supporting" color="secondary" role="status">
                {exportNotice}
              </Text>
            )}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 items-end">
              <Field>
                <FieldLabel htmlFor="audit-export-from-input" className="text-label">
                  {t("Export from")}
                </FieldLabel>
                <DatePickerField
                  id="audit-export-from-input"
                  value={exportFrom}
                  onChange={setExportFrom}
                  placeholder={t("Select start date")}
                />
              </Field>

              <Field>
                <FieldLabel htmlFor="audit-export-to-input" className="text-label">
                  {t("Export to")}
                </FieldLabel>
                <DatePickerField
                  id="audit-export-to-input"
                  value={exportTo}
                  onChange={setExportTo}
                  placeholder={t("Select end date")}
                />
              </Field>

              <Field>
                <FieldLabel htmlFor="audit-export-format-select" className="text-label">
                  {t("Format")}
                </FieldLabel>
                <Select
                  items={EXPORT_FORMAT_ITEMS}
                  value={exportFormat}
                  onValueChange={(val) => {
                    if (val === "csv" || val === "ndjson") setExportFormat(val);
                  }}
                >
                  <SelectTrigger
                    id="audit-export-format-select"
                    aria-label={t("Format")}
                    className="w-full"
                  >
                    <SelectValue>
                      {t(EXPORT_FORMAT_ITEMS.find((item) => item.value === exportFormat)?.label ??
                        t("NDJSON (.ndjson)"))}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {EXPORT_FORMAT_ITEMS.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {t(item.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <div>
                <Button
                  type="button"
                  variant="outline"
                  disabled={exportAudit.isPending}
                  aria-busy={exportAudit.isPending || undefined}
                  onClick={handleExport}
                >
                  {exportAudit.isPending ? (
                    <Spinner />
                  ) : (
                    <Download className="size-4" aria-hidden="true" />
                  )}
                  {t("Export audit log")}
                </Button>
              </div>
            </div>
            <Text type="supporting" color="secondary">
              {t("Maximum export window is 31 days. Currently applied action and actor filters apply to the export.")}
            </Text>
          </div>
        </Section>
      )}

      {isLoading && <LoadingState message={t("Loading audit log...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load audit events")}
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && events.length === 0 && (
        <EmptyState
          icon={<Activity className="size-5" aria-hidden="true" />}
          title={t("No audit events found")}
          description={t("Try adjusting your filter criteria.")}
          actions={
            hasFilters ? (
              <Button variant="outline" size="sm" onClick={clearFilters}>
                {t("Clear filters")}
              </Button>
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && events.length > 0 && (
        <div className="space-y-4">
          <Card padding={0} className="overflow-x-auto">
            <Table
              data={events}
              idKey="id"
              density="compact"
              dividers="rows"
              hasHover
              columns={[
                {
                  key: "created_at",
                  header: t("Time"),
                  width: proportional(1.2, { minWidth: 170 }),
                  renderCell: (event) => (
                    <Text type="supporting" hasTabularNumbers>
                      {event.created_at ? new Date(event.created_at).toLocaleString() : "-"}
                    </Text>
                  ),
                },
                {
                  key: "action",
                  header: t("Action"),
                  width: proportional(1.5, { minWidth: 160 }),
                  renderCell: (event) => (
                    <Text type="code" weight="medium">
                      {event.action}
                    </Text>
                  ),
                },
                {
                  key: "actor_label",
                  header: t("Actor"),
                  width: proportional(1.5, { minWidth: 180 }),
                  renderCell: (event) => (
                    <div className="flex flex-col">
                      <Text type="body">{event.actor_label}</Text>
                      <Text type="supporting">{event.actor_type}</Text>
                    </div>
                  ),
                },
                {
                  key: "target_label",
                  header: t("Target"),
                  width: proportional(1.5, { minWidth: 180 }),
                  renderCell: (event) => (
                    <div className="flex flex-col">
                      <Text type="body">{event.target_label || event.target_id}</Text>
                      <Text type="supporting">{event.target_type}</Text>
                    </div>
                  ),
                },
                {
                  key: "detail",
                  header: t("Details"),
                  width: proportional(3, { minWidth: 220 }),
                  renderCell: (event) => (
                    <span
                      className="block truncate font-mono text-caption text-muted-foreground"
                      title={event.detail ? JSON.stringify(event.detail) : undefined}
                    >
                      {event.detail ? JSON.stringify(event.detail) : "-"}
                    </span>
                  ),
                },
              ]}
            />
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
                {t("Load more")}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
