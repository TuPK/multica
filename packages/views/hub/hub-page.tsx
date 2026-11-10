"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { LayoutGrid, List, Package } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { cn } from "@multica/ui/lib/utils";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { useCurrentMember } from "@multica/core/permissions";
import { skillListOptions } from "@multica/core/workspace/queries";
import { pluginPackagesOptions } from "@multica/core/plugins";
import {
  buildHubItems,
  EMPTY_HUB_FILTERS,
  filterHubItems,
  hubItemMetaListOptions,
  sortHubItems,
  type HubItem,
  type HubItemKind,
  type HubSortField,
} from "@multica/core/hub";
import { CreateSkillDialog } from "../skills/components/create-skill-dialog";
import { AppLink, useNavigation } from "../navigation";
import { useT, useTimeAgo } from "../i18n";
import {
  CollectionNoMatches,
  CollectionPageHeader,
  CollectionPageState,
  CollectionSearch,
} from "../layout/collection-page";
import { PAGE_GUTTER } from "../layout/page-header";
import { AddToHubDialog } from "./add-to-hub-dialog";
import { HubActivityList } from "./hub-activity-list";
import { HubExportAllButton } from "./hub-export-actions";
import { HubQuotaMeter } from "./hub-quota-meter";
import { HubSourcesManager } from "./hub-sources-manager";
import { UpdateAvailableBadge } from "./hub-update-banner";

type Tab = "items" | "sources" | "activity";
type ViewMode = "list" | "catalog";

const SORT_DEFAULT_DIRECTION: Record<HubSortField, "asc" | "desc"> = {
  name: "asc",
  updated: "desc",
  created: "desc",
};

export function HubPage() {
  const { t } = useT("hub");
  const wsId = useWorkspaceId();
  const [tab, setTab] = useState<Tab>("items");
  const [view, setView] = useState<ViewMode>("list");
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState<"all" | HubItemKind>("all");
  const [sortField, setSortField] = useState<HubSortField>("updated");
  const [blankOpen, setBlankOpen] = useState(false);
  const [updatesOnly, setUpdatesOnly] = useState(false);
  const navigation = useNavigation();
  const p = useWorkspacePaths();
  const { role } = useCurrentMember(wsId);
  const canManageSources = role === "owner" || role === "admin";

  const skills = useQuery(skillListOptions(wsId));
  // The plugins backend sits behind a feature flag; its failure must not hide skills.
  const plugins = useQuery({ ...pluginPackagesOptions(wsId), retry: false });

  const all = useMemo(
    () => buildHubItems(skills.data ?? [], plugins.data?.packages ?? []),
    [skills.data, plugins.data],
  );
  const filters = useMemo(
    () => ({ ...EMPTY_HUB_FILTERS, query, kinds: kind === "all" ? [] : [kind] }),
    [query, kind],
  );
  // Meta is an overlay: a failure here leaves the list usable, just without update badges.
  const metaQuery = useQuery({ ...hubItemMetaListOptions(wsId), retry: false });
  const updateKeys = useMemo(
    () =>
      new Set(
        (metaQuery.data ?? []).filter((meta) => meta.update_available).map((meta) => `${meta.kind}:${meta.ref_id}`),
      ),
    [metaQuery.data],
  );
  const items = useMemo(() => {
    const sorted = sortHubItems(filterHubItems(all, filters), sortField, SORT_DEFAULT_DIRECTION[sortField]);
    return updatesOnly ? sorted.filter((item) => updateKeys.has(item.key)) : sorted;
  }, [all, filters, sortField, updatesOnly, updateKeys]);

  const loading = skills.isLoading;
  const failed = skills.isError;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <CollectionPageHeader
        icon={Package}
        title={t(($) => $.title)}
        tools={<CollectionSearch value={query} onChange={setQuery} label={t(($) => $.search_placeholder)} />}
        actions={
          <div className="flex items-center gap-2">
            {tab === "items" ? <ViewToggle view={view} onChange={setView} /> : null}
            {canManageSources ? <HubExportAllButton wsId={wsId} /> : null}
            <AddToHubDialog onBlank={() => setBlankOpen(true)} />
          </div>
        }
      />
      <div className={cn("flex flex-wrap items-center gap-2 border-b py-2", PAGE_GUTTER)}>
        <Segmented
          value={tab}
          onChange={setTab}
          options={[
            { value: "items", label: t(($) => $.tabs.items) },
            { value: "sources", label: t(($) => $.tabs.sources) },
            { value: "activity", label: t(($) => $.activity.tab) },
          ]}
        />
        {tab === "items" ? (
          <>
            <Segmented
              value={kind}
              onChange={setKind}
              options={[
                { value: "all", label: t(($) => $.kinds.all) },
                { value: "skill", label: t(($) => $.kinds.skill) },
                { value: "plugin", label: t(($) => $.kinds.plugin) },
              ]}
            />
            <Button
              variant={updatesOnly ? "secondary" : "ghost"}
              size="sm"
              aria-pressed={updatesOnly}
              onClick={() => setUpdatesOnly((on) => !on)}
            >
              {t(($) => $.update.filter)}
            </Button>
            <SortMenu value={sortField} onChange={setSortField} />
          </>
        ) : null}
        <div className="ml-auto">
          <HubQuotaMeter wsId={wsId} />
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {tab === "sources" ? (
          <HubSourcesManager wsId={wsId} canManage={canManageSources} />
        ) : tab === "activity" ? (
          <div className={cn("py-4", PAGE_GUTTER)}>
            <HubActivityList wsId={wsId} showItem />
          </div>
        ) : failed ? (
          <CollectionPageState
            icon={Package}
            tone="destructive"
            title={t(($) => $.load_error)}
            actions={
              <Button variant="outline" size="sm" onClick={() => void skills.refetch()}>
                {t(($) => $.retry)}
              </Button>
            }
          />
        ) : loading ? null : all.length === 0 ? (
          <CollectionPageState icon={Package} title={t(($) => $.empty_title)} />
        ) : items.length === 0 ? (
          <CollectionNoMatches
            title={t(($) => $.no_matches)}
            hasSearch={query !== ""}
            hasFilters={kind !== "all" || updatesOnly}
            onReset={() => {
              setQuery("");
              setKind("all");
              setUpdatesOnly(false);
            }}
          />
        ) : view === "list" ? (
          <ItemList items={items} updateKeys={updateKeys} />
        ) : (
          <ItemCatalog items={items} updateKeys={updateKeys} />
        )}
      </div>
      {blankOpen ? (
        <CreateSkillDialog
          initialMethod="manual"
          onClose={() => setBlankOpen(false)}
          onCreated={(skill) => navigation.push(p.hubItem("skill", skill.id))}
        />
      ) : null}
    </div>
  );
}

