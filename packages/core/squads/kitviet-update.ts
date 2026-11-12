// The Kit Viet update entry points for the web and desktop squads page. Its own subpath keeps the
// squads index, and the workspace hooks it pulls in, out of that page's import graph.
export { hasKitvietSquad, useUpgradeKitvietSquads, type KitvietWorkspaceUpgradeEntry } from "./mutations";
