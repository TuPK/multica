import { CATALOG_CONTRACTS, type CatalogContractProvider } from "./generated/contracts";

/**
 * The contracts the client code depends on (catalog `contracts` field, spec 6.3.4). The server lists
 * its own in server/internal/catalog/contracts.go. A template that provides one must stay in the
 * catalog under that id and stay stable, or the quickstart breaks for a customer.
 */
export const REQUIRED_CONTRACTS = [
  "squad.sdlc",
  "workflow.sdlc_auto",
  "squad.planning",
  "workflow.planning_auto",
  "company.executive_office",
] as const;

export type ContractId = (typeof REQUIRED_CONTRACTS)[number];

/** The stable templates that provide a contract. */
export function contractProviders(id: string): readonly CatalogContractProvider[] {
  return (CATALOG_CONTRACTS[id] ?? []).filter((p) => p.status === "stable");
}
