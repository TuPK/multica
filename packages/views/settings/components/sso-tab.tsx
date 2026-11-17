"use client";

import { useEffect, useId, useMemo, useState, type FormEvent } from "react";
import { AlertCircle, Check, Copy, Loader2 } from "@multica/ui/icons";
import { ApiError, errorCode } from "@multica/core/api";
import { useCurrentWorkspace } from "@multica/core/paths";
import { useCurrentMember } from "@multica/core/permissions";
import {
  useAddSSODomain,
  useDeleteSSOConnection,
  useRemoveSSODomain,
  useSaveSSOConnection,
  useSSOConnection,
  useSSODomains,
  useVerifySSODomain,
} from "@multica/core/security";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { copyText } from "@multica/ui/lib/clipboard";
import { toast } from "sonner";
import { useT } from "../../i18n";
import {
  SettingsCard,
  SettingsReadOnlyNotice,
  SettingsRow,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";

function isPlanUnavailableError(error: unknown): boolean {
  return (
    errorCode(error) === "plan_feature_unavailable" ||
    (error instanceof ApiError && error.status === 402)
  );
}

function mapDomainError(
  error: unknown,
  t: ReturnType<typeof useT<"settings">>["t"],
): string {
  switch (errorCode(error)) {
    case "plan_feature_unavailable":
      return t(($) => $.sso.errors.plan_unavailable);
    case "domain_already_verified":
      return t(($) => $.sso.errors.domain_already_verified);
    case "public_email_domain":
      return t(($) => $.sso.errors.public_email_domain);
    case "invalid_domain":
      return t(($) => $.sso.errors.invalid_domain);
    case "domain_not_verified":
      return t(($) => $.sso.errors.domain_not_verified);
    case "txt_record_not_found":
      return t(($) => $.sso.errors.txt_record_not_found);
    case "dns_lookup_failed":
      return t(($) => $.sso.errors.dns_lookup_failed);
    case "viewer_read_only":
      return t(($) => $.sso.errors.forbidden);
    default:
      if (error instanceof ApiError && error.status === 402) {
        return t(($) => $.sso.errors.plan_unavailable);
      }
      if (error instanceof ApiError && error.status === 403) {
        return t(($) => $.sso.errors.forbidden);
      }
      return t(($) => $.sso.errors.domain_failed);
  }
}

function mapConnectionError(
  error: unknown,
  t: ReturnType<typeof useT<"settings">>["t"],
): string {
  switch (errorCode(error)) {
    case "plan_feature_unavailable":
      return t(($) => $.sso.errors.plan_unavailable);
    case "domain_not_verified":
      return t(($) => $.sso.errors.domain_not_verified);
    case "invalid_issuer":
      return t(($) => $.sso.errors.invalid_issuer);
    case "invalid_client_id":
      return t(($) => $.sso.errors.invalid_client_id);
    case "client_secret_required":
      return t(($) => $.sso.errors.client_secret_required);
    case "invalid_role":
      return t(($) => $.sso.errors.invalid_role);
    case "invalid_status":
      return t(($) => $.sso.errors.invalid_status);
    case "issuer_unreachable":
      return t(($) => $.sso.errors.issuer_unreachable);
    case "sso_unavailable":
      return t(($) => $.sso.errors.unavailable);
    case "viewer_read_only":
      return t(($) => $.sso.errors.forbidden);
    default:
      if (error instanceof ApiError && error.status === 402) {
        return t(($) => $.sso.errors.plan_unavailable);
      }
      if (error instanceof ApiError && error.status === 403) {
        return t(($) => $.sso.errors.forbidden);
      }
      return t(($) => $.sso.errors.connection_failed);
  }
}

export function SSOTab() {
  const { t } = useT("settings");
  const baseId = useId();
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id ?? "";
  const { role, isLoading: memberLoading } = useCurrentMember(wsId);
  const isOwner = role === "owner";
  const readOnly = !memberLoading && !isOwner;

  const domainsQuery = useSSODomains(isOwner ? wsId : "");
  const connectionQuery = useSSOConnection(isOwner ? wsId : "");
  const addDomain = useAddSSODomain(wsId);
  const verifyDomain = useVerifySSODomain(wsId);
  const removeDomain = useRemoveSSODomain(wsId);
  const saveConnection = useSaveSSOConnection(wsId);
  const deleteConnection = useDeleteSSOConnection(wsId);

  const domains = domainsQuery.data?.domains ?? [];
  const connection = connectionQuery.data ?? null;
  const hasVerifiedDomain = domains.some((d) => d.verified_at !== null);

  const [domainInput, setDomainInput] = useState("");
  const [domainError, setDomainError] = useState<string | null>(null);
  const [verifyingDomain, setVerifyingDomain] = useState<string | null>(null);
  const [removingDomain, setRemovingDomain] = useState<string | null>(null);

  const [issuer, setIssuer] = useState(connection?.issuer ?? "");
  const [clientId, setClientId] = useState(connection?.client_id ?? "");
  const [clientSecret, setClientSecret] = useState("");
  const [defaultRole, setDefaultRole] = useState(
    connection?.default_role ?? "member",
  );
  const [status, setStatus] = useState(connection?.status ?? "active");
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [planUnavailable, setPlanUnavailable] = useState(false);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  useEffect(() => {
    setIssuer(connection?.issuer ?? "");
    setClientId(connection?.client_id ?? "");
    setClientSecret("");
    setDefaultRole(connection?.default_role ?? "member");
    setStatus(connection?.status ?? "active");
  }, [
    connection?.client_id,
    connection?.default_role,
    connection?.id,
    connection?.issuer,
    connection?.status,
    connection?.updated_at,
  ]);

  useEffect(() => {
    setDomainInput("");
    setDomainError(null);
    setConnectionError(null);
    setPlanUnavailable(false);
  }, [wsId]);

  const roleItems = useMemo(() => {
    const base = [
      { value: "member", label: t(($) => $.members.roles.member.label) },
      { value: "admin", label: t(($) => $.members.roles.admin.label) },
      { value: "viewer", label: t(($) => $.members.roles.viewer.label) },
    ];
    if (defaultRole && !base.some((item) => item.value === defaultRole)) {
      base.push({ value: defaultRole, label: defaultRole });
    }
    return base;
  }, [defaultRole, t]);

  const statusItems = useMemo(() => {
    const base = [
      { value: "active", label: t(($) => $.sso.status_active) },
      { value: "disabled", label: t(($) => $.sso.status_disabled) },
    ];
    if (status && !base.some((item) => item.value === status)) {
      base.push({ value: status, label: status });
    }
    return base;
  }, [status, t]);

  if (!workspace) return null;

  const planBlocked =
    planUnavailable ||
    isPlanUnavailableError(domainsQuery.error) ||
    isPlanUnavailableError(connectionQuery.error);

  const handleCopy = async (key: string, value: string) => {
    if (await copyText(value)) {
      setCopiedKey(key);
      window.setTimeout(() => {
        setCopiedKey((current) => (current === key ? null : current));
      }, 2000);
    }
  };

  const handleAddDomain = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = domainInput.trim();
    if (!trimmed || planBlocked) return;
    setDomainError(null);
    try {
      await addDomain.mutateAsync(trimmed);
      setDomainInput("");
    } catch (error) {
      if (isPlanUnavailableError(error)) {
        setPlanUnavailable(true);
        return;
      }
      setDomainError(mapDomainError(error, t));
    }
  };

  const handleVerifyDomain = async (domain: string) => {
    setDomainError(null);
    setVerifyingDomain(domain);
    try {
      await verifyDomain.mutateAsync(domain);
      toast.success(t(($) => $.sso.domain_verified_toast));
    } catch (error) {
      if (isPlanUnavailableError(error)) {
        setPlanUnavailable(true);
        return;
      }
      setDomainError(mapDomainError(error, t));
    } finally {
      setVerifyingDomain(null);
    }
  };

  const handleRemoveDomain = async (domain: string) => {
    setDomainError(null);
    setRemovingDomain(domain);
    try {
      await removeDomain.mutateAsync(domain);
      toast.success(t(($) => $.sso.domain_removed_toast));
    } catch (error) {
      setDomainError(mapDomainError(error, t));
    } finally {
      setRemovingDomain(null);
    }
  };

  const handleSaveConnection = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!hasVerifiedDomain || planBlocked) return;
    setConnectionError(null);
    try {
      await saveConnection.mutateAsync({
        issuer: issuer.trim(),
        client_id: clientId.trim(),
        ...(clientSecret ? { client_secret: clientSecret } : {}),
        default_role: defaultRole,
        status,
      });
      setClientSecret("");
      toast.success(t(($) => $.sso.connection_saved_toast));
    } catch (error) {
      if (isPlanUnavailableError(error)) {
        setPlanUnavailable(true);
        return;
      }
      setConnectionError(mapConnectionError(error, t));
    }
  };

  const handleDeleteConnection = async () => {
    setConnectionError(null);
    try {
      await deleteConnection.mutateAsync();
      setDeleteDialogOpen(false);
      toast.success(t(($) => $.sso.connection_deleted_toast));
    } catch (error) {
      setConnectionError(mapConnectionError(error, t));
    }
  };

  const domainInputId = `${baseId}-domain`;
  const issuerId = `${baseId}-issuer`;
  const clientIdInputId = `${baseId}-client-id`;
  const clientSecretId = `${baseId}-client-secret`;
  const defaultRoleId = `${baseId}-default-role`;
  const statusId = `${baseId}-status`;

  const canSubmitConnection =
    !planBlocked &&
    hasVerifiedDomain &&
    !!issuer.trim() &&
    !!clientId.trim() &&
    !saveConnection.isPending;

  return (
    <SettingsTab title={t(($) => $.sso.title)} scope="workspace">
      {readOnly ? (
        <SettingsReadOnlyNotice wsId={wsId} />
      ) : memberLoading || domainsQuery.isLoading || connectionQuery.isLoading ? (
        <SettingsCard>
          <div className="space-y-4 p-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        </SettingsCard>
      ) : (
        <>
          {planBlocked ? (
            <Alert variant="destructive">
              <AlertCircle aria-hidden="true" />
              <AlertDescription>
                {t(($) => $.sso.errors.plan_unavailable)}
              </AlertDescription>
            </Alert>
          ) : null}

          <SettingsSection
            title={t(($) => $.sso.domains_title)}
            anchor="domains"
          >
            <SettingsCard>
              <form
                noValidate
                onSubmit={handleAddDomain}
                className="flex flex-col gap-2 px-4 py-3.5 sm:flex-row sm:items-center"
              >
                <Label htmlFor={domainInputId} className="sr-only">
                  {t(($) => $.sso.domain_label)}
                </Label>
                <Input
                  id={domainInputId}
                  type="text"
                  autoComplete="off"
                  spellCheck={false}
                  disabled={planBlocked || addDomain.isPending}
                  value={domainInput}
                  placeholder={t(($) => $.sso.domain_placeholder)}
                  onChange={(event) => {
                    setDomainInput(event.currentTarget.value);
                    if (domainError) setDomainError(null);
                  }}
                  className="flex-1"
                />
                <Button
                  type="submit"
                  disabled={
                    planBlocked || addDomain.isPending || !domainInput.trim()
                  }
                  aria-busy={addDomain.isPending || undefined}
                >
                  {addDomain.isPending ? (
                    <Loader2 className="animate-spin motion-reduce:animate-none" />
                  ) : null}
                  {addDomain.isPending
                    ? t(($) => $.sso.adding_domain)
                    : t(($) => $.sso.add_domain)}
                </Button>
              </form>

              {domainError ? (
                <div className="px-4 py-3">
                  <Alert variant="destructive">
                    <AlertCircle aria-hidden="true" />
                    <AlertDescription>{domainError}</AlertDescription>
                  </Alert>
                </div>
              ) : null}

              {domains.length === 0 ? (
                <div className="px-4 py-3.5 text-caption text-muted-foreground">
                  {t(($) => $.sso.empty_domains)}
                </div>
              ) : (
                domains.map((dom) => {
                  const isVerified = dom.verified_at !== null;
                  const isVerifying = verifyingDomain === dom.domain;
                  const isRemoving = removingDomain === dom.domain;
                  return (
                    <div key={dom.domain} className="space-y-3 px-4 py-3.5">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="flex min-w-0 items-center gap-2">
                          <span className="truncate font-mono text-body font-medium">
                            {dom.domain}
                          </span>
                          <Badge variant={isVerified ? "default" : "secondary"}>
                            {isVerified
                              ? t(($) => $.sso.verified_badge)
                              : t(($) => $.sso.pending_badge)}
                          </Badge>
                        </div>
                        <div className="flex items-center gap-2">
                          {!isVerified ? (
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              disabled={planBlocked || isVerifying || isRemoving}
                              aria-busy={isVerifying || undefined}
                              onClick={() => void handleVerifyDomain(dom.domain)}
                            >
                              {isVerifying ? (
                                <Loader2 className="animate-spin motion-reduce:animate-none" />
                              ) : null}
                              {isVerifying
                                ? t(($) => $.sso.verifying)
                                : t(($) => $.sso.verify)}
                            </Button>
                          ) : null}
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            disabled={isVerifying || isRemoving}
                            aria-busy={isRemoving || undefined}
                            onClick={() => void handleRemoveDomain(dom.domain)}
                          >
                            {isRemoving
                              ? t(($) => $.sso.removing)
                              : t(($) => $.sso.remove)}
                          </Button>
                        </div>
                      </div>

                      {!isVerified ? (
                        <div className="grid gap-2 rounded-lg border border-surface-border bg-muted/30 p-3 text-caption">
                          <div className="flex items-center justify-between gap-2">
                            <div className="min-w-0">
                              <div className="text-muted-foreground">
                                {t(($) => $.sso.txt_name_label)}
                              </div>
                              <code className="block truncate font-mono text-foreground select-all">
                                {dom.txt_name}
                              </code>
                            </div>
                            <Button
                              type="button"
                              variant="outline"
                              size="icon-xs"
                              aria-label={t(($) => $.sso.copy_txt_name)}
                              onClick={() =>
                                void handleCopy(
                                  `txt-name-${dom.domain}`,
                                  dom.txt_name,
                                )
                              }
                            >
                              {copiedKey === `txt-name-${dom.domain}` ? (
                                <Check className="size-3.5" />
                              ) : (
                                <Copy className="size-3.5" />
                              )}
                            </Button>
                          </div>
                          <div className="flex items-center justify-between gap-2">
                            <div className="min-w-0">
                              <div className="text-muted-foreground">
                                {t(($) => $.sso.txt_value_label)}
                              </div>
                              <code className="block truncate font-mono text-foreground select-all">
                                {dom.txt_value}
                              </code>
                            </div>
                            <Button
                              type="button"
                              variant="outline"
                              size="icon-xs"
                              aria-label={t(($) => $.sso.copy_txt_value)}
                              onClick={() =>
                                void handleCopy(
                                  `txt-value-${dom.domain}`,
                                  dom.txt_value,
                                )
                              }
                            >
                              {copiedKey === `txt-value-${dom.domain}` ? (
                                <Check className="size-3.5" />
                              ) : (
                                <Copy className="size-3.5" />
                              )}
                            </Button>
                          </div>
                        </div>
                      ) : null}
                    </div>
                  );
                })
              )}
            </SettingsCard>
          </SettingsSection>

          <SettingsSection
            title={t(($) => $.sso.connection_title)}
            anchor="connection"
          >
            {!hasVerifiedDomain ? (
              <Alert role="note">
                <AlertCircle aria-hidden="true" />
                <AlertDescription>
                  {t(($) => $.sso.verify_domain_gate)}
                </AlertDescription>
              </Alert>
            ) : null}

            {connectionError ? (
              <Alert variant="destructive">
                <AlertCircle aria-hidden="true" />
                <AlertDescription>{connectionError}</AlertDescription>
              </Alert>
            ) : null}

            <form noValidate onSubmit={handleSaveConnection}>
              <SettingsCard>
                <SettingsRow
                  anchor="redirect-uri"
                  label={t(($) => $.sso.redirect_uri_label)}
                  size="text"
                >
                  {connection?.redirect_uri ? (
                    <div className="flex min-w-0 items-center gap-2">
                      <code className="min-w-0 flex-1 truncate rounded-md border bg-muted/50 px-3 py-1.5 font-mono text-caption select-all">
                        {connection.redirect_uri}
                      </code>
                      <Button
                        type="button"
                        variant="outline"
                        size="icon-sm"
                        aria-label={t(($) => $.sso.copy_redirect_uri)}
                        onClick={() =>
                          void handleCopy(
                            "redirect-uri",
                            connection.redirect_uri,
                          )
                        }
                      >
                        {copiedKey === "redirect-uri" ? (
                          <Check className="size-3.5" />
                        ) : (
                          <Copy className="size-3.5" />
                        )}
                      </Button>
                    </div>
                  ) : (
                    <span className="text-caption text-muted-foreground">
                      {t(($) => $.sso.redirect_uri_pending)}
                    </span>
                  )}
                </SettingsRow>

                <SettingsRow
                  anchor="issuer"
                  label={
                    <Label htmlFor={issuerId}>
                      {t(($) => $.sso.issuer_label)}
                    </Label>
                  }
                  size="text"
                >
                  <Input
                    id={issuerId}
                    type="url"
                    autoComplete="off"
                    value={issuer}
                    disabled={planBlocked || saveConnection.isPending}
                    placeholder={t(($) => $.sso.issuer_placeholder)}
                    onChange={(event) => setIssuer(event.currentTarget.value)}
                  />
                </SettingsRow>

                <SettingsRow
                  anchor="client-id"
                  label={
                    <Label htmlFor={clientIdInputId}>
                      {t(($) => $.sso.client_id_label)}
                    </Label>
                  }
                  size="text"
                >
                  <Input
                    id={clientIdInputId}
                    type="text"
                    autoComplete="off"
                    value={clientId}
                    disabled={planBlocked || saveConnection.isPending}
                    placeholder={t(($) => $.sso.client_id_placeholder)}
                    onChange={(event) =>
                      setClientId(event.currentTarget.value)
                    }
                  />
                </SettingsRow>

                <SettingsRow
                  anchor="client-secret"
                  label={
                    <span className="inline-flex items-center gap-2">
                      <Label htmlFor={clientSecretId}>
                        {t(($) => $.sso.client_secret_label)}
                      </Label>
                      {connection?.client_secret_set ? (
                        <Badge variant="secondary">
                          {t(($) => $.sso.client_secret_stored)}
                        </Badge>
                      ) : null}
                    </span>
                  }
                  size="text"
                >
                  <Input
                    id={clientSecretId}
                    type="password"
                    autoComplete="new-password"
                    value={clientSecret}
                    disabled={planBlocked || saveConnection.isPending}
                    placeholder={
                      connection?.client_secret_set
                        ? t(($) => $.sso.client_secret_placeholder_replace)
                        : t(($) => $.sso.client_secret_placeholder_new)
                    }
                    onChange={(event) =>
                      setClientSecret(event.currentTarget.value)
                    }
                  />
                </SettingsRow>

                <SettingsRow
                  anchor="default-role"
                  label={
                    <Label htmlFor={defaultRoleId}>
                      {t(($) => $.sso.default_role_label)}
                    </Label>
                  }
                  size="select"
                >
                  <Select
                    items={roleItems}
                    value={defaultRole}
                    disabled={planBlocked || saveConnection.isPending}
                    onValueChange={(value) => {
                      if (value) setDefaultRole(value);
                    }}
                  >
                    <SelectTrigger
                      id={defaultRoleId}
                      aria-label={t(($) => $.sso.default_role_label)}
                      className="w-full"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {roleItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </SettingsRow>

                <SettingsRow
                  anchor="status"
                  label={
                    <Label htmlFor={statusId}>
                      {t(($) => $.sso.status_label)}
                    </Label>
                  }
                  size="select"
                >
                  <Select
                    items={statusItems}
                    value={status}
                    disabled={planBlocked || saveConnection.isPending}
                    onValueChange={(value) => {
                      if (value) setStatus(value);
                    }}
                  >
                    <SelectTrigger
                      id={statusId}
                      aria-label={t(($) => $.sso.status_label)}
                      className="w-full"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {statusItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </SettingsRow>

                <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3.5">
                  <div>
                    {connection ? (
                      <Button
                        type="button"
                        variant="destructive"
                        size="sm"
                        disabled={deleteConnection.isPending}
                        onClick={() => setDeleteDialogOpen(true)}
                      >
                        {t(($) => $.sso.delete_connection)}
                      </Button>
                    ) : null}
                  </div>
                  <Button
                    type="submit"
                    disabled={!canSubmitConnection}
                    aria-busy={saveConnection.isPending || undefined}
                  >
                    {saveConnection.isPending ? (
                      <Loader2 className="animate-spin motion-reduce:animate-none" />
                    ) : null}
                    {saveConnection.isPending
                      ? t(($) => $.sso.saving_connection)
                      : t(($) => $.sso.save_connection)}
                  </Button>
                </div>
              </SettingsCard>
            </form>
          </SettingsSection>
        </>
      )}

      <AlertDialog
        open={deleteDialogOpen}
        onOpenChange={(open) => {
          if (!open && !deleteConnection.isPending) setDeleteDialogOpen(false);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t(($) => $.sso.delete_dialog_title)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.sso.delete_dialog_description)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteConnection.isPending}>
              {t(($) => $.sso.delete_dialog_cancel)}
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleteConnection.isPending}
              onClick={handleDeleteConnection}
            >
              {deleteConnection.isPending
                ? t(($) => $.sso.deleting_connection)
                : t(($) => $.sso.delete_dialog_confirm)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsTab>
  );
}
