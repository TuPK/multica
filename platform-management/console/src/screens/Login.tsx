import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { Banner } from "@astryxdesign/core/Banner";
import { Button } from "@astryxdesign/core/Button";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Shield } from "@multica/ui/icons";
import { InputOTP, InputOTPGroup, InputOTPSlot } from "@multica/ui/components/ui/input-otp";
import { useSendCode, useVerifyCode } from "../api/hooks";
import { t } from "../i18n";

interface LoginProps {
  onSuccess: () => void;
}

/** Six one-character slots, the same input the Multica sign-in page uses. */
function OtpSlots({
  label,
  value,
  onChange,
  disabled,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  return (
    <InputOTP
      autoFocus
      maxLength={6}
      value={value}
      onChange={onChange}
      disabled={disabled}
      aria-label={label}
      autoComplete="one-time-code"
      inputMode="numeric"
    >
      <InputOTPGroup>
        <InputOTPSlot index={0} />
        <InputOTPSlot index={1} />
        <InputOTPSlot index={2} />
        <InputOTPSlot index={3} />
        <InputOTPSlot index={4} />
        <InputOTPSlot index={5} />
      </InputOTPGroup>
    </InputOTP>
  );
}

export function Login({ onSuccess }: LoginProps) {
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [totp, setTotp] = useState("");
  const [step, setStep] = useState<"email" | "code" | "totp">("email");
  const [infoMessage, setInfoMessage] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const sendCode = useSendCode();
  const verifyCode = useVerifyCode();

  const handleSendCode = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || sendCode.isPending) return;
    setErrorMessage(null);
    try {
      await sendCode.mutateAsync({ email: email.trim() });
    } catch {
      // The server returns 204 for every address, but in any case we show the same message
    }
    setStep("code");
    setInfoMessage(t("If this address can sign in, a code has been sent"));
  };

  // Like the Multica sign-in page, a complete 6-digit code submits by itself, and
  // a wrong code clears the slots so the user can type again.
  const handleVerifyCode = async (value: string) => {
    if (value.length !== 6 || verifyCode.isPending) return;
    setErrorMessage(null);
    try {
      const result = await verifyCode.mutateAsync({ email: email.trim(), code: value });
      if (result?.mfaRequired) {
        setInfoMessage(null);
        setStep("totp");
        return;
      }
      onSuccess();
    } catch {
      setCode("");
      setErrorMessage(t("That code is not valid."));
    }
  };

  const handleVerifyTotp = async (value: string) => {
    if (value.length !== 6 || verifyCode.isPending) return;
    setErrorMessage(null);
    try {
      await verifyCode.mutateAsync({ email: email.trim(), code: code.trim(), totp: value });
      onSuccess();
    } catch {
      setTotp("");
      setErrorMessage(t("That code is not valid."));
    }
  };

  const backToEmail = () => {
    setStep("email");
    setCode("");
    setTotp("");
    setErrorMessage(null);
    setInfoMessage(null);
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-app-shell p-4">
      <Card padding={6} elevation="low" className="w-full max-w-sm space-y-5">
        <div className="space-y-1.5 text-center">
          <div className="mx-auto mb-3 flex size-10 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Shield className="size-5" aria-hidden="true" />
          </div>
          <Heading level={1} justify="center">
            {t("Platform Console")}
          </Heading>
          <Text type="supporting" as="p" justify="center">
            {step === "email"
              ? t("Sign in with your staff email address")
              : step === "code"
                ? t("Enter the verification code")
                : t("Enter the code from your authenticator app")}
          </Text>
        </div>

        <div className="space-y-4">
          {infoMessage && <Banner role="status" status="info" title={infoMessage} />}

          {errorMessage && <Banner role="alert" status="error" title={errorMessage} />}

          {step === "email" ? (
            <form onSubmit={handleSendCode} className="space-y-4">
              <TextInput
                id="email-input"
                type="email"
                label={t("Email address")}
                width="100%"
                value={email}
                onChange={(value) => setEmail(value)}
                placeholder={t("staff@example.com")}
                isRequired
                isDisabled={sendCode.isPending}
                autoComplete="email"
              />
              <Button
                type="submit"
                label={t("Send sign-in code")}
                variant="primary"
                width="100%"
                isDisabled={!email.trim() || sendCode.isPending}
                isLoading={sendCode.isPending}
                aria-busy={sendCode.isPending}
              />
            </form>
          ) : step === "code" ? (
            <div className="flex flex-col items-center gap-4">
              <OtpSlots
                label={t("6-digit code")}
                value={code}
                onChange={(value) => {
                  setCode(value);
                  void handleVerifyCode(value);
                }}
                disabled={verifyCode.isPending}
              />
              <Button
                type="button"
                variant="ghost"
                label={t("Use a different email")}
                width="100%"
                onClick={backToEmail}
              />
            </div>
          ) : (
            <div className="flex flex-col items-center gap-4">
              <OtpSlots
                label={t("Authenticator code")}
                value={totp}
                onChange={(value) => {
                  setTotp(value);
                  void handleVerifyTotp(value);
                }}
                disabled={verifyCode.isPending}
              />
              <Button
                type="button"
                variant="ghost"
                label={t("Start over")}
                width="100%"
                onClick={backToEmail}
              />
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
