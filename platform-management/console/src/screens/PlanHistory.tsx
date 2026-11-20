import { Card } from "@astryxdesign/core/Card";
import { Heading } from "@astryxdesign/core/Heading";
import { Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
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
import { History } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { useMe, usePlan, usePlanHistory } from "../api/hooks";
import type { PlanHistoryEntry, StaffRole } from "../api/schemas";
import { t } from "../i18n";

interface PlanHistoryProps {
  planKey: string | undefined;
  onBack?: () => void;
  userRole?: StaffRole;
}

function formatHistoryValue(value: unknown): string {
  if (value === null || value === undefined) return "-";
  if (typeof value === "string") return value === "" ? t("(empty)") : value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return JSON.stringify(value);
}

export function PlanHistory({ planKey, onBack, userRole }: PlanHistoryProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;
  const canView =
    effectiveRole === "system_admin" || effectiveRole === "billing";
  const canReadPlanMeta =
    effectiveRole === "system_admin" || effectiveRole === "admin";

  const { data: plan } = usePlan(
    planKey,
    Boolean(planKey) && canReadPlanMeta,
  );
  const {
    data: entries = [],
    isLoading,
    isError,
    error,
    refetch,
  } = usePlanHistory(planKey, Boolean(planKey) && canView);

  if (effectiveRole && !canView) {
    return (
      <div role="region" aria-label={t("Not found")}>
        <ErrorState
          title={t("Not found")}
          message={t("The page you requested was not found or you do not have permission to view it.")}
        />
      </div>
    );
  }

  return (
    <Layout
      header={
        <LayoutHeader padding={0}>
          <PageHeader
            title={
              plan?.name
                ? t("Plan History: {0}", plan.name)
                : planKey
                  ? t("Plan History: {0}", planKey)
                  : t("Plan History")
            }
            backAction={
              onBack
                ? { label: t("Back to plans"), onClick: onBack }
                : undefined
            }
            meta={
              plan ? (
                <span>
                  {plan.name}{" "}
                  <span className="font-mono text-muted-foreground">
                    ({plan.key})
                  </span>
                </span>
              ) : planKey ? (
                <span className="font-mono">{planKey}</span>
              ) : undefined
            }
          />
        </LayoutHeader>
      }
      content={
        <LayoutContent padding={0}>
          <div className="space-y-6">
            <Section>
              <div className="space-y-4">
                <Heading level={3}>{t("Change log")}</Heading>

                {isLoading && (
                  <LoadingState message={t("Loading plan history...")} />
                )}

                {isError && (
                  <ErrorState
                    title={t("Failed to load plan history")}
                    message={error instanceof Error ? error.message : undefined}
                    onRetry={refetch}
                  />
                )}

                {!isLoading && !isError && entries.length === 0 && (
                  <EmptyState
                    icon={<History className="size-5" aria-hidden="true" />}
                    title={t("No plan changes recorded")}
                    description={t("Plan updates, activations, and deactivations will appear here.")}
                  />
                )}

                {!isLoading && !isError && entries.length > 0 && (
                  <Card padding={0} className="overflow-x-auto">
                    <Table density="compact" dividers="rows" hasHover>
                      <TableHeader>
                        <TableRow isHeaderRow>
                          <TableHeaderCell>{t("When")}</TableHeaderCell>
                          <TableHeaderCell>{t("Who")}</TableHeaderCell>
                          <TableHeaderCell>{t("Action")}</TableHeaderCell>
                          <TableHeaderCell>{t("Field")}</TableHeaderCell>
                          <TableHeaderCell>{t("Old")}</TableHeaderCell>
                          <TableHeaderCell>{t("New")}</TableHeaderCell>
                          <TableHeaderCell>{t("Reason")}</TableHeaderCell>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {entries.map((entry: PlanHistoryEntry, idx: number) => (
                          <TableRow key={`${entry.at}-${entry.action}-${idx}`}>
                            <TableCell>
                              <Text type="supporting" hasTabularNumbers>
                                {entry.at
                                  ? new Date(entry.at).toLocaleString()
                                  : "-"}
                              </Text>
                            </TableCell>
                            <TableCell>
                              <Text type="body">{entry.actor || "-"}</Text>
                            </TableCell>
                            <TableCell>
                              <span className="font-mono text-caption text-foreground">
                                {entry.action}
                              </span>
                            </TableCell>
                            <TableCell>
                              {entry.changes.length === 0 ? (
                                <Text type="supporting">-</Text>
                              ) : (
                                <div className="space-y-1">
                                  {entry.changes.map((c, cIdx) => (
                                    <div
                                      key={`${c.field}-${cIdx}`}
                                      className="font-mono text-caption text-foreground"
                                    >
                                      {c.field}
                                    </div>
                                  ))}
                                </div>
                              )}
                            </TableCell>
                            <TableCell>
                              {entry.changes.length === 0 ? (
                                <Text type="supporting">-</Text>
                              ) : (
                                <div className="space-y-1">
                                  {entry.changes.map((c, cIdx) => (
                                    <div
                                      key={`${c.field}-old-${cIdx}`}
                                      className="max-w-52 truncate text-caption text-muted-foreground"
                                      title={formatHistoryValue(c.old)}
                                    >
                                      {formatHistoryValue(c.old)}
                                    </div>
                                  ))}
                                </div>
                              )}
                            </TableCell>
                            <TableCell>
                              {entry.changes.length === 0 ? (
                                <Text type="supporting">-</Text>
                              ) : (
                                <div className="space-y-1">
                                  {entry.changes.map((c, cIdx) => (
                                    <div
                                      key={`${c.field}-new-${cIdx}`}
                                      className="max-w-52 truncate text-caption text-foreground"
                                      title={formatHistoryValue(c.new)}
                                    >
                                      {formatHistoryValue(c.new)}
                                    </div>
                                  ))}
                                </div>
                              )}
                            </TableCell>
                            <TableCell>
                              <Text type="supporting">
                                {entry.reason || "-"}
                              </Text>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </Card>
                )}
              </div>
            </Section>
          </div>
        </LayoutContent>
      }
    />
  );
}
