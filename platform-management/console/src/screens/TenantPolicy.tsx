import { useEffect, useId, useState, type FormEvent } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Field } from "@astryxdesign/core/Field";
import { Heading } from "@astryxdesign/core/Heading";
import { HStack, Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
import { MetadataList, MetadataListItem } from "@astryxdesign/core/MetadataList";
import { Section } from "@astryxdesign/core/Section";
import { Text } from "@astryxdesign/core/Text";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
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
import { Switch } from "@multica/ui/components/ui/switch";
import { AlertCircle, CheckCircle2, RefreshCw } from "@multica/ui/icons";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { getTenantPolicyErrorMessage } from "../api/client";
import { useMe, useTenantPolicy, useTenants, useUpdateTenantPolicy } from "../api/hooks";
import type { TenantPolicyResponse, TenantPolicyValues } from "../api/schemas";
import type { StaffRole } from "../components/types";

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
  const shareLinksId = useId();
  const shareLinksLabelId = useId();
  const requireSsoId = useId();
  const requireSsoLabelId = useId();
  const requireMfaId = useId();
  const requireMfaLabelId = useId();
  const sessionId = useId();
  const sessionErrorId = useId();

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
      <div role="region" aria-label="Not found">
        <ErrorState
          title="Not found"
          message="The page you requested was not found or you do not have permission to view it."
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
      setSessionError("Session max age must be a non-negative integer in hours.");
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
        session_max_age_hours: parsedHours,
      });
      setSaveSuccess(true);
    } catch (err) {
      setSaveError(getTenantPolicyErrorMessage(err, "Failed to update tenant policy"));
    }
  };

  const fromTenantBadge = (field: PolicyFieldKey) =>
    data && isFieldFromTenant(data, field) ? <Badge variant="info" label="From tenant" /> : null;

  const content = (
    <div className="space-y-6">
      {needsTenantSelector && (
        <Section padding={4}>
          <div className="space-y-3">
            <Heading level={3}>Tenant</Heading>
            <div className="max-w-sm">
              <UiField>
                <UiFieldLabel htmlFor="tenant-policy-tenant-select" className="text-label">
                  Tenant
                </UiFieldLabel>
                <Select
                  items={tenantOptions.map((item) => ({ value: item.id, label: item.name }))}
                  value={activeTenantId}
                  onValueChange={(val) => {
                    if (val) {
                      setSelectedTenantId(val);
                      setSaveSuccess(false);
                    }
                  }}
                >
                  <SelectTrigger id="tenant-policy-tenant-select" aria-label="Tenant" className="w-full">
                    <SelectValue>
                      {tenantOptions.find((item) => item.id === activeTenantId)?.name ||
                        "Select a tenant"}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {tenantOptions.map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        {item.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </UiField>
            </div>
          </div>
        </Section>
      )}

      {policyQuery.isLoading && <LoadingState message="Loading tenant policy..." />}

      {policyQuery.isError && (
        <ErrorState
          title="Failed to load tenant policy"
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
                <Heading level={3}>Effective policy</Heading>
                <Badge variant="neutral" label={`Version ${data.version}`} />
              </HStack>
              <MetadataList columns="multi" label={{ position: "start" }}>
                <MetadataListItem label="Allowed invite domains">
                  <HStack
                    data-testid="effective-policy-allowed_invite_domains"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body">
                      {effectivePolicy.allowed_invite_domains.length > 0
                        ? effectivePolicy.allowed_invite_domains.join(", ")
                        : "Any domain"}
                    </Text>
                    {fromTenantBadge("allowed_invite_domains")}
                  </HStack>
                </MetadataListItem>

                <MetadataListItem label="Share links">
                  <HStack
                    data-testid="effective-policy-share_links_enabled"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body">
                      {effectivePolicy.share_links_enabled ? "Enabled" : "Disabled"}
                    </Text>
                    {fromTenantBadge("share_links_enabled")}
                  </HStack>
                </MetadataListItem>

                <MetadataListItem label="Require SSO">
                  <HStack
                    data-testid="effective-policy-require_sso"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body">{effectivePolicy.require_sso ? "Required" : "Optional"}</Text>
                    {fromTenantBadge("require_sso")}
                  </HStack>
                </MetadataListItem>

                <MetadataListItem label="Require MFA">
                  <HStack
                    data-testid="effective-policy-require_mfa"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body">{effectivePolicy.require_mfa ? "Required" : "Optional"}</Text>
                    {fromTenantBadge("require_mfa")}
                  </HStack>
                </MetadataListItem>

                <MetadataListItem label="Session max age">
                  <HStack
                    data-testid="effective-policy-session_max_age_hours"
                    align="center"
                    gap={2}
                    wrap="wrap"
                  >
                    <Text type="body" hasTabularNumbers>
                      {effectivePolicy.session_max_age_hours > 0
                        ? `${effectivePolicy.session_max_age_hours} hours`
                        : "Unlimited"}
                    </Text>
                    {fromTenantBadge("session_max_age_hours")}
                  </HStack>
                </MetadataListItem>
              </MetadataList>
            </div>
          </Section>

          <Section padding={4}>
            <div className="space-y-4">
              <Heading level={3}>Tenant policy floor</Heading>
              <form onSubmit={handleSubmit} className="space-y-4">
              {saveError && (
                <Alert variant="destructive">
                  <AlertCircle className="size-4" />
                  <AlertDescription>{saveError}</AlertDescription>
                </Alert>
              )}

              {saveSuccess && (
                <Alert>
                  <CheckCircle2 className="size-4" />
                  <AlertDescription>Tenant policy updated.</AlertDescription>
                </Alert>
              )}

              <Field
                label="Allowed invite domains"
                inputID={domainsId}
                isOptional
                description="Comma-separated email domains enforced as the tenant intersection."
                width="100%"
              >
                <Input
                  id={domainsId}
                  value={domainsInput}
                  onChange={(e) => {
                    setDomainsInput(e.target.value);
                    setSaveSuccess(false);
                  }}
                  placeholder="acme.com, partner.org"
                  disabled={!canEdit || updatePolicy.isPending}
                />
              </Field>

              <div className="space-y-3">
                <div className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
                  <div className="min-w-0">
                    <label
                      id={shareLinksLabelId}
                      htmlFor={shareLinksId}
                      className="text-body font-medium text-foreground"
                    >
                      Allow share links
                    </label>
                    <Text type="supporting" as="div">
                      Workspaces can only enable public share links when the tenant allows them.
                    </Text>
                  </div>
                  <Switch
                    id={shareLinksId}
                    aria-labelledby={shareLinksLabelId}
                    checked={shareLinksEnabled}
                    onCheckedChange={(checked) => {
                      setShareLinksEnabled(checked);
                      setSaveSuccess(false);
                    }}
                    disabled={!canEdit || updatePolicy.isPending}
                  />
                </div>

                <div className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
                  <div className="min-w-0">
                    <label
                      id={requireSsoLabelId}
                      htmlFor={requireSsoId}
                      className="text-body font-medium text-foreground"
                    >
                      Require SSO
                    </label>
                  </div>
                  <Switch
                    id={requireSsoId}
                    aria-labelledby={requireSsoLabelId}
                    checked={requireSso}
                    onCheckedChange={(checked) => {
                      setRequireSso(checked);
                      setSaveSuccess(false);
                    }}
                    disabled={!canEdit || updatePolicy.isPending}
                  />
                </div>

                <div className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
                  <div className="min-w-0">
                    <label
                      id={requireMfaLabelId}
                      htmlFor={requireMfaId}
                      className="text-body font-medium text-foreground"
                    >
                      Require MFA
                    </label>
                  </div>
                  <Switch
                    id={requireMfaId}
                    aria-labelledby={requireMfaLabelId}
                    checked={requireMfa}
                    onCheckedChange={(checked) => {
                      setRequireMfa(checked);
                      setSaveSuccess(false);
                    }}
                    disabled={!canEdit || updatePolicy.isPending}
                  />
                </div>
              </div>

              <Field
                label="Session max age (hours)"
                inputID={sessionId}
                isOptional
                description="Set 0 to leave session lifetime unrestricted by the tenant floor."
                status={
                  sessionError
                    ? { type: "error", message: sessionError, messageID: sessionErrorId }
                    : undefined
                }
                width="100%"
              >
                <Input
                  id={sessionId}
                  type="number"
                  min={0}
                  step={1}
                  value={sessionMaxAgeInput}
                  onChange={(e) => {
                    setSessionMaxAgeInput(e.target.value);
                    setSessionError(null);
                    setSaveSuccess(false);
                  }}
                  aria-describedby={sessionError ? sessionErrorId : undefined}
                  aria-invalid={sessionError ? true : undefined}
                  disabled={!canEdit || updatePolicy.isPending}
                />
              </Field>

              {canEdit && (
                <HStack justify="end">
                  <Button
                    type="submit"
                    variant="default"
                    disabled={updatePolicy.isPending}
                    aria-busy={updatePolicy.isPending || undefined}
                  >
                    {updatePolicy.isPending && <Spinner />}
                    Save policy
                  </Button>
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
      <LayoutHeader hasDivider>
        <HStack justify="between" align="center" wrap="wrap" gap={3}>
          <Heading level={1}>Tenant policy</Heading>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => policyQuery.refetch()}
            disabled={policyQuery.isFetching || !activeTenantId}
            aria-busy={policyQuery.isFetching || undefined}
          >
            {policyQuery.isFetching ? (
              <Spinner />
            ) : (
              <RefreshCw className="size-4" aria-hidden="true" />
            )}
            Refresh
          </Button>
        </HStack>
      </LayoutHeader>
      <LayoutContent>{content}</LayoutContent>
    </Layout>
  );
}