function Segmented<T extends string>({
  value,
  onChange,
  options,
}: {
  value: T;
  onChange: (value: T) => void;
  options: { value: T; label: string }[];
}) {
  return (
    <div role="group" className="flex items-center gap-0.5 rounded-md bg-muted p-0.5">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={option.value === value}
          onClick={() => onChange(option.value)}
          className={cn(
            "h-6 rounded-sm px-2 text-label text-muted-foreground transition-colors hover:text-foreground",
            option.value === value && "bg-background text-foreground shadow-xs",
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

function ViewToggle({ view, onChange }: { view: ViewMode; onChange: (view: ViewMode) => void }) {
  const { t } = useT("hub");
  return (
    <div role="group" aria-label={t(($) => $.view_label)} className="flex items-center gap-0.5">
      <Button
        variant={view === "list" ? "secondary" : "ghost"}
        size="icon-sm"
        aria-label={t(($) => $.view.list)}
        aria-pressed={view === "list"}
        onClick={() => onChange("list")}
      >
        <List />
      </Button>
      <Button
        variant={view === "catalog" ? "secondary" : "ghost"}
        size="icon-sm"
        aria-label={t(($) => $.view.catalog)}
        aria-pressed={view === "catalog"}
        onClick={() => onChange("catalog")}
      >
        <LayoutGrid />
      </Button>
    </div>
  );
}

function SortMenu({
  value,
  onChange,
}: {
  value: HubSortField;
  onChange: (value: HubSortField) => void;
}) {
  const { t } = useT("hub");
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="sm" />}>
        {t(($) => $.sort.label)}: {t(($) => $.sort[value])}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuRadioGroup value={value} onValueChange={(next) => onChange(next as HubSortField)}>
          {(["name", "updated", "created"] as const).map((field) => (
            <DropdownMenuRadioItem key={field} value={field}>
              {t(($) => $.sort[field])}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function useItemHref() {
  const p = useWorkspacePaths();
  // Skills keep their existing detail page; plugins open the Hub item page.
  return (item: HubItem) =>
    item.kind === "skill" ? p.skillDetail(item.refId) : p.hubItem("plugin", item.refId);
}

function ItemName({ item }: { item: HubItem }) {
  const href = useItemHref()(item);
  return href ? (
    <AppLink href={href} className="truncate font-medium hover:underline">
      {item.name}
    </AppLink>
  ) : (
    <span className="truncate font-medium">{item.name}</span>
  );
}

function SourceBadge({ item }: { item: HubItem }) {
  const { t } = useT("hub");
  const label = t(($) => $.source_types[item.source]);
  const className = "truncate rounded-sm bg-muted px-1.5 py-0.5 text-caption text-muted-foreground";
  return item.sourceUrl ? (
    <a
      href={item.sourceUrl}
      target="_blank"
      rel="noreferrer noopener"
      title={t(($) => $.open_source)}
      className={cn(className, "hover:text-foreground")}
    >
      {label}
    </a>
  ) : (
    <span className={className}>{label}</span>
  );
}

function ItemList({ items, updateKeys }: { items: HubItem[]; updateKeys: ReadonlySet<string> }) {
  const { t } = useT("hub");
  const timeAgo = useTimeAgo();
  return (
    <table className="w-full table-fixed text-label">
      <thead className="sticky top-0 bg-background text-left text-caption text-muted-foreground">
        <tr className="border-b">
          <th className={cn("py-2 font-normal", PAGE_GUTTER)}>{t(($) => $.columns.name)}</th>
          <th className="w-24 py-2 font-normal">{t(($) => $.columns.kind)}</th>
          <th className="w-40 py-2 font-normal">{t(($) => $.columns.source)}</th>
          <th className="w-24 py-2 font-normal">{t(($) => $.columns.version)}</th>
          <th className="w-28 py-2 pr-4 font-normal">{t(($) => $.columns.updated)}</th>
        </tr>
      </thead>
      <tbody>
        {items.map((item) => (
          <tr key={item.key} className="border-b last:border-b-0 hover:bg-muted/50">
            <td className={cn("flex min-w-0 flex-col py-2", PAGE_GUTTER)}>
              <span className="flex min-w-0 items-center gap-2">
                <ItemName item={item} />
                {updateKeys.has(item.key) ? <UpdateAvailableBadge /> : null}
              </span>
              {item.description ? (
                <span className="truncate text-caption text-muted-foreground">{item.description}</span>
              ) : null}
            </td>
            <td className="py-2">{t(($) => $.kind_singular[item.kind])}</td>
            <td className="py-2 pr-2">
              <SourceBadge item={item} />
            </td>
            <td className="py-2 text-muted-foreground">{item.version ?? ""}</td>
            <td className="py-2 pr-4 text-muted-foreground">{timeAgo(item.updatedAt)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function ItemCatalog({ items, updateKeys }: { items: HubItem[]; updateKeys: ReadonlySet<string> }) {
  const { t } = useT("hub");
  return (
    <ul className={cn("grid gap-3 py-4 sm:grid-cols-2 xl:grid-cols-3", PAGE_GUTTER)}>
      {items.map((item) => (
        <li key={item.key} className="flex min-w-0 flex-col gap-2 rounded-lg border bg-card p-3">
          <div className="flex min-w-0 items-center justify-between gap-2">
            <ItemName item={item} />
            {updateKeys.has(item.key) ? <UpdateAvailableBadge /> : null}
            <span className="shrink-0 text-caption text-muted-foreground">
              {t(($) => $.kind_singular[item.kind])}
            </span>
          </div>
          {item.description ? (
            <p className="line-clamp-2 text-label text-muted-foreground">{item.description}</p>
          ) : null}
          <div className="mt-auto flex items-center gap-2">
            <SourceBadge item={item} />
            {item.version ? <span className="text-caption text-muted-foreground">{item.version}</span> : null}
          </div>
        </li>
      ))}
    </ul>
  );
}
