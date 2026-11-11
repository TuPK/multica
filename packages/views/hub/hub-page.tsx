"use client";

import { useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowUpDown, Filter, LayoutGrid, List, Package } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { cn } from "@multica/ui/lib/utils";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { useCurrentMember } from "@multica/core/permissions";
import { agentListOptions, selectSkillAssignments, skillListOptions } from "@multica/core/workspace/queries";
import { pluginInstallationsOptions, pluginPackagesOptions } from "@multica/core/plugins";
import {
  activeHubFilterCount,
  buildHubItems,
  buildHubListRows,
  buildHubUsage,
  filterHubListRows,
  hubItemMetaListOptions,
  hubListLabels,
  sortHubListRows,
  useHubViewStore,
  type HubItemKind,
  type HubListFilters,
  type HubListRow,
  type HubListSortField,
  type HubSourceType,
  type HubStatusFilter,
  type HubUsageFilter,
  type HubViewMode,
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

/** Above this many rows the list renders only the rows in view. */
export const HUB_VIRTUALIZE_AFTER = 200;
const ROW_HEIGHT = 56;
const ROW_GRID =
  "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 md:grid-cols-[minmax(0,1fr)_6rem_10rem_6rem_7rem]";

const SOURCE_ORDER: readonly HubSourceType[] = [
  "manual",
  "runtime_local",
  "github",
  "clawhub",
  "skills_sh",
  "url_zip",
  "registry",
  "upload",
  "plugin",
];
const STATUS_OPTIONS: readonly HubStatusFilter[] = ["update_available", "scan_warning", "tracking", "forked", "manual"];
const USAGE_OPTIONS: readonly HubUsageFilter[] = ["used", "unused"];
const SORT_FIELDS: readonly HubListSortField[] = ["name", "updated", "created", "usedBy", "source"];

export function HubPage() {
  const { t } = useT("hub");
  const wsId = useWorkspaceId();
  const [tab, setTab] = useState<Tab>("items");
  const [blankOpen, setBlankOpen] = useState(false);
  const navigation = useNavigation();
  const p = useWorkspacePaths();
  const { role } = useCurrentMember(wsId);
  const canManageSources = role === "owner" || role === "admin";

  // Preferences live in the persisted view store; the search text is session-only there.
  const view = useHubViewStore((state) => state.view);
  const setView = useHubViewStore((state) => state.setView);
  const filters = useHubViewStore((state) => state.filters);
  const setFilterValues = useHubViewStore((state) => state.setFilterValues);
  const toggleFilter = useHubViewStore((state) => state.toggleFilter);
  const clearFilters = useHubViewStore((state) => state.clearFilters);
  const query = useHubViewStore((state) => state.search);
  const setQuery = useHubViewStore((state) => state.setSearch);
  const sortField = useHubViewStore((state) => state.sortField);
  const sortDirection = useHubViewStore((state) => state.sortDirection);
  const setSortField = useHubViewStore((state) => state.setSortField);
  const setSortDirection = useHubViewStore((state) => state.setSortDirection);

  const skills = useQuery(skillListOptions(wsId));
  // The plugins backend sits behind a feature flag; its failure must not hide skills.
  const plugins = useQuery({ ...pluginPackagesOptions(wsId), retry: false });
  // Usage and install state refine the list; without them every item reads as unused.
  const agents = useQuery({ ...agentListOptions(wsId), retry: false });
  const installations = useQuery({ ...pluginInstallationsOptions(wsId), retry: false });
  // Meta is an overlay: a failure here leaves the list usable, just without update badges.
  const metaQuery = useQuery({ ...hubItemMetaListOptions(wsId), retry: false });

  const all = useMemo(
    () => buildHubItems(skills.data ?? [], plugins.data?.packages ?? []),
    [skills.data, plugins.data],
  );
  const rows = useMemo(
    () =>
      buildHubListRows(
        all,
        metaQuery.data ?? [],
        buildHubUsage(
          selectSkillAssignments(agents.data),
          plugins.data?.packages ?? [],
          installations.data?.plugins ?? [],
        ),
      ),
    [all, metaQuery.data, agents.data, plugins.data, installations.data],
  );
  const items = useMemo(
    () => sortHubListRows(filterHubListRows(rows, filters, query), sortField, sortDirection),
    [rows, filters, query, sortField, sortDirection],
  );
  const labels = useMemo(() => hubListLabels(rows), [rows]);
  const sources = useMemo(() => {
    const present = new Set(rows.map((row) => row.source));
    return SOURCE_ORDER.filter((source) => present.has(source) || filters.sources.includes(source));
  }, [rows, filters.sources]);
  const activeCount = activeHubFilterCount(filters, query);
  const updatesOn = filters.statuses.includes("update_available");

  const loading = skills.isLoading;
  const failed = skills.isError;
  const resetAll = () => {
    clearFilters();
    setQuery("");
  };

  let content: ReactNode;
  if (tab === "sources") {
    content = (
      <div className="min-h-0 flex-1 overflow-y-auto">
        <HubSourcesManager wsId={wsId} canManage={canManageSources} />
      </div>
    );
  } else if (tab === "activity") {
    content = (
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className={cn("py-4", PAGE_GUTTER)}>
          <HubActivityList wsId={wsId} showItem />
        </div>
      </div>
    );
  } else if (failed) {
    content = (
      <div className="min-h-0 flex-1 overflow-y-auto">
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
      </div>
    );
  } else if (loading) {
    content = null;
  } else if (all.length === 0) {
    content = (
      <div className="min-h-0 flex-1 overflow-y-auto">
        <CollectionPageState icon={Package} title={t(($) => $.empty_title)} />
      </div>
    );
  } else if (items.length === 0) {
    content = (
      <div className="min-h-0 flex-1 overflow-y-auto">
        <CollectionNoMatches
          title={t(($) => $.no_matches)}
          hasSearch={query.trim() !== ""}
          hasFilters={activeCount > (query.trim() !== "" ? 1 : 0)}
          onReset={resetAll}
        />
      </div>
    );
  } else if (view === "list") {
    content = <ItemList items={items} />;
  } else {
    content = (
      <div className="min-h-0 flex-1 overflow-y-auto">
        <ItemCatalog items={items} />
      </div>
    );
  }

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
              value={filters.kinds.length === 1 ? filters.kinds[0]! : "all"}
              onChange={(next) => setFilterValues("kinds", next === "all" ? [] : [next as HubItemKind])}
              options={[
                { value: "all", label: t(($) => $.kinds.all) },
                { value: "skill", label: t(($) => $.kinds.skill) },
                { value: "plugin", label: t(($) => $.kinds.plugin) },
              ]}
            />
            <FilterMenu filters={filters} sources={sources} labels={labels} onToggle={toggleFilter} />
            <Button
              variant={updatesOn ? "secondary" : "ghost"}
              size="sm"
              aria-pressed={updatesOn}
              onClick={() => toggleFilter("statuses", "update_available")}
            >
              {t(($) => $.update.filter)}
            </Button>
            <SortMenu value={sortField} direction={sortDirection} onChange={setSortField} />
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t(($) => $.list.sort_direction)}
              title={t(($) => $.list.sort_direction)}
              onClick={() => setSortDirection(sortDirection === "asc" ? "desc" : "asc")}
            >
              <ArrowUpDown aria-hidden="true" />
            </Button>
            {activeCount > 0 ? (
              <Button variant="ghost" size="sm" onClick={resetAll}>
                {t(($) => $.list.clear)}
              </Button>
            ) : null}
          </>
        ) : null}
        <div className="ml-auto flex items-center gap-3">
          {tab === "items" && !loading && !failed && all.length > 0 ? (
            <span className="text-caption text-muted-foreground" aria-live="polite">
              {t(($) => $.list.count, { shown: items.length, total: rows.length })}
            </span>
          ) : null}
          <HubQuotaMeter wsId={wsId} />
        </div>
      </div>
      {content}
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

