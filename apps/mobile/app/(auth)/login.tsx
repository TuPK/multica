import { useState } from "react";
import { KeyboardAvoidingView, Linking, Platform, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router, useLocalSearchParams } from "expo-router";
import * as Haptics from "expo-haptics";
import { Text } from "@/components/ui/text";
import { TextField } from "@/components/ui/text-field";
import { Button } from "@/components/ui/button";
import { MulticaLogo } from "@/components/brand/multica-logo";
import { api } from "@/data/api";
import { useAuthStore } from "@/data/auth-store";
import { i18n, useT } from "@/lib/i18n";
import { mapAuthError } from "@/lib/auth-error";
import { ssoErrorKey } from "@/lib/sso-error";
import { beginSsoHandoff, buildSsoStartUrl, clearSsoVerifier } from "@/lib/sso-handoff";

export default function Login() {
  const { t } = useT("auth");
  const { sso_error: ssoError } = useLocalSearchParams<{ sso_error?: string }>();
  const sendCode = useAuthStore((s) => s.sendCode);
  const signOutReason = useAuthStore((s) => s.signOutReason);
  const mfaFailureReason = useAuthStore((s) => s.mfaFailureReason);
  const [email, setEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  // undefined = untouched, so a failure passed in by the sso-complete route
  // shows until the next attempt sets or clears the error.
  const [error, setError] = useState<string | null | undefined>(undefined);
  const shownError =
    error === undefined ? (ssoError ? String(i18n.t(`auth:${ssoErrorKey(ssoError)}`)) : null) : error;

  const reasonBanner =
    signOutReason === "account_suspended"
      ? t("reasons.account_suspended")
      : signOutReason === "session_revoked"
        ? t("reasons.session_revoked")
        : mfaFailureReason === "mfa_token_failed"
          ? t("mfa.token_failed")
          : mfaFailureReason === "mfa_token_invalid"
            ? t("mfa.token_invalid")
            : null;

  const onSubmit = async () => {
    const trimmed = email.trim();
    if (!trimmed) return;
    void Haptics.selectionAsync();
    setSubmitting(true);
    setError(null);
    try {
      await sendCode(trimmed);
      router.push({ pathname: "/verify", params: { email: trimmed } });
    } catch (err) {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      setError(mapAuthError(err, t("login.send_failed"), (key) => i18n.t(`auth:${key}`)));
    } finally {
      setSubmitting(false);
    }
  };

  const onSso = async () => {
    const trimmed = email.trim();
    if (!trimmed) return;
    void Haptics.selectionAsync();
    setSubmitting(true);
    setError(null);
    try {
      const discovery = await api.discoverSSO(trimmed);
      if (!discovery.sso || !discovery.start_url) {
        setError(t("sso.not_configured"));
        return;
      }
      const challenge = await beginSsoHandoff();
      await Linking.openURL(buildSsoStartUrl(discovery.start_url, challenge));
    } catch (err) {
      clearSsoVerifier();
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      setError(mapAuthError(err, t("sso.failed"), (key) => i18n.t(`auth:${key}`)));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <SafeAreaView className="flex-1 bg-background">
      <KeyboardAvoidingView
        className="flex-1"
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        <View className="flex-1 justify-center px-6 gap-6">
          <View className="items-center gap-3">
            <MulticaLogo size={30} />
            <View className="gap-1 items-center">
              <Text className="text-2xl font-semibold text-foreground">
                {t("login.title")}
              </Text>
              <Text className="text-sm text-muted-foreground text-center">
                {t("login.subtitle")}
              </Text>
            </View>
          </View>

          <View className="gap-3">
            {reasonBanner ? (
              <View className="rounded-md bg-destructive/10 border border-destructive/30 px-3 py-2">
                <Text className="text-sm text-destructive text-center">
                  {reasonBanner}
                </Text>
              </View>
            ) : null}
            <TextField
              autoCapitalize="none"
              autoComplete="email"
              autoFocus
              keyboardType="email-address"
              placeholder={t("login.email_placeholder")}
              value={email}
              onChangeText={setEmail}
              onSubmitEditing={onSubmit}
              returnKeyType="send"
              editable={!submitting}
              invalid={!!shownError}
            />
            {shownError ? (
              <Text className="text-sm text-destructive">{shownError}</Text>
            ) : null}
          </View>

          <Button
            size="lg"
            disabled={submitting || !email.trim()}
            onPress={onSubmit}
          >
            <Text>{submitting ? t("login.sending") : t("login.send_code")}</Text>
          </Button>
          <Button
            variant="outline"
            size="lg"
            disabled={submitting || !email.trim()}
            onPress={onSso}
          >
            <Text>{t("login.sso")}</Text>
          </Button>
        </View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
