import { createContext, useContext } from "react";

// Layout owns the sidebar collapsed state and publishes it here so tables can
// switch between inline row actions (collapsed) and a menu (expanded).
const SidebarCollapsedContext = createContext(false);

export const SidebarCollapsedProvider = SidebarCollapsedContext.Provider;

export function useSidebarCollapsed(): boolean {
  return useContext(SidebarCollapsedContext);
}
