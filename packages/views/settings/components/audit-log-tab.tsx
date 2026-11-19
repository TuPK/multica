"use client";

import { useMemo, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Download, Loader2 } from "@multica/ui/icons";
import { toast } from "sonner";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@multica/ui/components/ui/table";
import { useCurrentWorkspace } from "@multica/core/paths";
import { useCan } from "@multica/core/permissions";
import {
  auditEventsOptions,
  exportAuditCsv,
  type AuditEvent,
  type AuditFilters,
} from "@multica/core/audit";
import { useLocale, useT } from "../../i18n";
import { SettingsReadOnlyNotice, SettingsTab } from "./settings-layout";

export const AUDIT_ACTIONS = [
  "member.role_changed",
  "member.removed",
  "member.left",
  "invitation.created",
  "invitation.revoked",
  "invitation.accepted",
  "invitation.declined",
  "share_link.created",
  "share_link.revoked",
  "share_link.joined",
  "workspace.created",
  "workspace.updated",
  "workspace.deleted",
  "auth.login",
  "auth.sessions_revoked",
  "pat.created",
  "pat.revoked",
] as const;

export function AuditLogTab() {
  const { t } = useT("settings");
  const locale = useLocale();
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id ?? "";
  const canReadAudit = useCan(wsId, "audit.read").allowed;

  const [actionFilter, setActionFilter] = useState<string>("");
  const [fromDate, setFromDate] = useState<string>("");
  const [toDate, setToDate] = useState<string>("");
  const [isExporting, setIsExporting] = useState(false);

  const filterParams = useMemo<AuditFilters>(() => {
    const filters: AuditFilters = {};
    if (actionFilter) filters.action = actionFilter;
    if (fromDate) filters.from = `${fromDate}T00:00:00Z`;
    if (toDate) filters.to = `${toDate}T23:59:59Z`;
    return filters;
  }, [actionFilter, fromDate, toDate]);

  const query = useInfiniteQuery({
    ...auditEventsOptions(wsId, filterParams),
    enabled: Boolean(wsId && canReadAudit),
  });

  const events = useMemo(
    () => query.data?.pages.flatMap((page) => page.events) ?? [],
    [query.data?.pages],
  );

  const actionItems = useMemo(
    () => [
      { value: "all", label: t(($) => $.audit.filter_action_all) },
      ...AUDIT_ACTIONS.map((action) => ({ value: action, label: action })),
    ],
    [t],
  );

  const formatTime = (value: string) => {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "";
    return new Intl.DateTimeFormat(locale, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  };

  const formatActor = (event: AuditEvent) => {
    return event.actor_label || event.actor_type || "-";
  };

  const formatTarget = (event: AuditEvent) => {
    return event.target_label || event.target_id || event.target_type || "-";
  };

  const handleExportCsv = async () => {
    if (!wsId || isExporting) return;
    setIsExporting(true);
    try {
      const data = await exportAuditCsv(wsId, filterParams);
      const text = typeof data === "string" ? data : data.text;
      const blob = new Blob([text], { type: "text/csv;charset=utf-8;" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `audit-events-${new Date().toISOString().slice(0, 10)}.csv`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch {
      toast.error(t(($) => $.audit.export_failed));
    } finally {
      setIsExporting(false);
    }
  };

  if (!canReadAudit) {
    return (
      <SettingsTab title={t(($) => $.page.tabs.audit)} scope="workspace">
        <SettingsReadOnlyNotice wsId={wsId} />
      </SettingsTab>
    );
  }

  return (
    <SettingsTab
      title={t(($) => $.page.tabs.audit)}
      scope="workspace"
      actions={
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isExporting}
          aria-busy={isExporting || undefined}
          onClick={handleExportCsv}
        >
          {isExporting ? <Loader2 className="animate-spin" /> : <Download />}
          {t(($) => $.audit.export_csv)}
        </Button>
      }
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <Select
            items={actionItems}
            value={actionFilter || "all"}
            onValueChange={(val) =>
              setActionFilter(!val || val === "all" ? "" : val)
            }
          >
            <SelectTrigger
              aria-label={t(($) => $.audit.filter_action)}
              className="w-56"
            >
              <SelectValue>
                {actionItems.find(
                  (item) => item.value === (actionFilter || "all"),
                )?.label ?? t(($) => $.audit.filter_action_all)}
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

          <Input
            type="date"
            aria-label={t(($) => $.audit.filter_from)}
            value={fromDate}
            onChange={(e) => setFromDate(e.target.value)}
            className="w-40"
          />

          <Input
            type="date"
            aria-label={t(($) => $.audit.filter_to)}
            value={toDate}
            onChange={(e) => setToDate(e.target.value)}
            className="w-40"
          />
        </div>

        {query.isError ? (
          <p role="alert" className="text-caption text-destructive">
            {t(($) => $.audit.load_failed)}
          </p>
        ) : null}

        {query.isLoading ? (
          <div className="flex justify-center py-12">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : query.isSuccess && events.length === 0 ? (
          <div className="flex flex-col items-center gap-3 py-12 text-center">
            <p className="text-body text-muted-foreground">
              {t(($) => $.audit.empty)}
            </p>
            {actionFilter || fromDate || toDate ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  setActionFilter("");
                  setFromDate("");
                  setToDate("");
                }}
              >
                {t(($) => $.audit.clear_filters)}
              </Button>
            ) : null}
          </div>
        ) : null}

        {events.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t(($) => $.audit.column_time)}</TableHead>
                  <TableHead>{t(($) => $.audit.column_actor)}</TableHead>
                  <TableHead>{t(($) => $.audit.column_action)}</TableHead>
                  <TableHead>{t(($) => $.audit.column_target)}</TableHead>
                  <TableHead>{t(($) => $.audit.column_ip)}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((event) => (
                  <TableRow key={event.id}>
                    <TableCell className="whitespace-nowrap tabular-nums text-muted-foreground">
                      {formatTime(event.created_at)}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      {formatActor(event)}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      <code className="font-mono text-caption text-foreground">
                        {event.action}
                      </code>
                    </TableCell>
                    <TableCell className="max-w-xs truncate">
                      {formatTarget(event)}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground">
                      {event.ip || "-"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ) : null}

        {query.hasNextPage ? (
          <div className="flex justify-center pt-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={query.isFetchingNextPage}
              aria-busy={query.isFetchingNextPage || undefined}
              onClick={() => void query.fetchNextPage()}
            >
              {query.isFetchingNextPage ? (
                <Loader2 className="animate-spin" />
              ) : null}
              {t(($) => $.audit.load_more)}
            </Button>
          </div>
        ) : null}
      </div>
    </SettingsTab>
  );
}
