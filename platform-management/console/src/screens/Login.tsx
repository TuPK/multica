import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import { Field, FieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { AlertCircle, Info, Shield } from "@multica/ui/icons";
import { useSendCode, useVerifyCode } from "../api/hooks";

interface LoginProps {
  onSuccess: () => void;
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
    setInfoMessage("If this address can sign in, a code has been sent");
  };

  const handleVerifyCode = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!code.trim() || verifyCode.isPending) return;
    setErrorMessage(null);
    try {
      const result = await verifyCode.mutateAsync({ email: email.trim(), code: code.trim() });
      if (result?.mfaRequired) {
        setInfoMessage(null);
        setStep("totp");
        return;
      }
      onSuccess();
    } catch {
      setErrorMessage("That code is not valid.");
    }
  };

  const handleVerifyTotp = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!totp.trim() || verifyCode.isPending) return;
    setErrorMessage(null);
    try {
      await verifyCode.mutateAsync({ email: email.trim(), code: code.trim(), totp: totp.trim() });
      onSuccess();
    } catch {
      setErrorMessage("That code is not valid.");
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
            Platform Console
          </Heading>
          <Text type="supporting" as="p" justify="center">
            {step === "email"
              ? "Sign in with your staff email address"
              : step === "code"
                ? "Enter the verification code"
                : "Enter the code from your authenticator app"}
          </Text>
        </div>

        <div className="space-y-4">
          {infoMessage && (
            <Alert role="status" className="border-info/30 bg-info/10 text-foreground">
              <Info className="size-4 text-info" aria-hidden="true" />
              <AlertDescription className="text-caption text-foreground">
                {infoMessage}
              </AlertDescription>
            </Alert>
          )}

          {errorMessage && (
            <Alert variant="destructive" className="border-destructive/30 bg-destructive/5">
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              <AlertDescription className="text-caption">{errorMessage}</AlertDescription>
            </Alert>
          )}

          {step === "email" ? (
            <form onSubmit={handleSendCode} className="space-y-4">
              <Field>
                <FieldLabel htmlFor="email-input" className="text-label">
                  Email address
                </FieldLabel>
                <Input
                  id="email-input"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="staff@example.com"
                  required
                  disabled={sendCode.isPending}
                  autoComplete="email"
                />
              </Field>
              <Button
                type="submit"
                className="w-full"
                disabled={!email.trim() || sendCode.isPending}
                aria-busy={sendCode.isPending}
              >
                {sendCode.isPending && <Spinner className="size-3.5" />}
                Send sign-in code
              </Button>
            </form>
          ) : step === "code" ? (
            <form onSubmit={handleVerifyCode} className="space-y-4">
              <Field>
                <FieldLabel htmlFor="code-input" className="text-label">
                  6-digit code
                </FieldLabel>
                <Input
                  id="code-input"
                  type="text"
                  inputMode="numeric"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder="000000"
                  maxLength={6}
                  required
                  disabled={verifyCode.isPending}
                  autoFocus
                  className="text-center font-mono tabular-nums tracking-widest"
                />
              </Field>
              <Button
                type="submit"
                className="w-full"
                disabled={!code.trim() || verifyCode.isPending}
                aria-busy={verifyCode.isPending}
              >
                {verifyCode.isPending && <Spinner className="size-3.5" />}
                Sign in
              </Button>
              <Button
                type="button"
                variant="ghost"
                className="w-full text-caption"
                onClick={backToEmail}
              >
                Use a different email
              </Button>
            </form>
          ) : (
            <form onSubmit={handleVerifyTotp} className="space-y-4">
              <Field>
                <FieldLabel htmlFor="totp-input" className="text-label">
                  Authenticator code
                </FieldLabel>
                <Input
                  id="totp-input"
                  type="text"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={totp}
                  onChange={(e) => setTotp(e.target.value)}
                  placeholder="000000"
                  maxLength={6}
                  required
                  disabled={verifyCode.isPending}
                  autoFocus
                  className="text-center font-mono tabular-nums tracking-widest"
                />
              </Field>
              <Button
                type="submit"
                className="w-full"
                disabled={!totp.trim() || verifyCode.isPending}
                aria-busy={verifyCode.isPending}
              >
                {verifyCode.isPending && <Spinner className="size-3.5" />}
                Sign in
              </Button>
              <Button
                type="button"
                variant="ghost"
                className="w-full text-caption"
                onClick={backToEmail}
              >
                Start over
              </Button>
            </form>
          )}
        </div>
      </Card>
    </div>
  );
}
