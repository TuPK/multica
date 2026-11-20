import { useId, useMemo, useState, type FormEvent } from "react";
import { Field } from "@astryxdesign/core/Field";
import { Heading } from "@astryxdesign/core/Heading";
import { HStack, Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
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
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Field as UiField, FieldLabel as UiFieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { AlertCircle, Lock, Plus, RefreshCw } from "@multica/ui/icons";
import { RowActions } from "../components/RowActions";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { StatusBadge } from "../components/StatusBadge";
import { getLegalHoldErrorMessage } from "../api/client";
import {
  useLegalHolds,
  useMe,
  usePlaceLegalHold,
  useReleaseLegalHold,
} from "../api/hooks";
import type { LegalHold, LegalHoldScopeKind } from "../api/schemas";
import type { StaffRole } from "../components/types";
import { t } from "../i18n";
import { PageHeader } from "../components/PageHeader";

export interface LegalHoldsProps {
  userRole?: StaffRole;
}

const SCOPE_OPTIONS = [
  { value: "tenant", label: "Tenant" },
  { value: "workspace", label: "Workspace" },
  { value: "account", label: "Account" },
] as const;

const STATUS_FILTERS = [
  { value: "all", label: "All holds" },
  { value: "active", label: "Active holds" },
  { value: "released", label: "Released holds" },
] as const;

export function LegalHolds({ userRole }: LegalHoldsProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canAccess = !effectiveRole || effectiveRole === "system_admin";

  const query = useLegalHolds(canAccess);
  const placeHold = usePlaceLegalHold();
  const releaseHold = useReleaseLegalHold();

  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "released">("all");
  const [placeDialogOpen, setPlaceDialogOpen] = useState(false);
  const [scopeKind, setScopeKind] = useState<Exclude<LegalHoldScopeKind, "unknown">>("tenant");
  const [scopeId, setScopeId] = useState("");
  const [caseRef, setCaseRef] = useState("");
  const [reason, setReason] = useState("");
  const [placeError, setPlaceError] = useState<string | null>(null);

  const [releaseTarget, setReleaseTarget] = useState<LegalHold | null>(null);
  const [releaseError, setReleaseError] = useState<string | null>(null);

  const scopeIdInputId = useId();
  const caseRefInputId = useId();
  const reasonInputId = useId();

  const holds = useMemo(() => {
    const items = query.data ?? [];
    if (statusFilter === "active") return items.filter((item) => !item.released);
    if (statusFilter === "released") return items.filter((item) => item.released);
    return items;
  }, [query.data, statusFilter]);

  if (!canAccess) {
    return (
      <div role="region" aria-label={t("Not found")}>
        <ErrorState
          title={t("Not found")}
          message={t("The page you requested was not found or you do not have permission to view it.")}
        />
      </div>
    );
  }

  const openPlaceDialog = () => {
    setScopeKind("tenant");
    setScopeId("");
    setCaseRef("");
    setReason("");
    setPlaceError(null);
    setPlaceDialogOpen(true);
  };

  const canSubmitPlace =
    scopeId.trim() !== "" &&
    caseRef.trim() !== "" &&
    reason.trim() !== "" &&
    !placeHold.isPending;

  const handlePlaceSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!canSubmitPlace) return;
    setPlaceError(null);
    try {
      await placeHold.mutateAsync({
        scope_kind: scopeKind,
        scope_id: scopeId.trim(),
        case_ref: caseRef.trim(),
        reason: reason.trim(),
      });
      setPlaceDialogOpen(false);
    } catch (err) {
      setPlaceError(getLegalHoldErrorMessage(err, t("Failed to place legal hold")));
    }
  };

  const handleReleaseConfirm = async () => {
    if (!releaseTarget) return;
    setReleaseError(null);
    try {
      await releaseHold.mutateAsync({ id: releaseTarget.id });
      setReleaseTarget(null);
    } catch (err) {
      setReleaseError(getLegalHoldErrorMessage(err, t("Failed to release legal hold")));
    }
  };

  return (
    <Layout height="auto">
      <LayoutHeader padding={0}>
          <PageHeader
            title={t("Legal holds")}
            actions={
              <>
                <HStack gap={2} align="center">
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() => query.refetch()}
                              disabled={query.isFetching}
                              aria-busy={query.isFetching || undefined}
                            >
                              {query.isFetching ? (
                                <Spinner />
                              ) : (
                                <RefreshCw className="size-4" aria-hidden="true" />
                              )}
                              {t("Refresh")}
                            </Button>
                            <Button type="button" variant="default" size="sm" onClick={openPlaceDialog}>
                              <Plus className="size-4" aria-hidden="true" />
                              {t("Place legal hold")}
                            </Button>
                          </HStack>
              </>
            }
          />
        </LayoutHeader>

      <LayoutContent padding={0}>
        <div className="space-y-6">
          <Section padding={4}>
            <div className="space-y-4">
              <Heading level={3}>{t("Holds")}</Heading>
              <HStack wrap="wrap" gap={3} align="center">
                <div className="w-56">
                  <Select
                    items={[...STATUS_FILTERS]}
                    value={statusFilter}
                    onValueChange={(val) => {
                      if (val === "all" || val === "active" || val === "released") {
                        setStatusFilter(val);
                      }
                    }}
                  >
                    <SelectTrigger aria-label={t("Hold status")} className="w-full">
                      <SelectValue>
                        {t(STATUS_FILTERS.find((item) => item.value === statusFilter)?.label ??
                          t("All holds"))}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {STATUS_FILTERS.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {t(item.label)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </HStack>

              {query.isLoading && <LoadingState message={t("Loading legal holds...")} />}

              {query.isError && (
                <ErrorState
                  title={t("Failed to load legal holds")}
                  message={query.error instanceof Error ? query.error.message : undefined}
                  onRetry={query.refetch}
                />
              )}

              {!query.isLoading && !query.isError && holds.length === 0 && (
                <EmptyState
                  icon={<Lock className="size-5" aria-hidden="true" />}
                  title={t("No legal holds")}
                  description={t("Active preservation holds block immediate deletion of the covered entity.")}
                />
              )}

              {!query.isLoading && !query.isError && holds.length > 0 && (
                <Table density="compact" dividers="rows" hasHover>
                  <TableHeader>
                    <TableRow isHeaderRow>
                      <TableHeaderCell>{t("Case reference")}</TableHeaderCell>
                      <TableHeaderCell>{t("Scope")}</TableHeaderCell>
                      <TableHeaderCell>{t("Reason")}</TableHeaderCell>
                      <TableHeaderCell>{t("Placed by")}</TableHeaderCell>
                      <TableHeaderCell>{t("Status")}</TableHeaderCell>
                      <TableHeaderCell className="text-right">{t("Actions")}</TableHeaderCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {holds.map((hold) => (
                      <TableRow key={hold.id}>
                        <TableCell>
                          <span className="font-mono text-body text-foreground">
                            {hold.case_ref || "—"}
                          </span>
                        </TableCell>
                        <TableCell>
                          <div className="min-w-0">
                            <div className="font-medium text-foreground capitalize">
                              {hold.scope_kind}
                            </div>
                            <div className="font-mono text-caption text-muted-foreground">
                              {hold.scope_id}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Text type="supporting">{hold.reason || "—"}</Text>
                        </TableCell>
                        <TableCell>
                          <Text type="supporting">{hold.placed_by || "—"}</Text>
                        </TableCell>
                        <TableCell>
                          <StatusBadge
                            status={hold.released ? "released" : "legal_hold"}
                            label={hold.released ? t("Released") : t("Active")}
                          />
                        </TableCell>
                        <TableCell className="text-right">
                          {hold.released ? (
                            <Text type="supporting">—</Text>
                          ) : (
                            <RowActions
                              label={t("legal hold {0}", hold.id)}
                              actions={[
                                {
                                  label: t("Release"),
                                  onSelect: () => {
                                    setReleaseTarget(hold);
                                    setReleaseError(null);
                                  },
                                },
                              ]}
                            />
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </div>
          </Section>
        </div>
      </LayoutContent>

      <Dialog
        open={placeDialogOpen}
        onOpenChange={(open) => {
          if (!open && !placeHold.isPending) {
            setPlaceDialogOpen(false);
          }
        }}
      >
        <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto">
          <form onSubmit={handlePlaceSubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>{t("Place legal hold")}</DialogTitle>
              <DialogDescription>
                {t("Prevents permanent deletion of the target tenant, workspace, or account until released.")}
              </DialogDescription>
            </DialogHeader>

            {placeError && (
              <Alert variant="destructive">
                <AlertCircle className="size-4" />
                <AlertDescription>{placeError}</AlertDescription>
              </Alert>
            )}

            <UiField>
              <UiFieldLabel htmlFor="legal-hold-scope-kind" className="text-label">
                {t("Scope kind")}
              </UiFieldLabel>
              <Select
                items={[...SCOPE_OPTIONS]}
                value={scopeKind}
                onValueChange={(val) => {
                  if (val === "tenant" || val === "workspace" || val === "account") {
                    setScopeKind(val);
                  }
                }}
              >
                <SelectTrigger id="legal-hold-scope-kind" aria-label={t("Scope kind")} className="w-full">
                  <SelectValue>
                    {t(SCOPE_OPTIONS.find((item) => item.value === scopeKind)?.label ?? t("Tenant"))}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {SCOPE_OPTIONS.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {t(item.label)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </UiField>

            <Field label={t("Scope ID")} inputID={scopeIdInputId} isRequired width="100%">
              <Input
                id={scopeIdInputId}
                value={scopeId}
                onChange={(e) => setScopeId(e.target.value)}
                placeholder={t("UUID of the tenant, workspace, or account")}
                disabled={placeHold.isPending}
              />
            </Field>

            <Field label={t("Case reference")} inputID={caseRefInputId} isRequired width="100%">
              <Input
                id={caseRefInputId}
                value={caseRef}
                onChange={(e) => setCaseRef(e.target.value)}
                placeholder={t("CASE-2026-001")}
                disabled={placeHold.isPending}
              />
            </Field>

            <Field label={t("Reason")} inputID={reasonInputId} isRequired width="100%">
              <Textarea
                id={reasonInputId}
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder={t("Explain the litigation or compliance preservation basis...")}
                disabled={placeHold.isPending}
                rows={3}
              />
            </Field>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setPlaceDialogOpen(false)}
                disabled={placeHold.isPending}
              >
                {t("Cancel")}
              </Button>
              <Button
                type="submit"
                variant="default"
                disabled={!canSubmitPlace}
                aria-busy={placeHold.isPending || undefined}
              >
                {placeHold.isPending && <Spinner />}
                {t("Place hold")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={releaseTarget !== null}
        onOpenChange={(open) => {
          if (!open && !releaseHold.isPending) {
            setReleaseTarget(null);
            setReleaseError(null);
          }
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("Release legal hold")}</DialogTitle>
            {releaseTarget && (
              <DialogDescription>
                {t("Release hold")} {releaseTarget.case_ref} {t("covering")} {releaseTarget.scope_kind}{" "}
                {releaseTarget.scope_id}?
              </DialogDescription>
            )}
          </DialogHeader>

          {releaseError && (
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>{releaseError}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setReleaseTarget(null)}
              disabled={releaseHold.isPending}
            >
              {t("Cancel")}
            </Button>
            <Button
              type="button"
              variant="default"
              disabled={releaseHold.isPending}
              aria-busy={releaseHold.isPending || undefined}
              onClick={handleReleaseConfirm}
            >
              {releaseHold.isPending && <Spinner />}
              {t("Release hold")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Layout>
  );
}
