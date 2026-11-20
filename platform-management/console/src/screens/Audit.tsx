import { useMemo, useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Heading } from "@astryxdesign/core/Heading";
import { Section } from "@astryxdesign/core/Section";
import { proportional, Table } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Selector } from "@astryxdesign/core/Selector";
import { Field } from "@astryxdesign/core/Field";
import { Activity, Download } from "@multica/ui/icons";
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
    to: to ? new Date(to).toISOString() : undefined };

  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage } = useAuditEvents(filters);

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
        actor_id: actorId || undefined });

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
            <Button
              label={t("Clear filters")}
              variant="secondary"
              size="sm"
              onClick={clearFilters}
            />
          ) : undefined
        }
      />

      <Card padding={4}>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Selector
            label={t("Action")}
            width="100%"
            options={actionItems.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
            value={action || "all"}
            onChange={(val) => setAction(!val || val === "all" ? "" : val)}
          />

          <TextInput
            id="audit-actor-input"
            label={t("Actor ID")}
            type="text"
            value={actorId}
            onChange={(value) => setActorId(value)}
            placeholder={t("Filter by actor ID...")}
            width="100%"
          />

          <Field label={t("From Date")} inputID="audit-from-input" width="100%">
            <DatePickerField
              id="audit-from-input"
              value={from}
              onChange={setFrom}
              placeholder={t("Select start date")}
            />
          </Field>

          <Field label={t("To Date")} inputID="audit-to-input" width="100%">
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
              <Banner
                status="error"
                role="alert"
                title={exportError}
              />
            )}
            {exportNotice && (
              <Text type="supporting" color="secondary" role="status">
                {exportNotice}
              </Text>
            )}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 items-end">
              <Field label={t("Export from")} inputID="audit-export-from-input" width="100%">
                <DatePickerField
                  id="audit-export-from-input"
                  value={exportFrom}
                  onChange={setExportFrom}
                  placeholder={t("Select start date")}
                />
              </Field>

              <Field label={t("Export to")} inputID="audit-export-to-input" width="100%">
                <DatePickerField
                  id="audit-export-to-input"
                  value={exportTo}
                  onChange={setExportTo}
                  placeholder={t("Select end date")}
                />
              </Field>

              <Selector
                label={t("Format")}
                width="100%"
                options={EXPORT_FORMAT_ITEMS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                value={exportFormat}
                onChange={(val) => {
                    if (val === "csv" || val === "ndjson") setExportFormat(val);
                  }}
              />

              <div>
                <Button
                  type="button"
                  label={t("Export audit log")}
                  variant="secondary"
                  icon={<Download className="size-4" aria-hidden="true" />}
                  isLoading={exportAudit.isPending}
                  aria-busy={exportAudit.isPending || undefined}
                  onClick={handleExport}
                  isDisabled={exportAudit.isPending}
                />
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
                  ) },
                {
                  key: "action",
                  header: t("Action"),
                  width: proportional(1.5, { minWidth: 160 }),
                  renderCell: (event) => (
                    <Text type="code" weight="medium">
                      {event.action}
                    </Text>
                  ) },
                {
                  key: "actor_label",
                  header: t("Actor"),
                  width: proportional(1.5, { minWidth: 180 }),
                  renderCell: (event) => (
                    <div className="flex flex-col">
                      <Text type="body">{event.actor_label}</Text>
                      <Text type="supporting">{event.actor_type}</Text>
                    </div>
                  ) },
                {
                  key: "target_label",
                  header: t("Target"),
                  width: proportional(1.5, { minWidth: 180 }),
                  renderCell: (event) => (
                    <div className="flex flex-col">
                      <Text type="body">{event.target_label || event.target_id}</Text>
                      <Text type="supporting">{event.target_type}</Text>
                    </div>
                  ) },
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
                  ) },
              ]}
            />
          </Card>

          {hasNextPage && (
            <div className="flex justify-center pt-2">
              <Button
                label={t("Load more")}
                variant="secondary"
                isLoading={isFetchingNextPage}
                onClick={() => fetchNextPage()}
                aria-busy={isFetchingNextPage}
                isDisabled={isFetchingNextPage}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
