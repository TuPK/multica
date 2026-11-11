"use client";

import {
  createCollectionViewStore,
  type CollectionViewCore,
} from "../../collections/view-store";
import {
  EMPTY_HUB_LIST_FILTERS,
  HUB_LIST_SORT_DEFAULT_DIRECTION,
  type HubListFilters,
  type HubListSortField,
} from "../list";

// View preferences for the Hub list: list or catalog, sort, and filters.
// Persisted per workspace on this device. The search text is session-only (the
// shared collection store never persists it), so a query restored days later
// cannot hide rows without the user noticing. Server data never goes in here.

export type HubViewMode = "list" | "catalog";

export interface HubViewExtraState {
  view: HubViewMode;
}

export interface HubViewExtraActions {
  setView: (view: HubViewMode) => void;
  /** Replaces one filter dimension, for segmented controls that pick a single value. */
  setFilterValues: <K extends keyof HubListFilters>(key: K, values: HubListFilters[K]) => void;
}

export type HubViewState = CollectionViewCore<HubListSortField, never, HubListFilters> &
  HubViewExtraState &
  HubViewExtraActions;

export const useHubViewStore = createCollectionViewStore<
  HubListSortField,
  never,
  HubListFilters,
  HubViewExtraState,
  HubViewExtraActions
>({
  name: "multica_hub_view",
  defaultSortField: "updated",
  sortDefaultDirection: HUB_LIST_SORT_DEFAULT_DIRECTION,
  defaultHiddenColumns: [],
  emptyFilters: EMPTY_HUB_LIST_FILTERS,
  extraState: { view: "list" },
  extraActions: (set) => ({
    setView: (view) => set({ view } as Partial<HubViewState>),
    setFilterValues: (key, values) =>
      set((state) => ({ filters: { ...state.filters, [key]: values } }) as Partial<HubViewState>),
  }),
});
