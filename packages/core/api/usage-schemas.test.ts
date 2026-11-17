// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import type { ZodType } from "zod";
import { WorkspaceSubscriptionSummarySchema, WorkspaceSubscriptionUsageSchema } from "./schemas";
import {
  WorkspaceOrderSchema,
  WorkspaceOrderUsageSchema,
  WorkspacePlansResponseSchema,
} from "../billing/workspace-orders-schemas";
import { parseWithFallback } from "./schema";

// The platform's contract fixtures (see billing-platform-contract.test.ts).
function fixture(name: string): Record<string, any> {
  const url = new URL(
    `../../../platform-management/contract/testdata/${name}`,
    import.meta.url,
  );
  return JSON.parse(readFileSync(fileURLToPath(url), "utf8"));
}

function parseOrNull<T>(raw: unknown, schema: ZodType<T>): T | null {
  return parseWithFallback<T | null>(raw, schema, null, { endpoint: "test" });
}

describe("usage in the subscription summary", () => {
  it("reads the billing model and the current period of a usage plan", () => {
    const summary = parseOrNull(fixture("summary_usage.json"), WorkspaceSubscriptionSummarySchema);
    expect(summary?.billingModel).toBe("usage");
    expect(summary?.usage).toMatchObject({
      currency: "vnd",
      accruedAmount: 1234567,
      creditLimit: 2000000,
      unpaidOrderId: "dddddddd-0000-4000-8000-0000000000a1",
    });
    expect(summary?.usage?.meters.map((meter) => meter.key)).toEqual([
      "tasks_run",
      "storage_bytes",
      "members",
    ]);
    expect(summary?.usage?.meters[0]).toMatchObject({ quantity: 2000, amount: 48000 });
  });

  it("reads a seat plan as seat billing with no usage", () => {
    const summary = parseOrNull(fixture("summary_free.json"), WorkspaceSubscriptionSummarySchema);
    expect(summary?.billingModel).toBe("seat");
    expect(summary?.usage).toBeNull();
  });

  it("returns null for a usage meter whose amount is not a number", () => {
    const body = fixture("summary_usage.json");
    body.usage.meters[0].amount = "48000";
    expect(parseOrNull(body, WorkspaceSubscriptionSummarySchema)).toBeNull();
  });

  it("returns null for a usage period without its start", () => {
    const body = fixture("summary_usage.json");
    delete body.usage.period_start;
    expect(parseOrNull(body, WorkspaceSubscriptionSummarySchema)).toBeNull();
  });

  it("returns null for a negative accrued amount", () => {
    const body = fixture("summary_usage.json");
    body.usage.accrued_amount = -1;
    expect(parseOrNull(body, WorkspaceSubscriptionSummarySchema)).toBeNull();
  });
});

describe("GET /api/cloud-subscriptions/usage", () => {
  it("reads the current period, meter by meter", () => {
    const usage = parseOrNull(fixture("usage.json"), WorkspaceSubscriptionUsageSchema);
    expect(usage?.billingModel).toBe("usage");
    expect(usage?.usage?.meters).toHaveLength(3);
    expect(usage?.usage?.meters[1]).toMatchObject({
      key: "storage_bytes",
      per: 1073741824,
      unitPrice: 3000,
      discountPct: 20,
    });
  });

  it("returns null when a meter is missing its quantity", () => {
    const body = fixture("usage.json");
    delete body.usage.meters[2].quantity;
    expect(parseOrNull(body, WorkspaceSubscriptionUsageSchema)).toBeNull();
  });

  it("returns null when the discount is outside 0 to 100", () => {
    const body = fixture("usage.json");
    body.usage.meters[0].discount_pct = 150;
    expect(parseOrNull(body, WorkspaceSubscriptionUsageSchema)).toBeNull();
  });
});