function ViewToggle({ view, onChange }: { view: HubViewMode; onChange: (view: HubViewMode) => void }) {
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
  direction,
  onChange,
}: {
  value: HubListSortField;
  direction: "asc" | "desc";
  onChange: (value: HubListSortField) => void;
}) {
  const { t } = useT("hub");
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="sm" />}>
        {t(($) => $.sort.label)}: {t(($) => $.sort[value])}
        {direction === "asc" ? " ↑" : " ↓"}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuRadioGroup value={value} onValueChange={(next) => onChange(next as HubListSortField)}>
          {SORT_FIELDS.map((field) => (
            <DropdownMenuRadioItem key={field} value={field}>
              {t(($) => $.sort[field])}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * Multi-select filters for source, label, status and usage. Each choice toggles one value
 * in its dimension, and the list keeps the menu open so several can be set in one visit.
 */
function FilterMenu({
  filters,
  sources,
  labels,
  onToggle,
}: {
  filters: HubListFilters;
  sources: readonly HubSourceType[];
  labels: readonly string[];
  onToggle: (key: keyof HubListFilters, value: string) => void;
}) {
  const { t } = useT("hub");
  const count = filters.sources.length + filters.labels.length + filters.statuses.length + filters.usage.length;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant={count > 0 ? "secondary" : "ghost"} size="sm" />}>
        <Filter aria-hidden="true" />
        {t(($) => $.list.filters)}
        {count > 0 ? <span className="tabular-nums">({count})</span> : null}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-96 overflow-y-auto">
        <FilterSection label={t(($) => $.list.filter_source)}>
          {sources.map((source) => (
            <DropdownMenuCheckboxItem
              key={source}
              checked={filters.sources.includes(source)}
              onCheckedChange={() => onToggle("sources", source)}
            >
              {t(($) => $.source_types[source])}
            </DropdownMenuCheckboxItem>
          ))}
        </FilterSection>
        {labels.length > 0 ? (
          <>
            <DropdownMenuSeparator />
            <FilterSection label={t(($) => $.list.filter_label)}>
              {labels.map((label) => (
                <DropdownMenuCheckboxItem
                  key={label}
                  checked={filters.labels.includes(label)}
                  onCheckedChange={() => onToggle("labels", label)}
                >
                  {label}
                </DropdownMenuCheckboxItem>
              ))}
            </FilterSection>
          </>
        ) : null}
        <DropdownMenuSeparator />
        <FilterSection label={t(($) => $.list.filter_status)}>
          {STATUS_OPTIONS.map((status) => (
            <DropdownMenuCheckboxItem
              key={status}
              checked={filters.statuses.includes(status)}
              onCheckedChange={() => onToggle("statuses", status)}
            >
              {t(($) => $.list.status[status])}
            </DropdownMenuCheckboxItem>
          ))}
        </FilterSection>
        <DropdownMenuSeparator />
        <FilterSection label={t(($) => $.list.filter_usage)}>
          {USAGE_OPTIONS.map((usage) => (
            <DropdownMenuCheckboxItem
              key={usage}
              checked={filters.usage.includes(usage)}
              onCheckedChange={() => onToggle("usage", usage)}
            >
              {t(($) => $.list.usage[usage])}
            </DropdownMenuCheckboxItem>
          ))}
        </FilterSection>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function FilterSection({ label, children }: { label: string; children: ReactNode }) {
  return (
    <DropdownMenuGroup>
      <DropdownMenuLabel>{label}</DropdownMenuLabel>
      {children}
    </DropdownMenuGroup>
  );
}

function useItemHref() {
  const p = useWorkspacePaths();
  // Skills keep their existing detail page; plugins open the Hub item page.
  return (item: HubListRow) =>
    item.kind === "skill" ? p.skillDetail(item.refId) : p.hubItem("plugin", item.refId);
}

function ItemName({ item }: { item: HubListRow }) {
  const href = useItemHref()(item);
  return href ? (
    <AppLink href={href} className="truncate font-medium hover:underline">
      {item.name}
    </AppLink>
  ) : (
    <span className="truncate font-medium">{item.name}</span>
  );
}

function SourceBadge({ item }: { item: HubListRow }) {
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

/**
 * The list is a grid of rows. Past HUB_VIRTUALIZE_AFTER rows only the rows in view are
 * mounted; the header stays fixed above the scrolling body.
 */
function ItemList({ items }: { items: HubListRow[] }) {
  const { t } = useT("hub");
  const timeAgo = useTimeAgo();
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtual = items.length > HUB_VIRTUALIZE_AFTER;
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 10,
  });
  const visibleRows = virtual
    ? virtualizer.getVirtualItems().map((row) => ({ index: row.index, start: row.start }))
    : items.map((_, index) => ({ index, start: index * ROW_HEIGHT }));
  const totalHeight = virtual ? virtualizer.getTotalSize() : items.length * ROW_HEIGHT;

  return (
    <div role="table" aria-label={t(($) => $.list.table_label)} className="flex min-h-0 flex-1 flex-col text-label">
      <div role="rowgroup" className="border-b text-left text-caption text-muted-foreground">
        <div role="row" className={cn(ROW_GRID, "h-9", PAGE_GUTTER)}>
          <div role="columnheader" className="min-w-0 font-normal">{t(($) => $.columns.name)}</div>
          <div role="columnheader" className="hidden font-normal md:block">{t(($) => $.columns.kind)}</div>
          <div role="columnheader" className="hidden font-normal md:block">{t(($) => $.columns.source)}</div>
          <div role="columnheader" className="hidden font-normal md:block">{t(($) => $.columns.version)}</div>
          <div role="columnheader" className="hidden font-normal md:block">{t(($) => $.columns.updated)}</div>
        </div>
      </div>
      <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto">
        <div role="rowgroup" className="relative w-full" style={{ height: totalHeight }}>
          {visibleRows.map(({ index, start }) => {
            const item = items[index]!;
            const rowStyle: CSSProperties = { top: 0, height: ROW_HEIGHT, transform: `translateY(${start}px)` };
            return (
              <div
                key={item.key}
                role="row"
                aria-rowindex={index + 2}
                style={rowStyle}
                className={cn(ROW_GRID, "absolute inset-x-0 border-b hover:bg-muted/50", PAGE_GUTTER)}
              >
                <div role="cell" className="flex min-w-0 flex-col justify-center">
                  <span className="flex min-w-0 items-center gap-2">
                    <ItemName item={item} />
                    {item.updateAvailable ? <UpdateAvailableBadge /> : null}
                  </span>
                  {item.description ? (
                    <span className="truncate text-caption text-muted-foreground">{item.description}</span>
                  ) : null}
                </div>
                <div role="cell" className="hidden md:block">{t(($) => $.kind_singular[item.kind])}</div>
                <div role="cell" className="hidden min-w-0 md:block">
                  <SourceBadge item={item} />
                </div>
                <div role="cell" className="hidden text-muted-foreground md:block">{item.version ?? ""}</div>
                <div role="cell" className="hidden text-muted-foreground md:block">{timeAgo(item.updatedAt)}</div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function ItemCatalog({ items }: { items: HubListRow[] }) {
  const { t } = useT("hub");
  return (
    <ul className={cn("grid gap-3 py-4 sm:grid-cols-2 xl:grid-cols-3", PAGE_GUTTER)}>
      {items.map((item) => (
        <li key={item.key} className="flex min-w-0 flex-col gap-2 rounded-lg border bg-card p-3">
          <div className="flex min-w-0 items-center justify-between gap-2">
            <ItemName item={item} />
            {item.updateAvailable ? <UpdateAvailableBadge /> : null}
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
