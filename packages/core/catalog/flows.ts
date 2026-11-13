import { CATALOG_FLOWS } from "./generated/flows";
import type { CatalogFlow } from "./types";

/** The enforced flow of a squad template, by the squad template's id (for example `kv_trade`). */
export function flowForSquad(squadId: string): CatalogFlow | undefined {
  return Object.values(CATALOG_FLOWS).find((flow) => flow.squad === squadId);
}

/** The template_ref of the workflow row that runs this flow: `flow_<squad id>@<rev>`. */
export function flowTemplateRef(flow: Pick<CatalogFlow, "id" | "rev">): string {
  return `${flow.id}@${flow.rev}`;
}
