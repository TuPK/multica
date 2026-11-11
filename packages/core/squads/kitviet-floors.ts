/**
 * The floors of the Kit Viet P2 trades (spec 9.2: one floor per trade, created on demand). They are not
 * in COMPANY_FLOOR_NAMES: that list is what the starter wizard renders with translated labels, and the
 * quickstart creates a squad's floor by name (ensureCompanyFloor) when the workspace lacks it. A P2
 * squad template states its floorName from this list. Names stay English, like the other floors, so a
 * rerun finds them. The workspace cap of floors (24, server CompanyMaxFloors) counts these too.
 */
export const KITVIET_P2_FLOOR_NAMES = {
  health: "Healthcare",
  construction: "Construction",
  fnb: "F&B",
  insurance: "Insurance",
  banking: "Banking",
  realestate: "Real Estate",
  hospitality: "Hospitality",
  trade: "Import & Export",
  factory: "Manufacturing",
  edu: "Education",
} as const;

export type KitvietP2FloorKey = keyof typeof KITVIET_P2_FLOOR_NAMES;

export const KITVIET_P2_FLOOR_NAME_LIST: readonly string[] = Object.values(KITVIET_P2_FLOOR_NAMES);
