"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bot, File } from "@multica/ui/icons";
import { ActorAvatar } from "@multica/ui/components/common/actor-avatar";
import { Badge } from "@multica/ui/components/ui/badge";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { cn } from "@multica/ui/lib/utils";
import {
  hubFileContentOptions,
  PLUGIN_MANIFEST_FILENAME,
  SKILL_CONTENT_PATH,
} from "@multica/core/hub";
import { parseFrontmatter } from "@multica/core/skills/frontmatter";
import { pluginInstallationsOptions } from "@multica/core/plugins";
import { agentListOptions, selectSkillAssignments } from "@multica/core/workspace/queries";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type { PluginPackage } from "@multica/core/types";
import { OBJECT_PAGE_BODY, ObjectSection, ObjectSectionCard, ObjectSectionEmpty } from "../layout/object-page";
import { RichContent } from "../rich-content";
import { useT, useTimeAgo } from "../i18n";
import { formatHubSize, prettyJson } from "./hub-files";

const PLUGIN_README_PATH = "README.md";

function formatDigest(digest: string): string {
  return digest.slice(0, 12);
}

/** SKILL.md as it reads, with the draft text when the Files tab has unsaved edits. */
export function SkillOverview({
  wsId,
  refId,
  draftContent,
}: {
  wsId: string;
  refId: string;
  /** Unsaved SKILL.md text from the Files tab, shown in place of the saved text. */
  draftContent: string | undefined;
}) {
  const { t } = useT("hub");
  const file = useQuery({
    ...hubFileContentOptions(wsId, "skill", refId, SKILL_CONTENT_PATH),
    enabled: wsId.length > 0 && refId.length > 0,
  });
  const text = draftContent ?? file.data?.content ?? "";

  if (file.isLoading && draftContent === undefined) {
    return (
      <div className={cn(OBJECT_PAGE_BODY, "flex flex-col gap-2")} aria-busy="true">
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-1/2" />
      </div>
    );
  }
  const body = parseFrontmatter(text).body;
  return (
    <div className={OBJECT_PAGE_BODY}>
      {body.trim() === "" ? (
        <ObjectSectionEmpty icon={File} description={t(($) => $.overview.no_content)} />
      ) : (
        <ObjectSectionCard>
          <div className="px-4 py-3">
            <RichContent content={body} density="document" phase="settled" />
          </div>
        </ObjectSectionCard>
      )}
    </div>
  );
}

function PluginTextFile({
  wsId,
  refId,
  versionId,
  path,
  asManifest,
}: {
  wsId: string;
  refId: string;
  versionId: string;
  path: string;
  asManifest: boolean;
}) {
  const file = useQuery({
    ...hubFileContentOptions(wsId, "plugin", refId, path, versionId),
    enabled: wsId.length > 0 && refId.length > 0 && versionId.length > 0,
  });
  if (file.isLoading) return <Skeleton className="h-16 w-full" />;
  const text = file.data?.content;
  if (text === undefined || text === null) return null;
  if (asManifest) {
    return (
      <pre className="overflow-x-auto whitespace-pre px-4 py-3 font-mono text-caption leading-relaxed">
        {prettyJson(text) ?? text}
      </pre>
    );
  }
  return (
    <div className="px-4 py-3">
      <RichContent content={text} density="document" phase="settled" />
    </div>
  );
}

/** Plugin summary from the published package, then its manifest and README when the version has them. */
export function PluginOverview({
  wsId,
  refId,
  pkg,
  versionId,
  paths,
}: {
  wsId: string;
  refId: string;
  pkg: PluginPackage;
  versionId: string;
  paths: readonly string[];
}) {
  const { t } = useT("hub");
  const timeAgo = useTimeAgo();
  const latest = pkg.versions[0];
  const hasManifest = paths.includes(PLUGIN_MANIFEST_FILENAME);
  const hasReadme = paths.includes(PLUGIN_README_PATH);

  return (
    <div className={cn(OBJECT_PAGE_BODY, "flex flex-col gap-6")}>
      <ObjectSectionCard>
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 px-4 py-3 text-caption">
          <dt className="text-muted-foreground">{t(($) => $.overview.plugin_key)}</dt>
          <dd className="min-w-0 truncate font-mono">{pkg.plugin_key}</dd>
          <dt className="text-muted-foreground">{t(($) => $.overview.latest)}</dt>
          <dd className="font-mono">{latest?.version ?? ""}</dd>
          {latest ? (
            <>
              <dt className="text-muted-foreground">{t(($) => $.overview.published)}</dt>
              <dd>{timeAgo(latest.published_at)}</dd>
              <dt className="text-muted-foreground">{t(($) => $.item.checksum)}</dt>
              <dd className="font-mono" title={latest.digest}>
                {formatDigest(latest.digest)}
              </dd>
            </>
          ) : null}
          <dt className="text-muted-foreground">{t(($) => $.overview.versions)}</dt>
          <dd>{pkg.versions.length}</dd>
        </dl>
      </ObjectSectionCard>

      {hasManifest && (
        <ObjectSection title={t(($) => $.overview.manifest)}>
          <ObjectSectionCard>
            <PluginTextFile
              wsId={wsId}
              refId={refId}
              versionId={versionId}
              path={PLUGIN_MANIFEST_FILENAME}
              asManifest
            />
          </ObjectSectionCard>
        </ObjectSection>
      )}

      {hasReadme ? (
        <ObjectSection title={t(($) => $.overview.readme)}>
          <ObjectSectionCard>
            <PluginTextFile
              wsId={wsId}
              refId={refId}
              versionId={versionId}
              path={PLUGIN_README_PATH}
              asManifest={false}
            />
          </ObjectSectionCard>
        </ObjectSection>
      ) : (
        <ObjectSectionEmpty icon={File} description={t(($) => $.overview.no_readme)} />
      )}
    </div>
  );
}

