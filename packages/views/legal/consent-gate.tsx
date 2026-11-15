"use client";

import { useEffect, useState } from "react";
import { useAuthStore } from "@multica/core/auth";
import { useAcceptConsents } from "@multica/core/account";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { ExternalLink, Loader2 } from "@multica/ui/icons";
import { useT } from "../i18n";

export interface RequiredConsentDocument {
  document: string;
  version: string;
  url: string;
}

function getDocumentLabel(
  document: string,
  t: ReturnType<typeof useT<"settings">>["t"],
): string {
  if (document === "terms") {
    return t(($) => $.legal.terms);
  }
  if (document === "privacy") {
    return t(($) => $.legal.privacy);
  }
  return document;
}

export function ConsentGate() {
  const { t } = useT("settings");
  const user = useAuthStore((s) => s.user);
  const setUser = useAuthStore((s) => s.setUser);
  const consentRequired = (
    user as { consent_required?: RequiredConsentDocument[] } | null
  )?.consent_required;

  const acceptConsents = useAcceptConsents();
  const isPending = Boolean(acceptConsents?.isPending);
  const [open, setOpen] = useState(true);

  const hasRequired = Boolean(consentRequired && consentRequired.length > 0);

  useEffect(() => {
    if (hasRequired) {
      setOpen(true);
    }
  }, [hasRequired]);

  if (!hasRequired || !open || !consentRequired) {
    return null;
  }

  const handleAccept = async () => {
    if (isPending) return;
    try {
      const res = await acceptConsents.mutateAsync({
        documents: consentRequired.map((doc) => ({
          document: doc.document,
          version: doc.version,
        })),
      });

      setOpen(false);

      if (user) {
        const remaining = (
          res as { consent_required?: RequiredConsentDocument[] } | null | undefined
        )?.consent_required;
        setUser({ ...user, consent_required: remaining ?? [] });
      }
    } catch {
      // Preserve dialog and required documents on error
    }
  };

  return (
    <Dialog open={open} onOpenChange={() => {}} disablePointerDismissal>
      <DialogContent
        showCloseButton={false}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.preventDefault();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>{t(($) => $.legal.title)}</DialogTitle>
        </DialogHeader>
        <ul className="flex flex-col gap-2 py-2">
          {consentRequired.map((doc) => (
            <li key={doc.document} className="flex items-center justify-between">
              <a
                href={doc.url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-body text-primary underline underline-offset-4 hover:text-primary/80"
              >
                <span>{getDocumentLabel(doc.document, t)}</span>
                <ExternalLink className="size-3.5" />
              </a>
            </li>
          ))}
        </ul>
        <DialogFooter>
          <Button
            type="button"
            onClick={handleAccept}
            disabled={isPending}
            aria-busy={isPending || undefined}
          >
            {isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            {t(($) => $.legal.accept)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
