"use client";

import { useEffect, useId, useMemo, useState, type FormEvent } from "react";
import { AlertCircle, Loader2, Lock, Plus, X } from "@multica/ui/icons";
import { ApiError, clientErrorMessage, errorCode } from "@multica/core/api";
import { useCurrentWorkspace } from "@multica/core/paths";
import { useCurrentMember } from "@multica/core/permissions";
import {
  useSSODomains,
  useUpdateWorkspacePolicy,
  useWorkspacePolicy,
} from "@multica/core/security";
import type { PolicyFloor } from "@multica/core/types";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Switch } from "@multica/ui/components/ui/switch";
import { toast } from "sonner";
import { useT } from "../../i18n";
import {
  SettingsCard,
  SettingsReadOnlyNotice,
  SettingsRow,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";

const MAX_SESSION_AGE_HOURS = 8760;

const DEFAULT_POLICY = {
  allowed_invite_domains: [] as string[],
  share_links_enabled: true,
  require_sso: false,
  require_mfa: false,
  session_max_age_hours: 0,
  floor: null as PolicyFloor | null,
};

function mergeFloorDomains(
  floorDomains: readonly string[],
  workspaceDomains: readonly string[],
): string[] {
  if (floorDomains.length === 0) return [...workspaceDomains];
  if (workspaceDomains.length === 0) return [...floorDomains];
  const narrowed = workspaceDomains.filter((d) => floorDomains.includes(d));
  return narrowed.length > 0 ? narrowed : [...floorDomains];
}

function mergeFloorSessionAge(
  floorHours: number,
  workspaceHours: number,
): number {
  if (floorHours <= 0) return workspaceHours;
  if (workspaceHours <= 0) return floorHours;
  return Math.min(floorHours, workspaceHours);
}

function resolveEffectiveBaseline(policy: {
  allowed_invite_domains: string[];
  share_links_enabled: boolean;
  require_sso: boolean;
  require_mfa: boolean;
  session_max_age_hours: number;
  floor?: PolicyFloor | null;
}) {
  const floor = policy.floor ?? null;
  if (!floor) {
    return {
      allowed_invite_domains: policy.allowed_invite_domains,
      share_links_enabled: policy.share_links_enabled,
      require_sso: policy.require_sso,
      require_mfa: policy.require_mfa,
      session_max_age_hours: policy.session_max_age_hours,
    };
  }
  return {
    allowed_invite_domains: mergeFloorDomains(
      floor.allowed_invite_domains,
      policy.allowed_invite_domains,
    ),
    share_links_enabled: policy.share_links_enabled && floor.share_links_enabled,
    require_sso: policy.require_sso || floor.require_sso,
    require_mfa: policy.require_mfa || floor.require_mfa,
    session_max_age_hours: mergeFloorSessionAge(
      floor.session_max_age_hours,
      policy.session_max_age_hours,
    ),
  };
}

function TenantFloorNotice({
  badge,
  reason,
}: {
  badge: string;
  reason: string;
}) {
  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-caption text-muted-foreground">
      <Badge variant="secondary">
        <Lock className="size-3" aria-hidden="true" />
        {badge}
      </Badge>
      <span>{reason}</span>
    </div>
  );
}

function isPlanUnavailableError(error: unknown): boolean {
  return (
    errorCode(error) === "plan_feature_unavailable" ||
    (error instanceof ApiError && error.status === 402)
  );
}

function stringArraysEqual(a: readonly string[], b: readonly string[]): boolean {
  if (a.length !== b.length) return false;
  return a.every((item, idx) => item === b[idx]);
}

function parseSessionAgeHours(raw: string): number | null {
  const trimmed = raw.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const parsed = Number(trimmed);
  if (!Number.isSafeInteger(parsed) || parsed < 0 || parsed > MAX_SESSION_AGE_HOURS) {
    return null;
  }
  return parsed;
}

