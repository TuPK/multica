/**
 * The recipe of a sales source, version 1: how the collector reads one source. The Go collector
 * (server/internal/salescollect) validates the same shape; this type is what the quickstart writes
 * into a source document and what the Source Studio shows. A recipe never holds a secret: a token
 * or a key lives in the connection that the agent or the workspace owns.
 */

export const SALES_FAMILIES = [
  "hiring",
  "funding",
  "expansion",
  "launch",
  "regulatory",
  "tender",
  "event",
  "pain",
  "site_change",
  "registry",
  "competitor_customer",
  "trade_flow",
  "leadership_change",
  "directory",
  "own_data",
  "inbound",
  "ad_activity",
] as const;

export type SalesFamily = (typeof SALES_FAMILIES)[number];

export const SALES_METHODS = [
  "rss",
  "json_api",
  "html_jsonld",
  "html_list",
  "csv_drop",
  "kb_internal",
  "web_search",
  "social_connector",
  "provider",
  "human_share",
  "mailbox",
] as const;

export type SalesMethod = (typeof SALES_METHODS)[number];

/** Methods the collector runs itself. The others run inside an agent task. */
export const COLLECTOR_METHODS: readonly SalesMethod[] = ["rss", "json_api", "html_jsonld", "html_list", "csv_drop"];

export interface SalesRecipePagination {
  type: "none" | "page" | "cursor";
  max_pages: number;
  param?: string;
}

export interface SalesRecipe {
  version: 1;
  rung: number;
  method: SalesMethod;
  fetch: {
    engine: "http" | "crawl4ai";
    url: string;
    params?: Record<string, string>;
    pagination?: SalesRecipePagination;
    render: "none" | "browser";
  };
  parse: {
    /** A selector (html_list), a dot path (json_api), a schema.org type (html_jsonld), or "" for a feed or a top-level array. */
    items: string;
    fields: {
      entity: string;
      url: string;
      observed_at: string;
      text: string;
      website?: string;
    };
    schema?: Record<string, unknown>;
  };
  filter: {
    keywords_any: string[];
    keywords_none: string[];
    language: string;
    since_days: number;
  };
  map: {
    family: SalesFamily | null;
    entity_from: "page_name" | "link_domain" | "author" | "field";
  };
  provider?: {
    toolkit: string;
    actor: string;
    input: Record<string, unknown>;
    cost_cap_usd: number;
  };
  cadence: "daily" | "weekly" | "manual";
  rate_limit: string;
  max_items: number;
  verified_at: string | null;
  sample_count: number;
  field_coverage: number;
  fallbacks: { rung: number }[];
}

/** Keys a recipe must never hold, at any depth. */
const SECRET_KEY_PATTERN = /token|api[_-]?key|apikey|password|secret|cookie|authorization|session/i;

/** The first secret-looking key in a recipe, by its path, or null. */
export function findSecretKey(value: unknown, path = "recipe"): string | null {
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i += 1) {
      const hit = findSecretKey(value[i], `${path}[${i}]`);
      if (hit) return hit;
    }
    return null;
  }
  if (value && typeof value === "object") {
    for (const [key, child] of Object.entries(value)) {
      if (SECRET_KEY_PATTERN.test(key)) return `${path}.${key}`;
      const hit = findSecretKey(child, `${path}.${key}`);
      if (hit) return hit;
    }
  }
  return null;
}

/** A recipe skeleton for a source the squad has not verified yet. It runs nothing until a probe passes. */
export function skeletonRecipe(input: {
  rung: number;
  method: SalesMethod;
  url: string;
  family: SalesFamily | null;
  items?: string;
  fields?: Partial<SalesRecipe["parse"]["fields"]>;
  engine?: SalesRecipe["fetch"]["engine"];
  maxItems?: number;
}): SalesRecipe {
  return {
    version: 1,
    rung: input.rung,
    method: input.method,
    fetch: { engine: input.engine ?? "http", url: input.url, render: "none" },
    parse: {
      items: input.items ?? "",
      fields: {
        entity: input.fields?.entity ?? "",
        url: input.fields?.url ?? "",
        observed_at: input.fields?.observed_at ?? "",
        text: input.fields?.text ?? "",
        ...(input.fields?.website ? { website: input.fields.website } : {}),
      },
    },
    filter: { keywords_any: [], keywords_none: [], language: "", since_days: 30 },
    map: { family: input.family, entity_from: "field" },
    cadence: "daily",
    rate_limit: "1/s",
    max_items: input.maxItems ?? 100,
    verified_at: null,
    sample_count: 0,
    field_coverage: 0,
    fallbacks: [],
  };
}
