import type { ReactNode } from "react";
import { ActivityIndicator, View } from "react-native";
import { router } from "expo-router";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useT } from "@/lib/i18n";
import { useWorkspaceAccessStatus } from "@/lib/use-workspace-access";

/**
 * Guards write-only routes (create/edit modals and formSheet pickers),
 * including cold-start deep links: waits for members to resolve before hiding
 * controls and showing a short read-only notice with a Back action.
 */
export function WriteAccessGuard({ children }: { children: ReactNode }) {
  const slug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const { canWrite, isLoading, block } = useWorkspaceAccessStatus();
  const { t } = useT(["workspace", "common"]);

  if (isLoading) {
    return (
      <View className="flex-1 bg-background items-center justify-center">
        <ActivityIndicator />
      </View>
    );
  }

  if (!canWrite) {
    const message =
      block === "read_only"
        ? t("workspace:access.read_only_banner")
        : t("workspace:access.viewer_write_blocked");

    const onBack = () => {
      if (router.canGoBack()) {
        router.back();
      } else if (slug) {
        router.replace(`/${slug}/inbox`);
      }
    };

    return (
      <View className="flex-1 bg-background items-center justify-center px-6 gap-4">
        <Text className="text-sm text-muted-foreground text-center">
          {message}
        </Text>
        <Button variant="outline" onPress={onBack}>
          <Text>{t("common:actions.back")}</Text>
        </Button>
      </View>
    );
  }

  return <>{children}</>;
}
