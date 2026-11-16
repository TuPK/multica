import { useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@multica/ui/components/ui/card";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Shield } from "@multica/ui/icons";
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
    <div className="flex min-h-screen items-center justify-center p-4 bg-background">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <div className="mx-auto mb-2 flex size-10 items-center justify-center rounded-full bg-primary/10 text-primary">
            <Shield className="size-5" />
          </div>
          <CardTitle>Platform Console</CardTitle>
          <CardDescription>
            {step === "email"
              ? "Sign in with your staff email address"
              : step === "code"
                ? "Enter the verification code"
                : "Enter the code from your authenticator app"}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {infoMessage && (
            <div
              role="status"
              className="mb-4 rounded-md bg-muted p-3 text-caption text-muted-foreground"
            >
              {infoMessage}
            </div>
          )}

          {errorMessage && (
            <div
              role="alert"
              className="mb-4 rounded-md bg-destructive/10 p-3 text-caption text-destructive"
            >
              {errorMessage}
            </div>
          )}

          {step === "email" ? (
            <form onSubmit={handleSendCode} className="space-y-4">
              <div className="space-y-1.5">
                <label
                  htmlFor="email-input"
                  className="text-caption font-medium text-foreground"
                >
                  Email address
                </label>
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
              </div>
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
              <div className="space-y-1.5">
                <label
                  htmlFor="code-input"
                  className="text-caption font-medium text-foreground"
                >
                  6-digit code
                </label>
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
                />
              </div>
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
              <div className="space-y-1.5">
                <label
                  htmlFor="totp-input"
                  className="text-caption font-medium text-foreground"
                >
                  Authenticator code
                </label>
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
                />
              </div>
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
        </CardContent>
      </Card>
    </div>
  );
}