function mapPolicySaveError(
  error: unknown,
  t: ReturnType<typeof useT<"settings">>["t"],
): string {
  switch (errorCode(error)) {
    case "plan_feature_unavailable":
      return t(($) => $.policy.errors.plan_unavailable);
    case "domain_not_verified":
      return t(($) => $.policy.errors.domain_not_verified);
    case "invite_domain_not_allowed":
      return t(($) => $.policy.errors.invite_domain_not_allowed);
    case "share_links_disabled":
      return t(($) => $.policy.errors.share_links_disabled);
    case "policy_below_tenant_floor": {
      const bodyError =
        error instanceof ApiError &&
        error.body &&
        typeof error.body === "object" &&
        "error" in error.body &&
        typeof (error.body as { error?: unknown }).error === "string"
          ? (error.body as { error: string }).error
          : undefined;
      return (
        clientErrorMessage(error) ??
        bodyError ??
        t(($) => $.policy.errors.below_tenant_floor)
      );
    }
    case "viewer_read_only":
      return t(($) => $.policy.errors.forbidden);
    default:
      if (error instanceof ApiError && error.status === 402) {
        return t(($) => $.policy.errors.plan_unavailable);
      }
      if (error instanceof ApiError && error.status === 403) {
        return t(($) => $.policy.errors.forbidden);
      }
      return t(($) => $.policy.errors.save_failed);
  }
}

