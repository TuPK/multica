"use client";

import { useId, useState, type FormEvent } from "react";
import { AlertCircle, Check, CheckCircle2, Copy, Loader2 } from "@multica/ui/icons";
import { ApiError, errorCode } from "@multica/core/api";
import {
  useConfirmMFA,
  useDisableMFA,
  useEnrollMFA,
} from "@multica/core/security";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { copyText } from "@multica/ui/lib/clipboard";
import { QRCode } from "react-qr-code";
import { toast } from "sonner";
import { useT } from "../../i18n";
import {
  SettingsCard,
  SettingsRow,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";

type Phase = "idle" | "enrolling" | "recovery" | "already_on";

function mapMfaError(
  error: unknown,
  t: ReturnType<typeof useT<"settings">>["t"],
): string {
  switch (errorCode(error)) {
    case "invalid_code":
      return t(($) => $.two_step.errors.invalid_code);
    case "mfa_not_enabled":
      return t(($) => $.two_step.errors.not_enabled);
    case "mfa_unavailable":
      return t(($) => $.two_step.errors.unavailable);
    case "mfa_not_enrolling":
      return t(($) => $.two_step.restart_notice);
    case "viewer_read_only":
      return t(($) => $.two_step.errors.forbidden);
    default:
      if (error instanceof ApiError && error.status === 503) {
        return t(($) => $.two_step.errors.unavailable);
      }
      if (error instanceof ApiError && error.status === 403) {
        return t(($) => $.two_step.errors.forbidden);
      }
      return t(($) => $.two_step.errors.failed);
  }
}

export function TwoStepTab() {
  const { t } = useT("settings");
  const baseId = useId();
  const enrollMFA = useEnrollMFA();
  const confirmMFA = useConfirmMFA();
  const disableMFA = useDisableMFA();

  const [phase, setPhase] = useState<Phase>("idle");
  const [enrollment, setEnrollment] = useState<{
    secret: string;
    otpauth_uri: string;
  } | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [savedConfirmed, setSavedConfirmed] = useState(false);
  const [confirmCode, setConfirmCode] = useState("");
  const [disableCode, setDisableCode] = useState("");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [copiedField, setCopiedField] = useState<
    "secret" | "uri" | "codes" | null
  >(null);

  const handleCopy = async (
    field: "secret" | "uri" | "codes",
    value: string,
  ) => {
    if (await copyText(value)) {
      setCopiedField(field);
      window.setTimeout(() => {
        setCopiedField((current) => (current === field ? null : current));
      }, 2000);
    }
  };

  const handleStartEnroll = async () => {
    setErrorMessage(null);
    try {
      const data = await enrollMFA.mutateAsync();
      setEnrollment(data);
      setConfirmCode("");
      setPhase("enrolling");
    } catch (error) {
      if (errorCode(error) === "mfa_already_enabled") {
        setEnrollment(null);
        setPhase("already_on");
        return;
      }
      setErrorMessage(mapMfaError(error, t));
    }
  };

  const handleConfirm = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = confirmCode.trim();
    if (!/^\d{6}$/.test(trimmed)) {
      setErrorMessage(t(($) => $.two_step.code_invalid_format));
      return;
    }
    setErrorMessage(null);
    try {
      const res = await confirmMFA.mutateAsync(trimmed);
      setEnrollment(null);
      setConfirmCode("");
      setRecoveryCodes(res.recovery_codes);
      setSavedConfirmed(false);
      setPhase("recovery");
      toast.success(t(($) => $.two_step.enabled_toast));
    } catch (error) {
      if (errorCode(error) === "mfa_not_enrolling") {
        setEnrollment(null);
        setConfirmCode("");
        setPhase("idle");
        setErrorMessage(t(($) => $.two_step.restart_notice));
        return;
      }
      setErrorMessage(mapMfaError(error, t));
    }
  };

  const handleFinishRecovery = () => {
    if (!savedConfirmed) return;
    setRecoveryCodes([]);
    setSavedConfirmed(false);
    setErrorMessage(null);
    setPhase("already_on");
  };

  const handleDisable = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = disableCode.trim();
    if (!trimmed) {
      setErrorMessage(t(($) => $.two_step.disable_code_required));
      return;
    }
    setErrorMessage(null);
    try {
      await disableMFA.mutateAsync(trimmed);
      setDisableCode("");
      setPhase("idle");
      toast.success(t(($) => $.two_step.disabled_toast));
    } catch (error) {
      if (errorCode(error) === "mfa_not_enabled") {
        setPhase("idle");
      }
      setErrorMessage(mapMfaError(error, t));
    }
  };

  const confirmCodeId = `${baseId}-confirm-code`;
  const disableCodeId = `${baseId}-disable-code`;

  return (
    <SettingsTab title={t(($) => $.two_step.title)} scope="account">
      {errorMessage ? (
        <Alert variant="destructive">
          <AlertCircle aria-hidden="true" />
          <AlertDescription>{errorMessage}</AlertDescription>
        </Alert>
      ) : null}

      {phase === "idle" ? (
        <SettingsSection title={t(($) => $.two_step.setup_title)}>
          <SettingsCard>
            <SettingsRow
              anchor="setup"
              label={t(($) => $.two_step.setup_title)}
            >
              <Button
                type="button"
                disabled={enrollMFA.isPending}
                aria-busy={enrollMFA.isPending || undefined}
                onClick={handleStartEnroll}
              >
                {enrollMFA.isPending ? (
                  <Loader2 className="animate-spin motion-reduce:animate-none" />
                ) : null}
                {enrollMFA.isPending
                  ? t(($) => $.two_step.starting)
                  : t(($) => $.two_step.setup_button)}
              </Button>
            </SettingsRow>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      {phase === "already_on" ? (
        <SettingsSection title={t(($) => $.two_step.setup_title)}>
          <SettingsCard>
            <SettingsRow
              anchor="status"
              label={
                <span className="inline-flex items-center gap-2">
                  <span>{t(($) => $.two_step.already_on_title)}</span>
                  <Badge variant="default">
                    <CheckCircle2 className="size-3" aria-hidden="true" />
                    {t(($) => $.two_step.setup_title)}
                  </Badge>
                </span>
              }
              description={t(($) => $.two_step.already_on_consequence)}
            >
              <span />
            </SettingsRow>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      {phase === "enrolling" && enrollment ? (
        <SettingsSection title={t(($) => $.two_step.setup_title)}>
          <SettingsCard>
            <div className="space-y-4 p-4">
              <p className="text-body text-muted-foreground">
                {t(($) => $.two_step.scan_qr)}
              </p>

              <div className="inline-flex rounded-lg border border-surface-border bg-white p-3">
                <QRCode value={enrollment.otpauth_uri} size={160} />
              </div>

              <div className="space-y-1.5">
                <div className="text-caption font-medium text-muted-foreground">
                  {t(($) => $.two_step.secret_label)}
                </div>
                <div className="flex min-w-0 items-center gap-2">
                  <code className="min-w-0 flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 font-mono text-body select-all">
                    {enrollment.secret}
                  </code>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label={t(($) => $.two_step.copy_secret)}
                    onClick={() => handleCopy("secret", enrollment.secret)}
                  >
                    {copiedField === "secret" ? (
                      <Check className="size-4" />
                    ) : (
                      <Copy className="size-4" />
                    )}
                  </Button>
                </div>
              </div>

              <div className="space-y-1.5">
                <div className="text-caption font-medium text-muted-foreground">
                  {t(($) => $.two_step.uri_label)}
                </div>
                <div className="flex min-w-0 items-center gap-2">
                  <code className="min-w-0 flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 font-mono text-caption select-all">
                    {enrollment.otpauth_uri}
                  </code>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label={t(($) => $.two_step.copy_uri)}
                    onClick={() => handleCopy("uri", enrollment.otpauth_uri)}
                  >
                    {copiedField === "uri" ? (
                      <Check className="size-4" />
                    ) : (
                      <Copy className="size-4" />
                    )}
                  </Button>
                </div>
              </div>

              <form noValidate onSubmit={handleConfirm} className="space-y-3 pt-2">
                <div className="space-y-1.5">
                  <Label htmlFor={confirmCodeId}>
                    {t(($) => $.two_step.code_label)}
                  </Label>
                  <Input
                    id={confirmCodeId}
                    type="text"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    value={confirmCode}
                    disabled={confirmMFA.isPending}
                    placeholder={t(($) => $.two_step.code_placeholder)}
                    onChange={(event) =>
                      setConfirmCode(event.currentTarget.value)
                    }
                    className="max-w-48 font-mono"
                  />
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    type="submit"
                    disabled={confirmMFA.isPending || !confirmCode.trim()}
                    aria-busy={confirmMFA.isPending || undefined}
                  >
                    {confirmMFA.isPending ? (
                      <Loader2 className="animate-spin motion-reduce:animate-none" />
                    ) : null}
                    {confirmMFA.isPending
                      ? t(($) => $.two_step.confirming)
                      : t(($) => $.two_step.confirm_button)}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={confirmMFA.isPending}
                    onClick={() => {
                      setEnrollment(null);
                      setConfirmCode("");
                      setErrorMessage(null);
                      setPhase("idle");
                    }}
                  >
                    {t(($) => $.two_step.cancel)}
                  </Button>
                </div>
              </form>
            </div>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      {phase === "recovery" ? (
        <SettingsSection title={t(($) => $.two_step.recovery_title)}>
          <SettingsCard>
            <div className="space-y-4 p-4">
              <Alert role="note">
                <AlertCircle aria-hidden="true" />
                <AlertDescription>
                  {t(($) => $.two_step.recovery_warning)}
                </AlertDescription>
              </Alert>

              <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
                {recoveryCodes.map((code) => (
                  <code
                    key={code}
                    className="rounded-md border bg-muted/50 px-2.5 py-1.5 text-center font-mono text-body select-all"
                  >
                    {code}
                  </code>
                ))}
              </div>

              <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    handleCopy("codes", recoveryCodes.join("\n"))
                  }
                >
                  {copiedField === "codes" ? (
                    <Check className="size-3.5" />
                  ) : (
                    <Copy className="size-3.5" />
                  )}
                  {copiedField === "codes"
                    ? t(($) => $.two_step.copied)
                    : t(($) => $.two_step.copy_codes)}
                </Button>

                <div className="flex flex-wrap items-center gap-4">
                  <label className="flex items-center gap-2 text-body">
                    <Checkbox
                      checked={savedConfirmed}
                      onCheckedChange={(v) => setSavedConfirmed(v === true)}
                    />
                    {t(($) => $.two_step.confirm_saved)}
                  </label>
                  <Button
                    type="button"
                    disabled={!savedConfirmed}
                    onClick={handleFinishRecovery}
                  >
                    {t(($) => $.two_step.done)}
                  </Button>
                </div>
              </div>
            </div>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      {phase === "idle" || phase === "already_on" ? (
        <SettingsSection title={t(($) => $.two_step.disable_title)}>
          <SettingsCard>
            <form noValidate onSubmit={handleDisable}>
              <SettingsRow
                anchor="disable"
                label={
                  <Label htmlFor={disableCodeId}>
                    {t(($) => $.two_step.disable_code_label)}
                  </Label>
                }
                description={t(($) => $.two_step.disable_consequence)}
                size="text"
              >
                <div className="flex items-center gap-2">
                  <Input
                    id={disableCodeId}
                    type="text"
                    autoComplete="off"
                    value={disableCode}
                    disabled={disableMFA.isPending}
                    placeholder={t(($) => $.two_step.disable_code_placeholder)}
                    onChange={(event) =>
                      setDisableCode(event.currentTarget.value)
                    }
                    className="font-mono"
                  />
                  <Button
                    type="submit"
                    variant="destructive"
                    size="sm"
                    disabled={disableMFA.isPending || !disableCode.trim()}
                    aria-busy={disableMFA.isPending || undefined}
                  >
                    {disableMFA.isPending ? (
                      <Loader2 className="animate-spin motion-reduce:animate-none" />
                    ) : null}
                    {disableMFA.isPending
                      ? t(($) => $.two_step.disabling)
                      : t(($) => $.two_step.disable_button)}
                  </Button>
                </div>
              </SettingsRow>
            </form>
          </SettingsCard>
        </SettingsSection>
      ) : null}
    </SettingsTab>
  );
}
