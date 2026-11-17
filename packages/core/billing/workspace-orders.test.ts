// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "../api/client";
import {
  workspaceOrderKeys,
  workspaceOrderOptions,
  workspaceOrdersOptions,
  workspacePlanKeys,
  workspacePlansOptions,
} from "./workspace-orders-queries";

function stubFetchJson(body: unknown, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(typeof body === "string" ? body : JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

const client = () => new ApiClient("https://api.example.test");

const order = {
  id: "11111111-1111-4111-8111-111111111111",
  workspace_id: "22222222-2222-4222-8222-222222222222",
  kind: "subscribe",
  plan_key: "pro",
  interval: "month",
  seats: 2,
  amount: 298000,
  vat_amount: 29800,
  total_amount: 327800,
  currency: "vnd",
  status: "pending",
  provider: "manual",
  transfer_code: "MULABCDEFGH",
  bank_details: {
    bank_name: "Vietcombank",
    account_name: "CONG TY TNHH MULTICA",
    account_number: "0123456789",
  },
  buyer: {
    name: "ACME",
    tax_code: "0123456789",
    address: "1 Le Loi",
    invoice_email: "ke.toan@example.com",
  },
  period_start: "2026-08-15T10:30:45Z",
  period_end: "2026-09-15T10:30:45Z",
  expires_at: "2026-08-22T10:30:45Z",
  paid_at: null,
  created_at: "2026-08-15T10:30:45Z",
};

const plan = {
  key: "pro",
  name: "Pro",
  currency: "vnd",
  seat_price_month: 149000,
  seat_price_year: 1490000,
  seats_fixed: null,
  min_seats: 1,
  trial_days: 14,
  audience: "business",
};

describe("workspace plans", () => {
  it("reads the public plans into camelCase", async () => {
    stubFetchJson({ plans: [plan] });
    await expect(client().listWorkspacePlans("ws-1")).resolves.toEqual([
      {
        key: "pro",
        name: "Pro",
        currency: "vnd",
        seatPriceMonth: 149000,
        seatPriceYear: 1490000,
        seatsFixed: null,
        minSeats: 1,
        trialDays: 14,
        audience: "business",
        billingModel: "seat",
        usageRates: {},
        effectiveDiscountPct: 0,
        scheduledPriceChange: null,
      },
    ]);
    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toBe(
      "https://api.example.test/api/workspaces/ws-1/plans",
    );
  });

  it("keeps unknown keys and defaults the optional fields", async () => {
    stubFetchJson({
      plans: [{ key: "x", name: "X", currency: "vnd", surprise: true }],
    });
    await expect(client().listWorkspacePlans("ws-1")).resolves.toEqual([
      {
        key: "x",
        name: "X",
        currency: "vnd",
        seatPriceMonth: null,
        seatPriceYear: null,
        seatsFixed: null,
        minSeats: null,
        trialDays: 0,
        audience: "any",
        billingModel: "seat",
        usageRates: {},
        effectiveDiscountPct: 0,
        scheduledPriceChange: null,
      },
    ]);
  });

  it("parses scheduled_price_change when present and defaults null when absent or null", async () => {
    stubFetchJson({
      plans: [
        {
          ...plan,
          scheduled_price_change: {
            effective_from: "2027-01-01T00:00:00Z",
            seat_price_month: 199000,
            seat_price_year: 1990000,
          },
        },
        {
          ...plan,
          key: "business",
          name: "Business",
          scheduled_price_change: null,
        },
      ],
    });
    const parsed = await client().listWorkspacePlans("ws-1");
    expect(parsed?.[0]?.scheduledPriceChange).toEqual({
      effectiveFrom: "2027-01-01T00:00:00Z",
      seatPriceMonth: 199000,
      seatPriceYear: 1990000,
    });
    expect(parsed?.[1]?.scheduledPriceChange).toBeNull();
  });

  it("falls back to null when scheduled_price_change is malformed", async () => {
    for (const bad of [
      {},
      { effective_from: 123 },
      { effective_from: "" },
      { effective_from: "2027-01-01T00:00:00Z", seat_price_month: -5 },
      { effective_from: "2027-01-01T00:00:00Z", seat_price_year: "1990000" },
      "bad",
    ]) {
      stubFetchJson({
        plans: [{ ...plan, scheduled_price_change: bad }],
      });
      await expect(client().listWorkspacePlans("ws-1")).resolves.toBeNull();
    }
  });

  it("falls back to any for a missing or unknown audience", async () => {
    stubFetchJson({
      plans: [
        { key: "a", name: "A", currency: "vnd", audience: "enterprise" },
      ],
    });
    await expect(client().listWorkspacePlans("ws-1")).resolves.toEqual([
      expect.objectContaining({ key: "a", audience: "any" }),
    ]);
  });

  it("returns null, not an empty list, for a malformed body", async () => {
    for (const body of [
      { plans: "not-an-array" },
      { plans: [{ key: "", name: "X", currency: "vnd" }] },
      { plans: [{ key: "x", name: "X", currency: "vnd", seat_price_month: -1 }] },
      { plans: [{ key: "x", name: "X", currency: "vnd", seat_price_month: "149000" }] },
      [plan],
      null,
    ]) {
      stubFetchJson(body);
      await expect(client().listWorkspacePlans("ws-1")).resolves.toBeNull();
    }
  });
});

describe("workspace orders", () => {
  it("reads an order, VAT and bank details included", async () => {
    stubFetchJson({ orders: [order] });
    const orders = await client().listWorkspaceOrders("ws-1");
    expect(orders).toHaveLength(1);
    expect(orders?.[0]).toMatchObject({
      id: order.id,
      workspaceId: order.workspace_id,
      kind: "subscribe",
      planKey: "pro",
      seats: 2,
      amount: 298000,
      vatAmount: 29800,
      totalAmount: 327800,
      currency: "vnd",
      status: "pending",
      transferCode: "MULABCDEFGH",
      bankDetails: {
        bankName: "Vietcombank",
        accountName: "CONG TY TNHH MULTICA",
        accountNumber: "0123456789",
      },
      buyer: {
        name: "ACME",
        taxCode: "0123456789",
        address: "1 Le Loi",
        invoiceEmail: "ke.toan@example.com",
      },
      expiresAt: "2026-08-22T10:30:45Z",
      paidAt: null,
    });
    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toBe(
      "https://api.example.test/api/workspaces/ws-1/orders",
    );
  });

  it("defaults the buyer, the bank and the total when they are missing", async () => {
    const { buyer: _buyer, bank_details: _bank, total_amount: _total, ...bare } = order;
    stubFetchJson({ orders: [bare] });
    const orders = await client().listWorkspaceOrders("ws-1");
    expect(orders?.[0]?.buyer).toEqual({
      name: "",
      taxCode: "",
      address: "",
      invoiceEmail: "",
    });
    expect(orders?.[0]?.bankDetails).toEqual({
      bankName: "",
      accountName: "",
      accountNumber: "",
    });
    expect(orders?.[0]?.totalAmount).toBe(327800);
  });

  it("keeps an unknown status and kind as they came", async () => {
    stubFetchJson({ orders: [{ ...order, status: "disputed", kind: "gift" }] });
    const orders = await client().listWorkspaceOrders("ws-1");
    expect(orders?.[0]).toMatchObject({ status: "disputed", kind: "gift" });
  });

  it("returns null for a malformed list", async () => {
    for (const body of [
      { orders: {} },
      { orders: [{ ...order, amount: "298000" }] },
      { orders: [{ ...order, amount: 1.5 }] },
      { orders: [{ ...order, vat_amount: -1 }] },
      { orders: [{ ...order, id: undefined }] },
      { orders: [{ ...order, transfer_code: 7 }] },
      { orders: [{ ...order, expires_at: 20261231 }] },
      { orders: [{ ...order, buyer: "ACME" }] },
      [order],
    ]) {
      stubFetchJson(body);
      await expect(client().listWorkspaceOrders("ws-1")).resolves.toBeNull();
    }
  });

  it("reads one order, and returns null for a malformed one", async () => {
    stubFetchJson(order);
    await expect(client().getWorkspaceOrder("ws-1", order.id)).resolves.toMatchObject({
      id: order.id,
      status: "pending",
    });
    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toBe(
      `https://api.example.test/api/workspaces/ws-1/orders/${order.id}`,
    );

    stubFetchJson({ ...order, seats: "2" });
    await expect(client().getWorkspaceOrder("ws-1", order.id)).resolves.toBeNull();
    stubFetchJson({ orders: [order] });
    await expect(client().getWorkspaceOrder("ws-1", order.id)).resolves.toBeNull();
  });

  it("throws for a non-2xx answer, which is not an empty order list", async () => {
    stubFetchJson({ error: "insufficient permissions" }, 403);
    await expect(client().listWorkspaceOrders("ws-1")).rejects.toBeInstanceOf(ApiError);
  });
});

describe("the buyer of an order", () => {
  const buyer = {
    name: "ACME",
    taxCode: "0123456789",
    address: "1 Le Loi",
    invoiceEmail: "ke.toan@example.com",
  };

  it("sends the four fields in snake case and reads the order back", async () => {
    stubFetchJson(order);
    const saved = await client().updateWorkspaceOrderBuyer("ws-1", order.id, buyer);
    expect(saved?.buyer).toEqual(buyer);

    const [url, init] = vi.mocked(fetch).mock.calls[0] ?? [];
    expect(url).toBe(
      `https://api.example.test/api/workspaces/ws-1/orders/${order.id}/buyer`,
    );
    expect((init as RequestInit).method).toBe("PUT");
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({
      name: "ACME",
      tax_code: "0123456789",
      address: "1 Le Loi",
      invoice_email: "ke.toan@example.com",
    });
  });

  it("returns null for a malformed answer", async () => {
    stubFetchJson({ ok: true });
    await expect(
      client().updateWorkspaceOrderBuyer("ws-1", order.id, buyer),
    ).resolves.toBeNull();
  });

  it("throws on 409 order_not_pending", async () => {
    stubFetchJson(
      { error: "the order is no longer pending", code: "order_not_pending", status: "paid" },
      409,
    );
    await expect(
      client().updateWorkspaceOrderBuyer("ws-1", order.id, buyer),
    ).rejects.toMatchObject({ status: 409 });
  });
});

describe("checkout request", () => {
  it("sends plan_key and seats when given, and neither when not", async () => {
    const session = {
      request_id: "r",
      session_id: "s",
      url: "https://app.example.com/acme/billing/orders/r",
    };
    stubFetchJson(session);
    await client().createWorkspaceSubscriptionCheckout({
      interval: "year",
      idempotencyKey: "k1",
      planKey: "business",
      seats: 7,
    });
    const first = vi.mocked(fetch).mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(first.body))).toEqual({
      interval: "year",
      idempotency_key: "k1",
      plan_key: "business",
      seats: 7,
    });

    stubFetchJson(session);
    await client().createWorkspaceSubscriptionCheckout({
      interval: "month",
      idempotencyKey: "k2",
    });
    const second = vi.mocked(fetch).mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(second.body))).toEqual({
      interval: "month",
      idempotency_key: "k2",
    });
  });
});