export function PolicyTab() {
  const { t } = useT("settings");
  const baseId = useId();
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id ?? "";
  const { role, isLoading: memberLoading } = useCurrentMember(wsId);

  const isOwner = role === "owner";
  const canReadPolicy = role === "owner" || role === "admin";
  const readOnly = !memberLoading && !isOwner;

  const policyQuery = useWorkspacePolicy(canReadPolicy ? wsId : "");
  const domainsQuery = useSSODomains(isOwner ? wsId : "");
  const updatePolicy = useUpdateWorkspacePolicy(wsId);

  const serverPolicy = policyQuery.data ?? DEFAULT_POLICY;
  const floor = serverPolicy.floor ?? null;
  const effectiveBaseline = useMemo(
    () => resolveEffectiveBaseline(serverPolicy),
    [serverPolicy],
  );

  const [allowedDomains, setAllowedDomains] = useState<string[]>(
    effectiveBaseline.allowed_invite_domains,
  );
  const [shareLinksEnabled, setShareLinksEnabled] = useState(
    effectiveBaseline.share_links_enabled,
  );
  const [requireMfa, setRequireMfa] = useState(effectiveBaseline.require_mfa);
  const [requireSso, setRequireSso] = useState(effectiveBaseline.require_sso);
  const [sessionMaxAgeInput, setSessionMaxAgeInput] = useState(
    String(effectiveBaseline.session_max_age_hours),
  );
  const [domainDraft, setDomainDraft] = useState("");
  const [domainError, setDomainError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [planUnavailable, setPlanUnavailable] = useState(false);

  useEffect(() => {
    setAllowedDomains(effectiveBaseline.allowed_invite_domains);
    setShareLinksEnabled(effectiveBaseline.share_links_enabled);
    setRequireMfa(effectiveBaseline.require_mfa);
    setRequireSso(effectiveBaseline.require_sso);
    setSessionMaxAgeInput(String(effectiveBaseline.session_max_age_hours));
  }, [
    effectiveBaseline.allowed_invite_domains,
    effectiveBaseline.require_mfa,
    effectiveBaseline.require_sso,
    effectiveBaseline.session_max_age_hours,
    effectiveBaseline.share_links_enabled,
  ]);

  useEffect(() => {
    setDomainDraft("");
    setDomainError(null);
    setSaveError(null);
    setPlanUnavailable(false);
  }, [wsId]);

  const floorDomains = useMemo(
    () => floor?.allowed_invite_domains ?? [],
    [floor?.allowed_invite_domains],
  );
  const domainsFloorConstrained = floorDomains.length > 0;
  const domainsFloorLocked = floorDomains.length === 1;
  const shareLinksFloorLocked = floor != null && !floor.share_links_enabled;
  const requireMfaFloorLocked = floor != null && floor.require_mfa;
  const requireSsoFloorLocked = floor != null && floor.require_sso;
  const floorSessionMaxAge = floor?.session_max_age_hours ?? 0;
  const sessionAgeFloorConstrained = floorSessionMaxAge > 0;
  const sessionAgeFloorLocked = floorSessionMaxAge === 1;

  const verifiedDomains = useMemo(
    () =>
      (domainsQuery.data?.domains ?? [])
        .filter((d) => d.verified_at !== null)
        .map((d) => d.domain.toLowerCase()),
    [domainsQuery.data?.domains],
  );
  const availableVerifiedDomains = useMemo(
    () =>
      verifiedDomains.filter(
        (d) =>
          !allowedDomains.includes(d) &&
          (!domainsFloorConstrained || floorDomains.includes(d)),
      ),
    [allowedDomains, domainsFloorConstrained, floorDomains, verifiedDomains],
  );

  if (!workspace) return null;

  const parsedSessionAge = parseSessionAgeHours(sessionMaxAgeInput);
  const sessionAgeBelowFloor =
    sessionAgeFloorConstrained &&
    parsedSessionAge !== null &&
    (parsedSessionAge === 0 || parsedSessionAge > floorSessionMaxAge);
  const sessionAgeInvalid = parsedSessionAge === null || sessionAgeBelowFloor;

  const hasChanges =
    !stringArraysEqual(allowedDomains, effectiveBaseline.allowed_invite_domains) ||
    shareLinksEnabled !== effectiveBaseline.share_links_enabled ||
    requireMfa !== effectiveBaseline.require_mfa ||
    requireSso !== effectiveBaseline.require_sso ||
    (parsedSessionAge !== null &&
      parsedSessionAge !== effectiveBaseline.session_max_age_hours);

  const planBlocked =
    planUnavailable || isPlanUnavailableError(policyQuery.error);
  const controlsDisabled =
    readOnly || planBlocked || updatePolicy.isPending;

  const handleAddDomain = (rawDomain: string) => {
    const normalized = rawDomain.trim().toLowerCase();
    if (!normalized) return;
    if (domainsFloorConstrained && !floorDomains.includes(normalized)) {
      setDomainError(t(($) => $.policy.floor.domain_outside_floor));
      return;
    }
    if (allowedDomains.includes(normalized)) {
      setDomainError(
        t(($) => $.policy.allowed_domains.duplicate_error, {
          domain: normalized,
        }),
      );
      return;
    }
    if (!verifiedDomains.includes(normalized)) {
      setDomainError(
        t(($) => $.policy.allowed_domains.unverified_error, {
          domain: normalized,
        }),
      );
      return;
    }
    setAllowedDomains((prev) => [...prev, normalized]);
    setDomainDraft("");
    setDomainError(null);
  };

  const handleRemoveDomain = (domain: string) => {
    if (
      domainsFloorLocked ||
      (domainsFloorConstrained && allowedDomains.length <= 1)
    ) {
      return;
    }
    setAllowedDomains((prev) => prev.filter((item) => item !== domain));
    setDomainError(null);
  };

  const handleSave = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (
      readOnly ||
      planBlocked ||
      parsedSessionAge === null ||
      sessionAgeBelowFloor ||
      !hasChanges
    ) {
      return;
    }

    const patch: Parameters<typeof updatePolicy.mutateAsync>[0] = {};
    if (!stringArraysEqual(allowedDomains, serverPolicy.allowed_invite_domains)) {
      patch.allowed_invite_domains = allowedDomains;
    }
    if (shareLinksEnabled !== serverPolicy.share_links_enabled) {
      patch.share_links_enabled = shareLinksEnabled;
    }
    if (requireMfa !== serverPolicy.require_mfa) {
      patch.require_mfa = requireMfa;
    }
    if (requireSso !== serverPolicy.require_sso) {
      patch.require_sso = requireSso;
    }
    if (parsedSessionAge !== serverPolicy.session_max_age_hours) {
      patch.session_max_age_hours = parsedSessionAge;
    }

    setSaveError(null);
    try {
      await updatePolicy.mutateAsync(patch);
      toast.success(t(($) => $.policy.saved_toast));
    } catch (error) {
      if (isPlanUnavailableError(error)) {
        setPlanUnavailable(true);
      }
      setSaveError(mapPolicySaveError(error, t));
    }
  };

  const domainInputId = `${baseId}-domain-input`;
  const shareLinksId = `${baseId}-share-links`;
  const requireMfaId = `${baseId}-require-mfa`;
  const requireSsoId = `${baseId}-require-sso`;
  const sessionAgeId = `${baseId}-session-age`;

  const displayError =
    saveError ??
    (planBlocked
      ? t(($) => $.policy.errors.plan_unavailable)
      : null);

  return (
    <SettingsTab title={t(($) => $.policy.title)} scope="workspace">
      {readOnly ? <SettingsReadOnlyNotice wsId={wsId} /> : null}

      {memberLoading || (canReadPolicy && policyQuery.isLoading) ? (
        <SettingsCard>
          <div className="space-y-4 p-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        </SettingsCard>
      ) : canReadPolicy && policyQuery.isError && !planBlocked ? (
        <SettingsCard>
          <div className="flex items-center justify-between gap-4 px-4 py-3.5">
            <div className="flex items-center gap-2 text-body text-destructive">
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              <span>{t(($) => $.policy.load_failed)}</span>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void policyQuery.refetch()}
            >
              {t(($) => $.policy.retry)}
            </Button>
          </div>
        </SettingsCard>
      ) : (
        <SettingsSection>
          <form noValidate onSubmit={handleSave} className="space-y-4">
            {displayError ? (
              <Alert variant="destructive">
                <AlertCircle aria-hidden="true" />
                <AlertDescription>{displayError}</AlertDescription>
              </Alert>
            ) : null}

            <SettingsCard>
              <SettingsRow
                anchor="allowed-invite-domains"
                label={
                  <Label htmlFor={domainInputId}>
                    {t(($) => $.policy.allowed_domains.label)}
                  </Label>
                }
                description={
                  <>
                    <span>{t(($) => $.policy.allowed_domains.consequence)}</span>
                    {domainsFloorConstrained ? (
                      <TenantFloorNotice
                        badge={t(($) => $.policy.floor.badge)}
                        reason={t(($) => $.policy.floor.domains_reason, {
                          domains: floorDomains.join(", "),
                        })}
                      />
                    ) : null}
                  </>
                }
                size="text"
                align="start"
              >
                <div className="space-y-2.5">
                  {allowedDomains.length > 0 ? (
                    <ul className="space-y-1.5">
                      {allowedDomains.map((domain) => (
                        <li
                          key={domain}
                          className="flex items-center justify-between gap-2 rounded-lg border border-surface-border px-3 py-1.5"
                        >
                          <code className="min-w-0 truncate font-mono text-body">
                            {domain}
                          </code>
                          {!readOnly ? (
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon-xs"
                              disabled={
                                controlsDisabled ||
                                domainsFloorLocked ||
                                (domainsFloorConstrained &&
                                  allowedDomains.length <= 1)
                              }
                              aria-label={t(
                                ($) => $.policy.allowed_domains.remove_aria,
                                { domain },
                              )}
                              onClick={() => handleRemoveDomain(domain)}
                            >
                              <X className="size-3.5" />
                            </Button>
                          ) : null}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-caption text-muted-foreground">
                      {t(($) => $.policy.allowed_domains.empty)}
                    </p>
                  )}

                  {!readOnly ? (
                    <>
                      <div className="flex items-center gap-2">
                        <Input
                          id={domainInputId}
                          type="text"
                          autoComplete="off"
                          spellCheck={false}
                          value={domainDraft}
                          disabled={controlsDisabled || domainsFloorLocked}
                          aria-invalid={domainError !== null}
                          placeholder={t(
                            ($) => $.policy.allowed_domains.placeholder,
                          )}
                          onChange={(event) => {
                            setDomainDraft(event.currentTarget.value);
                            if (domainError) setDomainError(null);
                          }}
                          onKeyDown={(event) => {
                            if (event.key === "Enter") {
                              event.preventDefault();
                              handleAddDomain(domainDraft);
                            }
                          }}
                        />
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={
                            controlsDisabled ||
                            domainsFloorLocked ||
                            !domainDraft.trim()
                          }
                          onClick={() => handleAddDomain(domainDraft)}
                        >
                          {t(($) => $.policy.allowed_domains.add)}
                        </Button>
                      </div>

                      {domainError ? (
                        <p className="text-caption text-destructive">
                          {domainError}
                        </p>
                      ) : null}

                      {availableVerifiedDomains.length > 0 ? (
                        <div className="flex flex-wrap items-center gap-1.5">
                          <span className="text-caption text-muted-foreground">
                            {t(
                              ($) => $.policy.allowed_domains.verified_list_label,
                            )}
                          </span>
                          {availableVerifiedDomains.map((domain) => (
                            <Button
                              key={domain}
                              type="button"
                              variant="outline"
                              size="xs"
                              disabled={controlsDisabled || domainsFloorLocked}
                              onClick={() => handleAddDomain(domain)}
                            >
                              <Plus className="size-3" />
                              {domain}
                            </Button>
                          ))}
                        </div>
                      ) : verifiedDomains.length === 0 &&
                        !domainsQuery.isLoading &&
                        !domainsFloorLocked ? (
                        <p className="text-caption text-muted-foreground">
                          {t(($) => $.policy.allowed_domains.no_verified)}
                        </p>
                      ) : null}
                    </>
                  ) : null}
                </div>
              </SettingsRow>

              <SettingsRow
                anchor="share-links"
                label={
                  <Label htmlFor={shareLinksId}>
                    {t(($) => $.policy.share_links.label)}
                  </Label>
                }
                description={
                  <>
                    <span>{t(($) => $.policy.share_links.consequence)}</span>
                    {shareLinksFloorLocked ? (
                      <TenantFloorNotice
                        badge={t(($) => $.policy.floor.badge)}
                        reason={t(($) => $.policy.floor.locked_reason)}
                      />
                    ) : null}
                  </>
                }
              >
                <Switch
                  id={shareLinksId}
                  checked={shareLinksEnabled}
                  disabled={controlsDisabled || shareLinksFloorLocked}
                  aria-label={t(($) => $.policy.share_links.label)}
                  onCheckedChange={setShareLinksEnabled}
                />
              </SettingsRow>

              <SettingsRow
                anchor="require-mfa"
                label={
                  <Label htmlFor={requireMfaId}>
                    {t(($) => $.policy.require_mfa.label)}
                  </Label>
                }
                description={
                  <>
                    <span>{t(($) => $.policy.require_mfa.consequence)}</span>
                    {requireMfaFloorLocked ? (
                      <TenantFloorNotice
                        badge={t(($) => $.policy.floor.badge)}
                        reason={t(($) => $.policy.floor.locked_reason)}
                      />
                    ) : null}
                  </>
                }
              >
                <Switch
                  id={requireMfaId}
                  checked={requireMfa}
                  disabled={controlsDisabled || requireMfaFloorLocked}
                  aria-label={t(($) => $.policy.require_mfa.label)}
                  onCheckedChange={setRequireMfa}
                />
              </SettingsRow>

              <SettingsRow
                anchor="require-sso"
                label={
                  <Label htmlFor={requireSsoId}>
                    {t(($) => $.policy.require_sso.label)}
                  </Label>
                }
                description={
                  <>
                    <span>{t(($) => $.policy.require_sso.consequence)}</span>
                    {requireSsoFloorLocked ? (
                      <TenantFloorNotice
                        badge={t(($) => $.policy.floor.badge)}
                        reason={t(($) => $.policy.floor.locked_reason)}
                      />
                    ) : null}
                  </>
                }
              >
                <Switch
                  id={requireSsoId}
                  checked={requireSso}
                  disabled={controlsDisabled || requireSsoFloorLocked}
                  aria-label={t(($) => $.policy.require_sso.label)}
                  onCheckedChange={setRequireSso}
                />
              </SettingsRow>

              <SettingsRow
                anchor="session-max-age"
                label={
                  <Label htmlFor={sessionAgeId}>
                    {t(($) => $.policy.session_max_age.label)}
                  </Label>
                }
                description={
                  <>
                    <span>{t(($) => $.policy.session_max_age.consequence)}</span>
                    {sessionAgeFloorConstrained ? (
                      <TenantFloorNotice
                        badge={t(($) => $.policy.floor.badge)}
                        reason={t(($) => $.policy.floor.session_max_age_reason, {
                          hours: floorSessionMaxAge,
                        })}
                      />
                    ) : null}
                  </>
                }
                size="code"
              >
                <div className="space-y-1">
                  <Input
                    id={sessionAgeId}
                    type="number"
                    min={sessionAgeFloorConstrained ? 1 : 0}
                    max={
                      sessionAgeFloorConstrained
                        ? floorSessionMaxAge
                        : MAX_SESSION_AGE_HOURS
                    }
                    step={1}
                    value={sessionMaxAgeInput}
                    disabled={controlsDisabled || sessionAgeFloorLocked}
                    aria-invalid={sessionAgeInvalid}
                    onChange={(event) =>
                      setSessionMaxAgeInput(event.currentTarget.value)
                    }
                  />
                  {sessionAgeInvalid ? (
                    <p className="text-caption text-destructive">
                      {sessionAgeBelowFloor
                        ? t(($) => $.policy.floor.session_max_age_exceeded, {
                            hours: floorSessionMaxAge,
                          })
                        : t(($) => $.policy.session_max_age.invalid_range)}
                    </p>
                  ) : null}
                </div>
              </SettingsRow>

              {!readOnly ? (
                <div className="flex justify-end px-4 py-3.5">
                  <Button
                    type="submit"
                    disabled={
                      controlsDisabled || !hasChanges || sessionAgeInvalid
                    }
                    aria-busy={updatePolicy.isPending || undefined}
                  >
                    {updatePolicy.isPending ? (
                      <Loader2 className="animate-spin motion-reduce:animate-none" />
                    ) : null}
                    {updatePolicy.isPending
                      ? t(($) => $.policy.saving)
                      : t(($) => $.policy.save)}
                  </Button>
                </div>
              ) : null}
            </SettingsCard>
          </form>
        </SettingsSection>
      )}
    </SettingsTab>
  );
}
