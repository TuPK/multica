"use client";

import { useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { BookOpenText, ExternalLink, Package } from "@multica/ui/icons";
import { Badge } from "@multica/ui/components/ui/badge";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { cn } from "@multica/ui/lib/utils";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import {
  hubKeys,
  useHubItem,
  useHubItemMeta,
  type HubItem,
  type HubItemKind,
} from "@multica/core/hub";
import { pluginPackagesOptions } from "@multica/core/plugins";
import {
  memberListOptions,
  skillDetailOptions,
  skillVersionsOptions,
} from "@multica/core/workspace/queries";
import { useT, useTimeAgo } from "../i18n";
import { CollectionPageState } from "../layout/collection-page";
import { ObjectIconMark, ObjectIdentity, ObjectPage } from "../layout/object-page";
import { PAGE_GUTTER, PAGE_RAIL } from "../layout/page-header";
import { SkillVersionsTab } from "../skills/components/skill-versions-tab";
import { useCanEditSkill } from "../skills/hooks/use-can-edit-skill";
import { HubActivityList } from "./hub-activity-list";
import { HubDownloadBundleButton } from "./hub-export-actions";
import { HubItemFiles } from "./hub-item-files";
import { PluginOverview, PluginUsage, PluginVersions, SkillOverview, SkillUsage } from "./hub-item-sections";
import { HubItemSourceTab } from "./hub-source-tab";
import { HubUpdateBanner } from "./hub-update-banner";
import { formatHubSize } from "./hub-files";
import { useHubFiles } from "./use-hub-files";

const HUB_ITEM_LAYOUT_ID = "multica_hub_item_layout";
const TABS = ["files", "overview", "versions", "usage", "source", "activity"] as const;
type HubItemTab = (typeof TABS)[number];

export function isHubItemKind(value: string): value is HubItemKind {
  return value === "skill" || value === "plugin";
}

/**
 * One Hub item: a skill or a published plugin. The header keeps its place while
 * the body loads, and each tab reads only what it shows.
 */
export function HubItemPage({ kind, refId }: { kind: string; refId: string }) {
  const { t } = useT("hub");
  const wsId = useWorkspaceId();
  const itemKind = isHubItemKind(kind) ? kind : null;
  const { item, isLoading, isError } = useHubItem(wsId, itemKind ?? "skill", refId);

  if (itemKind === null) {
    return (
      <HubItemFrame leaf={null}>
        <CollectionPageState icon={Package} title={t(($) => $.item.not_found)} />
      </HubItemFrame>
    );
  }
  if (isLoading) {
    return (
      <HubItemFrame leaf={<Skeleton className="h-4 w-40" />}>
        <div className={cn(PAGE_RAIL, PAGE_GUTTER, "flex flex-col gap-3 py-6")} aria-busy="true">
          <Skeleton className="h-5 w-1/3" />
          <Skeleton className="h-4 w-1/2" />
        </div>
      </HubItemFrame>
    );
  }
  if (isError) {
    return (
      <HubItemFrame leaf={null}>
        <CollectionPageState icon={Package} tone="destructive" title={t(($) => $.item.load_error)} />
      </HubItemFrame>
    );
  }
  if (!item) {
    return (
      <HubItemFrame leaf={null}>
        <CollectionPageState icon={Package} title={t(($) => $.item.not_found)} />
      </HubItemFrame>
    );
  }
  return <HubItemView item={item} wsId={wsId} />;
}

function HubItemFrame({ leaf, children }: { leaf: ReactNode; children: ReactNode }) {
  const { t } = useT("hub");
  const paths = useWorkspacePaths();
  return (
    <ObjectPage
      layoutId={HUB_ITEM_LAYOUT_ID}
      segments={[{ href: paths.hub(), label: t(($) => $.title) }]}
      leaf={leaf ?? <span className="truncate text-muted-foreground">{t(($) => $.title)}</span>}
    >
      {children}
    </ObjectPage>
  );
}

// Source badge, as in the list. Links out when the source has a URL.
function SourceBadge({ item }: { item: HubItem }) {
  const { t } = useT("hub");
  const label = t(($) => $.source_types[item.source]);
  const className =
    "inline-flex items-center gap-1 truncate rounded-sm bg-muted px-1.5 py-0.5 text-caption text-muted-foreground transition-colors";
  return item.sourceUrl ? (
    <a
      href={item.sourceUrl}
      target="_blank"
      rel="noreferrer noopener"
      title={t(($) => $.open_source)}
      className={cn(className, "hover:bg-muted/80 hover:text-foreground")}
    >
      <span>{label}</span>
      <ExternalLink className="size-2.5 shrink-0 opacity-70" aria-hidden="true" />
    </a>
  ) : (
    <span className={className}>{label}</span>
  );
}

function HubItemView({ item, wsId }: { item: HubItem; wsId: string }) {
  const { t } = useT("hub");
  const timeAgo = useTimeAgo();
  const qc = useQueryClient();
  const paths = useWorkspacePaths();
  const userId = useAuthStore((s) => s.user?.id ?? null);
  const [tab, setTab] = useState<HubItemTab>("files");

  const kind = item.kind;
  const refId = item.refId;
  const isSkill = kind === "skill";

  const skill = useQuery({ ...skillDetailOptions(wsId, refId), enabled: isSkill });
  const skillVersions = useQuery({ ...skillVersionsOptions(wsId, refId), enabled: isSkill });
  const members = useQuery(memberListOptions(wsId));
  // The plugin package list sits behind a feature flag; its failure must not hide the item's shell.
  const packages = useQuery({ ...pluginPackagesOptions(wsId), enabled: !isSkill, retry: false });

  const pkg = packages.data?.packages.find((candidate) => candidate.id === refId);
  const newest = pkg?.versions[0];
  const myRole = members.data?.find((member) => member.user_id === userId)?.role ?? null;
  const canEditSkill = useCanEditSkill(skill.data, wsId);
  // Publishing a plugin version is an admin decision: owners and admins only.
  const canEditPlugin = myRole === "admin" || myRole === "owner";
  const canEdit = isSkill ? canEditSkill : canEditPlugin;
  const itemMeta = useHubItemMeta(wsId, kind, refId);

  const files = useHubFiles({
    wsId,
    kind,
    refId,
    versionId: newest?.id,
    ready: isSkill || newest !== undefined,
    canEdit,
  });

  const version = isSkill ? skillVersions.data?.[0]?.version : newest?.version;
  const reservedPath = isSkill ? "SKILL.md" : "multica.plugin.json";
  const publishedVersions = pkg?.versions.map((candidate) => candidate.version) ?? [];

  const header = (
    <div className={cn(PAGE_RAIL, PAGE_GUTTER, "flex flex-col gap-3 pt-6")}>
      <ObjectIdentity
        media={<ObjectIconMark icon={isSkill ? BookOpenText : Package} />}
        title={item.name}
        description={item.description || undefined}
        trailing={<Badge variant="outline">{t(($) => $.kind_singular[kind])}</Badge>}
      />
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-2 text-caption text-muted-foreground">
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
          {version ? <span className="font-mono">{t(($) => $.item.version, { version })}</span> : null}
          <SourceBadge item={item} />
          {files.totalBytes !== null ? <span>{formatHubSize(files.totalBytes)}</span> : null}
          {newest ? (
            <span className="flex min-w-0 items-center gap-1">
              <span>{t(($) => $.item.checksum)}</span>
              <code className="truncate font-mono" title={newest.digest}>
                {newest.digest.slice(0, 12)}
              </code>
            </span>
          ) : null}
          <span>{t(($) => $.item.updated, { time: timeAgo(item.updatedAt) })}</span>
        </div>
        <div className="flex items-center gap-2">
          <HubDownloadBundleButton wsId={wsId} kind={kind} refId={refId} />
        </div>
      </div>
    </div>
  );

  const body = (
    <Tabs
      value={tab}
      onValueChange={(next) => setTab(next as HubItemTab)}
      className="flex min-h-0 flex-1 flex-col"
    >
      <div className={cn(PAGE_RAIL, PAGE_GUTTER, "shrink-0 pt-4")}>
        <TabsList variant="line" className="border-b">
          <TabsTrigger value="files" className="relative">
            {t(($) => $.tabs.files)}
            {files.isDirty && (
              <span
                className="ml-1.5 inline-block size-1.5 shrink-0 rounded-full bg-brand"
                aria-hidden="true"
              />
            )}
          </TabsTrigger>
          <TabsTrigger value="overview">{t(($) => $.tabs.overview)}</TabsTrigger>
          <TabsTrigger value="versions">{t(($) => $.tabs.versions)}</TabsTrigger>
          <TabsTrigger value="usage">{t(($) => $.tabs.usage)}</TabsTrigger>
          <TabsTrigger value="source">{t(($) => $.source_tab.title)}</TabsTrigger>
          <TabsTrigger value="activity">{t(($) => $.activity.tab)}</TabsTrigger>
        </TabsList>
      </div>

      <TabsContent value="files" className="flex min-h-full flex-col md:h-full md:min-h-0">
        <HubItemFiles
          kind={kind}
          canEdit={canEdit}
          files={files}
          reservedPath={reservedPath}
          currentVersion={newest?.version ?? null}
          publishedVersions={publishedVersions}
        />
      </TabsContent>

      <TabsContent value="overview" className="py-6">
        {isSkill ? (
          <SkillOverview wsId={wsId} refId={refId} draftContent={files.draftFor("SKILL.md")?.content} />
        ) : pkg && newest ? (
          <PluginOverview wsId={wsId} refId={refId} pkg={pkg} versionId={newest.id} paths={files.paths} />
        ) : (
          <Skeleton className="mx-auto h-24 w-full max-w-[760px]" />
        )}
      </TabsContent>

      <TabsContent value="versions" className="py-6">
        {isSkill ? (
          skill.data ? (
            <div className="mx-auto w-full max-w-[1100px] px-4">
              <SkillVersionsTab
                skill={skill.data}
                wsId={wsId}
                canEdit={canEdit}
                hasUnsavedChanges={files.isDirty}
                members={members.data ?? []}
                onRestored={() => {
                  // A restore rewrites the files, so the Files tab must read them again.
                  void qc.invalidateQueries({ queryKey: hubKeys.files(wsId, kind, refId) });
                }}
              />
            </div>
          ) : (
            <Skeleton className="mx-auto h-24 w-full max-w-[760px]" />
          )
        ) : pkg ? (
          <PluginVersions wsId={wsId} pkg={pkg} />
        ) : (
          <Skeleton className="mx-auto h-24 w-full max-w-[760px]" />
        )}
      </TabsContent>

      <TabsContent value="source" className="py-6">
        <HubItemSourceTab
          wsId={wsId}
          item={item}
          meta={itemMeta.meta}
          metaLoading={itemMeta.isLoading}
          canEdit={canEdit}
        />
      </TabsContent>

      <TabsContent value="activity" className="py-6">
        <div className="mx-auto w-full max-w-[760px] px-4">
          <HubActivityList wsId={wsId} filter={{ kind, refId }} />
        </div>
      </TabsContent>

      <TabsContent value="usage" className="py-6">
        {isSkill ? (
          <SkillUsage wsId={wsId} refId={refId} />
        ) : (
          pkg ? <PluginUsage wsId={wsId} pluginKey={pkg.plugin_key} /> : <Skeleton className="mx-auto h-24 w-full max-w-[760px]" />
        )}
      </TabsContent>
    </Tabs>
  );

  return (
    <ObjectPage
      layoutId={HUB_ITEM_LAYOUT_ID}
      segments={[{ href: paths.hub(), label: t(($) => $.title) }]}
      leaf={<span className="min-w-0 truncate text-foreground">{item.name}</span>}
    >
      <div className="flex min-h-full flex-col md:h-full">
        {header}
        <div className={cn(PAGE_RAIL, PAGE_GUTTER, "pt-3 empty:hidden")}>
          <HubUpdateBanner
            wsId={wsId}
            kind={kind}
            refId={refId}
            name={item.name}
            meta={itemMeta.meta}
            canEdit={canEdit}
          />
        </div>
        {body}
      </div>
    </ObjectPage>
  );
}
