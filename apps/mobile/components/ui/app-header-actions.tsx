/**
 * Header utility buttons shared across primary tabs (Inbox / My Issues).
 * Provides two global actions on the right: search and create-issue. A
 * guest (MUL-9420) cannot create issues, so it gets search only.
 *
 * The workspace menu (global nav, workspace switcher, settings) is reached
 * via the "More" tab in the bottom bar.
 *
 * Tab-specific actions (e.g. My Issues filter) MUST NOT live here — they
 * mix scope levels with global actions and would clutter the strip.
 */
import { router } from "expo-router";
import { IconButton } from "@/components/ui/icon-button";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useT } from "@/lib/i18n";
import { useCanWrite } from "@/lib/use-workspace-access";
import { useIsGuest } from "@/lib/use-is-guest";

export function HeaderActions() {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const slug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const canWrite = useCanWrite(wsId);
  const { t } = useT();
  const guest = useIsGuest();

  const onSearch = () => {
    if (slug) router.push(`/${slug}/search`);
  };
  const onCreate = () => {
    if (slug) router.push(`/${slug}/new-issue`);
  };

  return (
    <>
      <IconButton
        name="search"
        onPress={onSearch}
        accessibilityLabel={t("common:actions.search")}
      />
      {!guest && (
        <IconButton
          name="add"
          iconSize={24}
          onPress={onCreate}
          accessibilityLabel={t("navigation:routes.new_issue")}
        />
      )}
    </>
  );
}