describe("GET /api/workspaces/{id}/orders/{orderId}/usage", () => {
  const body = {
    order_id: "dddddddd-0000-4000-8000-0000000000a1",
    lines: [
      {
        day: "2026-08-03",
        meter: "tasks_run",
        quantity: 120,
        unit_price: 30000,
        per: 1000,
        discount_pct: 20,
        net_amount: 2880,
        note: "",
      },
      {
        day: "2026-08-04",
        meter: "members",
        quantity: 10,
        unit_price: 49000,
        per: 1,
        discount_pct: 0,
        net_amount: 490000,
        note: "workspace_suspended",
      },
    ],
  };

  it("reads each day and meter of a usage invoice", () => {
    const parsed = parseOrNull(body, WorkspaceOrderUsageSchema);
    expect(parsed?.orderId).toBe("dddddddd-0000-4000-8000-0000000000a1");
    expect(parsed?.lines[0]).toMatchObject({
      day: "2026-08-03",
      meter: "tasks_run",
      netAmount: 2880,
      discountPct: 20,
    });
    expect(parsed?.lines[1]?.note).toBe("workspace_suspended");
  });

  it("returns null for a line whose net amount is fractional", () => {
    const malformed: Record<string, any> = structuredClone(body);
    malformed.lines[0].net_amount = 2880.5;
    expect(parseOrNull(malformed, WorkspaceOrderUsageSchema)).toBeNull();
  });

  it("returns null when the lines are not a list", () => {
    expect(parseOrNull({ ...body, lines: "none" }, WorkspaceOrderUsageSchema)).toBeNull();
  });
});

describe("usage orders", () => {
  const usageOrder = {
    id: "dddddddd-0000-4000-8000-0000000000a1",
    workspace_id: "77777777-7777-4777-8777-777777777777",
    kind: "usage",
    plan_key: "payg",
    interval: "month",
    seats: 0,
    amount: 1234567,
    vat_amount: 123457,
    currency: "vnd",
    status: "pending",
    provider: "manual",
    transfer_code: "MUL-USAGE-1",
    bank_details: {},
    buyer: {},
    period_start: "2026-08-01T00:00:00Z",
    period_end: "2026-09-01T00:00:00Z",
    expires_at: null,
    paid_at: null,
    created_at: "2026-09-02T00:00:00Z",
  };

  it("reads a usage invoice that never expires", () => {
    const order = parseOrNull(usageOrder, WorkspaceOrderSchema);
    expect(order?.kind).toBe("usage");
    expect(order?.expiresAt).toBeNull();
  });

  it("returns null for an order whose expiry is a number", () => {
    expect(parseOrNull({ ...usageOrder, expires_at: 1 }, WorkspaceOrderSchema)).toBeNull();
  });
});

describe("plans with usage rates", () => {
  const plans = {
    plans: [
      {
        key: "payg",
        name: "Pay as you go",
        currency: "vnd",
        seat_price_month: null,
        seat_price_year: null,
        seats_fixed: null,
        min_seats: null,
        trial_days: 0,
        audience: "any",
        billing_model: "usage",
        usage_rates: { tasks_run: { per: 1000, price: 30000 } },
        effective_discount_pct: 20,
      },
      {
        key: "unlimited",
        name: "Unlimited",
        currency: "vnd",
        seat_price_month: 249000,
        seat_price_year: 2490000,
        seats_fixed: null,
        min_seats: 1,
        trial_days: 14,
        audience: "any",
      },
    ],
  };

  it("reads the usage rates and the Business discount of a usage plan", () => {
    const parsed = parseOrNull(plans, WorkspacePlansResponseSchema);
    expect(parsed?.[0]).toMatchObject({
      billingModel: "usage",
      usageRates: { tasks_run: { per: 1000, price: 30000 } },
      effectiveDiscountPct: 20,
    });
  });

  it("reads a seat plan with no usage rates and no discount", () => {
    const parsed = parseOrNull(plans, WorkspacePlansResponseSchema);
    expect(parsed?.[1]).toMatchObject({ billingModel: "seat", usageRates: {}, effectiveDiscountPct: 0 });
  });

  it("returns null when the effective discount is above 100", () => {
    const body: Record<string, any> = structuredClone(plans);
    body.plans[0].effective_discount_pct = 150;
    expect(parseOrNull(body, WorkspacePlansResponseSchema)).toBeNull();
  });
});
