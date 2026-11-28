"use client";

import {
  serializeAttestationBody,
  useVerifyAttestationQuery,
} from "@multica/core/durable/attestations";
import { useWorkspaceId } from "@multica/core/hooks";
import { Badge } from "@multica/ui/components/ui/badge";
import { Loader2, ShieldCheck, ShieldOff } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { useReachEnabled } from "../reach/use-reach-enabled";

/**
 * Verifies a signed track-record attestation document through POST /api/attestations/verify
 * and displays Verified (with issuer host when present) or Unverified (durable spec 13.2).
 * Never displays Verified while a request is pending or on any error/malformed response.
 */
export function AttestationBadge({
  document,
  wsId,
  className,
}: {
  document: unknown;
  wsId?: string;
  className?: string;
}) {
  const enabled = useReachEnabled();
  if (!enabled) return null;
  if (wsId) {
    return <AttestationBadgeInner wsId={wsId} document={document} className={className} />;
  }
  return <AttestationBadgeWithWorkspace document={document} className={className} />;
}

function AttestationBadgeWithWorkspace({
  document,
  className,
}: {
  document: unknown;
  className?: string;
}) {
  const wsId = useWorkspaceId();
  return <AttestationBadgeInner wsId={wsId} document={document} className={className} />;
}

function AttestationBadgeInner({
  wsId,
  document,
  className,
}: {
  wsId: string;
  document: unknown;
  className?: string;
}) {
  const { t } = useT("durable");
  const hasDocument =
    document !== undefined &&
    document !== null &&
    serializeAttestationBody(document).trim() !== "";
  const query = useVerifyAttestationQuery(wsId, document, hasDocument);

  if (!hasDocument) {
    return (
      <Badge
        variant="outline"
        data-status="unverified"
        className={cn("text-muted-foreground", className)}
      >
        <ShieldOff aria-hidden="true" />
        {t(($) => $.attestation.unverified)}
      </Badge>
    );
  }

  if (query.isPending) {
    return (
      <Badge
        variant="outline"
        data-status="pending"
        className={cn("text-muted-foreground", className)}
      >
        <Loader2 className="animate-spin" aria-hidden="true" />
        {t(($) => $.attestation.verifying)}
      </Badge>
    );
  }

  if (query.isSuccess && query.data.valid === true) {
    const issuer = query.data.issuer.trim();
    return (
      <Badge
        variant="outline"
        data-status="verified"
        className={cn("border-success/40 bg-success/10 text-success", className)}
      >
        <ShieldCheck aria-hidden="true" />
        {issuer
          ? t(($) => $.attestation.verified_by, { issuer })
          : t(($) => $.attestation.verified)}
      </Badge>
    );
  }

  return (
    <Badge
      variant="outline"
      data-status="unverified"
      className={cn("text-muted-foreground", className)}
    >
      <ShieldOff aria-hidden="true" />
      {t(($) => $.attestation.unverified)}
    </Badge>
  );
}