describe("query keys", () => {
  it("the orders keys include the workspace id", () => {
    expect(workspaceOrderKeys.all("ws-1")).toContain("ws-1");
    expect(workspaceOrderKeys.list("ws-1")).toContain("ws-1");
    expect(workspaceOrderKeys.detail("ws-1", "order-1")).toEqual(
      expect.arrayContaining(["ws-1", "order-1"]),
    );
    expect(workspaceOrdersOptions("ws-1").queryKey).toContain("ws-1");
    expect(workspaceOrderOptions("ws-1", "order-1").queryKey).toContain("ws-1");
  });

  it("two workspaces never share an orders or plans key", () => {
    expect(workspaceOrdersOptions("ws-1").queryKey).not.toEqual(
      workspaceOrdersOptions("ws-2").queryKey,
    );
    expect(workspaceOrderOptions("ws-1", "o").queryKey).not.toEqual(
      workspaceOrderOptions("ws-2", "o").queryKey,
    );
    expect(workspacePlansOptions("ws-1").queryKey).toContain("ws-1");
    expect(workspacePlanKeys.list("ws-1")).not.toEqual(workspacePlanKeys.list("ws-2"));
  });

  it("the detail and list keys sit under one prefix, so one invalidation reaches both", () => {
    const prefix = workspaceOrderKeys.all("ws-1");
    expect(workspaceOrderKeys.list("ws-1").slice(0, prefix.length)).toEqual(prefix);
    expect(workspaceOrderKeys.detail("ws-1", "o").slice(0, prefix.length)).toEqual(prefix);
  });

  it("does not query without a workspace or an order id", () => {
    expect(workspaceOrdersOptions("").enabled).toBe(false);
    expect(workspacePlansOptions("").enabled).toBe(false);
    expect(workspaceOrderOptions("ws-1", "").enabled).toBe(false);
    expect(workspaceOrderOptions("ws-1", "o").enabled).toBe(true);
  });
});
