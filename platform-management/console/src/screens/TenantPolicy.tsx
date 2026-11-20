import { Card } from "@astryxdesign/core/Card";
import { useEffect, useId, useState, type FormEvent } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Heading } from "@astryxdesign/core/Heading";
import { HStack, Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { Section } from "@astryxdesign/core/Section";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Switch } from "@astryxdesign/core/Switch";
import { NumberInput } from "@astryxdesign/core/NumberInput";
import { Selector } from "@astryxdesign/core/Selector";
import { RefreshCw } from "@multica/ui/icons";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { getTenantPolicyErrorMessage } from "../api/client";
import { useMe, useTenantPolicy, useTenants, useUpdateTenantPolicy } from "../api/hooks";
import type { TenantPolicyResponse, TenantPolicyValues } from "../api/schemas";
import type { StaffRole } from "../components/types";
import { t } from "../i18n";
import { PageHeader } from "../components/PageHeader";

export interface TenantPolicyProps {
  tenantId?: string;
  embedded?: boolean;
  userRole?: StaffRole;
}

type PolicyFieldKey = keyof TenantPolicyValues;

function parseDomainList(raw: string): string[] {
  return raw
    .split(/[,\s]+/)
    .map((item) => item.trim().toLowerCase())
    .filter(Boolean);
}

function extractTenantOptions(data: unknown): Array<{ id: string; name: string }> {
  if (Array.isArray(data)) {
    return data.filter(
      (item): item is { id: string; name: string } =>
        Boolean(item && typeof item === "object" && "id" in item && "name" in item),
    );
  }
  if (data && typeof data === "object" && "tenants" in data) {
    const list = (data as { tenants?: unknown }).tenants;
    if (Array.isArray(list)) {
      return list.filter(
        (item): item is { id: string; name: string } =>
          Boolean(item && typeof item === "object" && "id" in item && "name" in item),
      );
    }
  }
  return [];
}

function isFieldFromTenant(data: TenantPolicyResponse, field: PolicyFieldKey): boolean {
  const hasExplicitSources =
    Object.keys(data.sources ?? {}).length > 0 || (data.tenant_fields?.length ?? 0) > 0;
  if (hasExplicitSources) {
    if (data.sources?.[field]) {
      return data.sources[field] === "tenant";
    }
    return Boolean(data.tenant_fields?.includes(field));
  }

  switch (field) {
    case "allowed_invite_domains":
      return data.policy.allowed_invite_domains.length > 0;
    case "share_links_enabled":
      return !data.policy.share_links_enabled;
    case "require_sso":
      return data.policy.require_sso;
    case "require_mfa":
      return data.policy.require_mfa;
    case "session_max_age_hours":
      return data.policy.session_max_age_hours > 0;
  }
}

