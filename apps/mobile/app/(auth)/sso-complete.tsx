import { useEffect, useRef } from "react";
import { ActivityIndicator, View } from "react-native";
import { router, useLocalSearchParams } from "expo-router";
import { useAuthStore } from "@/data/auth-store";
import { api } from "@/data/api";
import { clearSsoVerifier, takeSsoVerifier } from "@/lib/sso-handoff";

/**
 * Landing route for multica://sso-complete, the end of the system-browser SSO
 * flow. `code` is exchanged (with the in-memory verifier) for a session;
 * `error` or any failure returns to /login with a message.
 */
export default function SsoComplete() {
  const { code, error } = useLocalSearchParams<{ code?: string; error?: string }>();
  const finishSSOLogin = useAuthStore((s) => s.finishSSOLogin);
  const handled = useRef(false);

  useEffect(() => {
    if (handled.current) return;
    handled.current = true;

    const fail = (reason: string) => {
      clearSsoVerifier();
      router.replace({ pathname: "/login", params: { sso_error: reason } });
    };

    if (error) return fail(error);
    const verifier = takeSsoVerifier();
    if (!code || !verifier) return fail("sso_login_failed");

    api
      .exchangeSSO(code, verifier)
      .then(async (res) => {
        if (!res.token) return fail("sso_login_failed");
        await finishSSOLogin(res);
        router.replace("/");
      })
      .catch(() => fail("sso_login_failed"));
  }, [code, error, finishSSOLogin]);

  return (
    <View className="flex-1 items-center justify-center bg-background">
      <ActivityIndicator />
    </View>
  );
}
