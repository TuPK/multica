"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { useEnablePrincipalProfile } from "@multica/core/durable/principal";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { Loader2 } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { buildConsentText, principalErrorMessage } from "./error-copy";

/**
 * Explicit opt-in consent before keeping a person's principal profile (durable spec 6.6).
 * States the four required privacy guarantees, keeps the confirm action disabled until the
 * consent checkbox is ticked, and defaults sharing with owners to off.
 */
export function ConsentDialog({
  open,
  onOpenChange,
  wsId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wsId: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <ConsentForm wsId={wsId} onDone={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

function ConsentForm({ wsId, onDone }: { wsId: string; onDone: () => void }) {
  const { t } = useT("durable");
  const enable = useEnablePrincipalProfile(wsId);
  const [consented, setConsented] = useState(false);
  const [shareWithOwners, setShareWithOwners] = useState(false);

  const consentText = buildConsentText(t);
  const canSubmit = consented && !enable.isPending;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!canSubmit) return;
    enable
      .mutateAsync({
        consent_text: consentText,
        share_with_owners: shareWithOwners,
      })
      .then(
        () => {
          toast.success(t(($) => $.principal.consent.enabled));
          onDone();
        },
        (err: unknown) => {
          toast.error(principalErrorMessage(err, t, t(($) => $.principal.consent.enable_failed)));
        },
      );
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{t(($) => $.principal.consent.title)}</DialogTitle>
      </DialogHeader>

      <ul className="flex flex-col gap-2 text-body text-foreground">
        <li>{t(($) => $.principal.consent.point_data)}</li>
        <li>{t(($) => $.principal.consent.point_access)}</li>
        <li>{t(($) => $.principal.consent.point_no_eval)}</li>
        <li>{t(($) => $.principal.consent.point_delete)}</li>
      </ul>

      <div className="flex items-center justify-between gap-3 rounded-md border border-surface-border px-3 py-2">
        <Label htmlFor="principal-consent-share" className="text-body font-normal">
          {t(($) => $.principal.consent.share_with_owners)}
        </Label>
        <Switch
          id="principal-consent-share"
          checked={shareWithOwners}
          disabled={enable.isPending}
          onCheckedChange={setShareWithOwners}
        />
      </div>

      <div className="flex items-start gap-2.5">
        <Checkbox
          id="principal-consent-accept"
          checked={consented}
          disabled={enable.isPending}
          onCheckedChange={(checked) => setConsented(Boolean(checked))}
          className="mt-0.5"
        />
        <Label htmlFor="principal-consent-accept" className="text-body font-normal leading-snug">
          {t(($) => $.principal.consent.checkbox)}
        </Label>
      </div>

      <DialogFooter>
        <DialogClose render={<Button type="button" variant="outline" disabled={enable.isPending} />}>
          {t(($) => $.principal.cancel)}
        </DialogClose>
        <Button type="submit" disabled={!canSubmit} aria-busy={enable.isPending || undefined}>
          {enable.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
          {t(($) => $.principal.consent.confirm)}
        </Button>
      </DialogFooter>
    </form>
  );
}