export function TenantPolicy({ tenantId, embedded = false, userRole }: TenantPolicyProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;

  const isBilling = effectiveRole === "billing";
  const isAdmin = effectiveRole === "admin";
  const isSupport = effectiveRole === "support";
  const isSystemAdmin = effectiveRole === "system_admin";

  const adminTenantMismatch = Boolean(
    isAdmin && tenantId && me?.tenant_id && tenantId !== me.tenant_id,
  );
  const adminMissingTenant = Boolean(isAdmin && !tenantId && !me?.tenant_id);
  const canAccess = !isBilling && !adminTenantMismatch && !adminMissingTenant;

  const needsTenantSelector = Boolean(!tenantId && (isSystemAdmin || isSupport || !effectiveRole));
  const { data: tenantsData } = useTenants();
  const tenantOptions = extractTenantOptions(tenantsData);
  const [selectedTenantId, setSelectedTenantId] = useState("");

  const activeTenantId =
    tenantId ??
    (isAdmin ? (me?.tenant_id ?? "") : selectedTenantId || tenantOptions[0]?.id || "");

  const policyQuery = useTenantPolicy(
    activeTenantId || undefined,
    canAccess && Boolean(activeTenantId),
  );
  const updatePolicy = useUpdateTenantPolicy();

  const [domainsInput, setDomainsInput] = useState("");
  const [shareLinksEnabled, setShareLinksEnabled] = useState(true);
  const [requireSso, setRequireSso] = useState(false);
  const [requireMfa, setRequireMfa] = useState(false);
  const [sessionMaxAgeInput, setSessionMaxAgeInput] = useState("0");
  const [sessionError, setSessionError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);

  const domainsId = useId();
  const sessionId = useId();

  useEffect(() => {
    const policy = policyQuery.data?.policy;
    if (!policy) return;
    setDomainsInput(policy.allowed_invite_domains.join(", "));
    setShareLinksEnabled(policy.share_links_enabled);
    setRequireSso(policy.require_sso);
    setRequireMfa(policy.require_mfa);
    setSessionMaxAgeInput(String(policy.session_max_age_hours));
    setSessionError(null);
    setSaveError(null);
  }, [policyQuery.data, activeTenantId]);

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

  const canEdit = !isSupport && (isSystemAdmin || isAdmin || !effectiveRole);
  const data = policyQuery.data;
  const effectivePolicy = data ? (data.effective_policy ?? data.policy) : null;

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!canEdit || !data || !activeTenantId) return;

    setSaveError(null);
    setSaveSuccess(false);
    setSessionError(null);

    const parsedHours = Number(sessionMaxAgeInput.trim() || "0");
    if (!Number.isInteger(parsedHours) || parsedHours < 0) {
      setSessionError(t("Session max age must be a non-negative integer in hours."));
      return;
    }

    try {
      await updatePolicy.mutateAsync({
        tenantId: activeTenantId,
        expected_version: data.version,
        allowed_invite_domains: parseDomainList(domainsInput),
        share_links_enabled: shareLinksEnabled,
        require_sso: requireSso,
        require_mfa: requireMfa,
        session_max_age_hours: parsedHours });
      setSaveSuccess(true);
    } catch (err) {
      setSaveError(getTenantPolicyErrorMessage(err, t("Failed to update tenant policy")));
    }
  };

  const fromTenantBadge = (field: PolicyFieldKey) =>
    data && isFieldFromTenant(data, field) ? <Badge variant="info" label={t("From tenant")} /> : null;

  const content = (
    <div className="space-y-6">
      {needsTenantSelector && (
        <Section padding={4}>
          <div className="space-y-3">
            <Heading level={3}>{t("Tenant")}</Heading>
            <div className="max-w-sm">
              <Selector
                label={t("Tenant")}
                isLabelHidden
                width="100%"
                options={tenantOptions.map((item) => ({ value: item.id, label: item.name }))}
                value={activeTenantId}
                onChange={(val) => {
                  if (val) {
                    setSelectedTenantId(val);
                    setSaveSuccess(false);
                  }
                }}
              />
            </div>
          </div>
        </Section>
      )}

      {policyQuery.isLoading && <LoadingState message={t("Loading tenant policy...")} />}

      {policyQuery.isError && (
        <ErrorState
          title={t("Failed to load tenant policy")}
          message={
            policyQuery.error instanceof Error ? policyQuery.error.message : undefined
          }
          onRetry={policyQuery.refetch}
        />
      )}

      {data && effectivePolicy && (
        <>
          <Section padding={4}>
            <div className="space-y-4">
              <HStack justify="between" align="center" wrap="wrap" gap={2}>
                <Heading level={3}>{t("Effective policy")}</Heading>
                <Badge variant="neutral" label={t("Version {0}", data.version)} />
              </HStack>
              <MetadataList columns={3}>
                <MetadataListItem label={t("Allowed invite domains")}>
                  <HStack
                    data-testid="effective-policy-allowed_invite_domains"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body">
                      {effectivePolicy.allowed_invite_domains.length > 0
                        ? effectivePolicy.allowed_invite_domains.join(", ")
                        : t("Any domain")}
                    </Text>
                    {fromTenantBadge("allowed_invite_domains")}
                  </HStack>
                </MetadataListItem>

                <MetadataListItem label={t("Share links")}>
                  <HStack
                    data-testid="effective-policy-share_links_enabled"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body">
                      {effectivePolicy.share_links_enabled ? t("Enabled") : t("Disabled")}
                    </Text>
                    {fromTenantBadge("share_links_enabled")}
                  </HStack>
                </MetadataListItem>

                <MetadataListItem label={t("Require SSO")}>
                  <HStack
                    data-testid="effective-policy-require_sso"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body">{effectivePolicy.require_sso ? t("Required") : t("Optional")}</Text>
                    {fromTenantBadge("require_sso")}
                  </HStack>
                </MetadataListItem>

                <MetadataListItem label={t("Require MFA")}>
                  <HStack
                    data-testid="effective-policy-require_mfa"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body">{effectivePolicy.require_mfa ? t("Required") : t("Optional")}</Text>
                    {fromTenantBadge("require_mfa")}
                  </HStack>
                </MetadataListItem>

                <MetadataListItem label={t("Session max age")}>
                  <HStack
                    data-testid="effective-policy-session_max_age_hours"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body" hasTabularNumbers>
                      {effectivePolicy.session_max_age_hours > 0
                        ? t("{0} hours", effectivePolicy.session_max_age_hours)
                        : t("Unlimited")}
                    </Text>
                    {fromTenantBadge("session_max_age_hours")}
                  </HStack>
                </MetadataListItem>
              </MetadataList>
            </div>
          </Section>

          <Section padding={4}>
            <div className="space-y-4">
              <Heading level={3}>{t("Tenant policy floor")}</Heading>
              <form onSubmit={handleSubmit} className="space-y-4">
              {saveError && (
                <Banner
                  status="error"
                  role="alert"
                  title={saveError}
                />
              )}

              {saveSuccess && (
                <Banner
                  status="info"
                  title={t("Tenant policy updated.")}
                />
              )}

              <TextInput
                label={t("Allowed invite domains")}
                id={domainsId}
                isOptional
                description={t("Comma-separated email domains enforced as the tenant intersection.")}
                width="100%"
                value={domainsInput}
                onChange={(value) => {
                    setDomainsInput(value);
                    setSaveSuccess(false);
                  }}
                placeholder={t("acme.com, partner.org")}
                isDisabled={!canEdit || updatePolicy.isPending}
              />

              <div className="space-y-3">
                <Card padding={3} elevation="none">
                  <Switch
                    label={t("Allow share links")}
                    description={t("Workspaces can only enable public share links when the tenant allows them.")}
                    labelPosition="start"
                    labelSpacing="spread"
                    width="100%"
                    value={shareLinksEnabled}
                    onChange={(checked) => {
                      setShareLinksEnabled(checked);
                      setSaveSuccess(false);
                    }}
                    isDisabled={!canEdit || updatePolicy.isPending}
                  />
                </Card>

                <Card padding={3} elevation="none">
                  <Switch
                    label={t("Require SSO")}
                    labelPosition="start"
                    labelSpacing="spread"
                    width="100%"
                    value={requireSso}
                    onChange={(checked) => {
                      setRequireSso(checked);
                      setSaveSuccess(false);
                    }}
                    isDisabled={!canEdit || updatePolicy.isPending}
                  />
                </Card>

                <Card padding={3} elevation="none">
                  <Switch
                    label={t("Require MFA")}
                    labelPosition="start"
                    labelSpacing="spread"
                    width="100%"
                    value={requireMfa}
                    onChange={(checked) => {
                      setRequireMfa(checked);
                      setSaveSuccess(false);
                    }}
                    isDisabled={!canEdit || updatePolicy.isPending}
                  />
                </Card>
              </div>

              <NumberInput
                label={t("Session max age (hours)")}
                id={sessionId}
                isOptional
                description={t("Set 0 to leave session lifetime unrestricted by the tenant floor.")}
                status={sessionError ? { type: "error", message: sessionError } : undefined}
                width="100%"
                min={0}
                step={1}
                isIntegerOnly
                value={sessionMaxAgeInput.trim() === "" ? null : Number(sessionMaxAgeInput)}
                onChange={(value) => {
                  setSessionMaxAgeInput(value === null ? "" : String(value));
                  setSessionError(null);
                  setSaveSuccess(false);
                }}
                isDisabled={!canEdit || updatePolicy.isPending}
              />

              {canEdit && (
                <HStack justify="end">
                  <Button
                    type="submit"
                    label={t("Save policy")}
                    variant="primary"
                    isLoading={updatePolicy.isPending}
                    aria-busy={updatePolicy.isPending || undefined}
                    isDisabled={updatePolicy.isPending}
                  />
                </HStack>
              )}
              </form>
            </div>
          </Section>
        </>
      )}
    </div>
  );

  if (embedded) {
    return content;
  }

  return (
    <Layout height="auto">
      <LayoutHeader padding={0}>
          <PageHeader
            title={t("Tenant policy")}
            actions={
              <>
                <Button
                  type="button"
                  label={t("Refresh")}
                  variant="secondary"
                  size="sm"
                  icon={<RefreshCw className="size-4" aria-hidden="true" />}
                  isLoading={policyQuery.isFetching}
                  onClick={() => policyQuery.refetch()}
                  aria-busy={policyQuery.isFetching || undefined}
                  isDisabled={policyQuery.isFetching || !activeTenantId}
                />
              </>
            }
          />
        </LayoutHeader>
      <LayoutContent padding={0}>{content}</LayoutContent>
    </Layout>
  );
}
