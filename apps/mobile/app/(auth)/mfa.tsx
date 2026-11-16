import { useState } from "react";
import { KeyboardAvoidingView, Platform, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { Redirect, router } from "expo-router";
import * as Haptics from "expo-haptics";
import { MulticaLogo } from "@/components/brand/multica-logo";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { TextField } from "@/components/ui/text-field";
import { useAuthStore } from "@/data/auth-store";
import { mapAuthError } from "@/lib/auth-error";
import { i18n, useT } from "@/lib/i18n";

export default function MfaVerify() {
  const { t } = useT("auth");
  const user = useAuthStore((s) => s.user);
  const hasMfaChallenge = useAuthStore((s) => s.hasMfaChallenge);
  const verifyMFA = useAuthStore((s) => s.verifyMFA);
  const clearMFAChallenge = useAuthStore((s) => s.clearMFAChallenge);

  const [code, setCode] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (user) {
    return <Redirect href="/" />;
  }
  if (!hasMfaChallenge) {
    return <Redirect href="/login" />;
  }

  const onSubmit = async () => {
    const trimmed = code.trim();
    if (!trimmed || submitting) return;
    void Haptics.selectionAsync();
    setSubmitting(true);
    setError(null);
    try {
      await verifyMFA(trimmed);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.replace("/");
    } catch (err) {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      if (!useAuthStore.getState().hasMfaChallenge) {
        router.replace("/login");
        return;
      }
      setError(
        mapAuthError(err, t("mfa.verify_failed"), (key) => i18n.t(`auth:${key}`)),
      );
      setSubmitting(false);
    }
  };

  const onBackToSignIn = () => {
    clearMFAChallenge();
    router.replace("/login");
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
                {t("mfa.title")}
              </Text>
              <Text className="text-sm text-muted-foreground text-center">
                {t("mfa.subtitle")}
              </Text>
            </View>
          </View>

          <View className="gap-3">
            <TextField
              autoCapitalize="none"
              autoComplete="off"
              autoCorrect={false}
              autoFocus
              placeholder={t("mfa.code_placeholder")}
              value={code}
              onChangeText={setCode}
              onSubmitEditing={onSubmit}
              returnKeyType="done"
              editable={!submitting}
              invalid={!!error}
            />
            {error ? (
              <Text className="text-sm text-destructive">{error}</Text>
            ) : null}
          </View>

          <View className="gap-3">
            <Button
              size="lg"
              disabled={submitting || !code.trim()}
              onPress={onSubmit}
            >
              <Text>{submitting ? t("mfa.verifying") : t("mfa.verify")}</Text>
            </Button>

            <Button
              variant="ghost"
              disabled={submitting}
              onPress={onBackToSignIn}
            >
              <Text>{t("mfa.back_to_sign_in")}</Text>
            </Button>
          </View>
        </View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