/** Published versions of a plugin, newest first. Versions never change once published. */
export function PluginVersions({ wsId, pkg }: { wsId: string; pkg: PluginPackage }) {
  const { t } = useT("hub");
  const timeAgo = useTimeAgo();
  const installs = useQuery(pluginInstallationsOptions(wsId));
  const installedVersionIds = new Set(
    (installs.data?.plugins ?? [])
      .filter((install) => install.plugin_key === pkg.plugin_key)
      .map((install) => install.package_version_id),
  );
  return (
    <div className={OBJECT_PAGE_BODY}>
      <ObjectSectionCard>
        <div className="divide-y">
          {pkg.versions.map((version) => (
            <div key={version.id} className="flex min-h-11 flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2.5 text-caption transition-colors hover:bg-muted/30">
              <span className="font-mono font-medium">{version.version}</span>
              {installedVersionIds.has(version.id) && (
                <Badge variant="secondary">{t(($) => $.versions.installed)}</Badge>
              )}
              <span className="text-muted-foreground">{t(($) => $.versions.published, { time: timeAgo(version.published_at) })}</span>
              <span className="text-muted-foreground">{formatHubSize(version.size_bytes)}</span>
              <span className="ml-auto font-mono text-muted-foreground" title={version.digest}>
                {formatDigest(version.digest)}
              </span>
            </div>
          ))}
        </div>
      </ObjectSectionCard>
    </div>
  );
}

/** Agents that have this skill assigned. Read from the agent list, which already carries each agent's skills. */
export function SkillUsage({ wsId, refId }: { wsId: string; refId: string }) {
  const { t } = useT("hub");
  const agents = useQuery(agentListOptions(wsId));
  const assigned = useMemo(
    () => selectSkillAssignments(agents.data).get(refId) ?? [],
    [agents.data, refId],
  );
  return (
    <div className={OBJECT_PAGE_BODY}>
      <ObjectSection title={t(($) => $.usage.agents_title)} count={assigned.length}>
        {assigned.length === 0 ? (
          <ObjectSectionEmpty icon={Bot} description={t(($) => $.usage.agents_empty)} />
        ) : (
          <ObjectSectionCard>
            <div className="divide-y">
              {assigned.map((agent) => (
                <div key={agent.id} className="flex min-h-11 items-center gap-2.5 px-3 py-2 transition-colors hover:bg-muted/30">
                  <ActorAvatar
                    name={agent.name}
                    initials={agent.name.slice(0, 2).toUpperCase()}
                    avatarUrl={resolvePublicFileUrl(agent.avatar_url)}
                    isAgent
                    size="md"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-body font-medium">{agent.name}</div>
                    {agent.description && (
                      <div className="truncate text-caption text-muted-foreground">{agent.description}</div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </ObjectSectionCard>
        )}
      </ObjectSection>
    </div>
  );
}

/** Where a plugin is installed in this workspace. Installation state only; plugins have no per-agent usage. */
export function PluginUsage({ wsId, pluginKey }: { wsId: string; pluginKey: string }) {
  const { t } = useT("hub");
  const installs = useQuery(pluginInstallationsOptions(wsId));
  const rows = (installs.data?.plugins ?? []).filter((install) => install.plugin_key === pluginKey);
  return (
    <div className={OBJECT_PAGE_BODY}>
      <ObjectSection title={t(($) => $.usage.installs_title)} count={rows.length}>
        {rows.length === 0 ? (
          <ObjectSectionEmpty icon={Bot} description={t(($) => $.usage.installs_empty)} />
        ) : (
          <ObjectSectionCard>
            <div className="divide-y">
              {rows.map((install) => (
                <div key={install.id} className="flex min-h-11 flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2.5 text-caption transition-colors hover:bg-muted/30">
                  <span className="min-w-0 flex-1 truncate font-medium">{install.name}</span>
                  <span className="font-mono text-muted-foreground">
                    {t(($) => $.usage.install_version, { version: install.version })}
                  </span>
                  <Badge variant={install.enabled ? "secondary" : "outline"}>
                    {install.enabled ? t(($) => $.usage.enabled) : t(($) => $.usage.disabled)}
                  </Badge>
                </div>
              ))}
            </div>
          </ObjectSectionCard>
        )}
      </ObjectSection>
    </div>
  );
}
