// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  CreateWorkspaceSubscriptionCheckoutResponseSchema,
  CreateWorkspaceSubscriptionPortalResponseSchema,
  PurchaseWorkspaceSeatsResponseSchema,
  WorkspaceSeatPurchasePreviewSchema,
  WorkspaceSubscriptionPricesSchema,
  WorkspaceSubscriptionSeatReconcileResultSchema,
  WorkspaceSubscriptionSummarySchema,
} from "./schemas";

// The platform's own responses, written by its Go test (go test ./contract
// -update in platform-management). Each of its billing routes has one, and each
// must decode through the schema the client reads that route with, so a change
// to the wire format that the client cannot read fails here.
function fixture(name: string): Record<string, any> {
  const url = new URL(
    `../../../platform-management/contract/testdata/${name}`,
    import.meta.url,
  );
  return JSON.parse(readFileSync(fileURLToPath(url), "utf8"));
}

describe("platform billing routes through the client's zod schemas", () => {
  it("GET summary of a free workspace with no subscription row", () => {
    const result = WorkspaceSubscriptionSummarySchema.safeParse(
      fixture("summary_free.json"),
    );
    expect(result.success).toBe(true);
    if (!result.success) return;
    const summary = result.data;
    // The row's version is 0, and the page needs a positive one.
    expect(summary.seatCapacity?.version).toBeGreaterThan(0);
    expect(summary.entitlement.version).toBe(0);
    expect(summary.entitlement.plan).toBe("free");
    expect(summary.billingInterval).toBeNull();
    expect(summary.paymentProvider).toBe("manual");
    expect(summary.hasStripeCustomer).toBe(false);
    expect(summary.availableActions).toEqual({
      checkout: true,
      portal: false,
      purchaseSeats: false,
    });
  });

  it("GET summary of a workspace with no seat limit has null seat capacity", () => {
    const result = WorkspaceSubscriptionSummarySchema.safeParse(
      fixture("summary_unlimited_seats.json"),
    );
    expect(result.success).toBe(true);
    if (!result.success) return;
    expect(result.data.seatCapacity).toBeNull();
    expect(result.data.entitlement.seats).toBe(0);
  });

  it("GET summary with a pending seat purchase fills pending_quantity and active_purchase", () => {
    const result = WorkspaceSubscriptionSummarySchema.safeParse(
      fixture("summary_paid_pending_purchase.json"),
    );
    expect(result.success).toBe(true);
    if (!result.success) return;
    const capacity = result.data.seatCapacity;
    expect(capacity?.pendingQuantity).toBe(8);
    expect(capacity?.activePurchase).toEqual({
      requestId: "66666666-6666-4666-8666-666666666666",
      targetSeats: 8,
      status: "pending",
      expiresAt: "2026-08-22T10:30:45Z",
    });
    expect(result.data.availableActions.purchaseSeats).toBe(true);
  });

  it("GET prices, of the current plan and of the plan named in the query", () => {
    for (const name of ["prices.json", "prices_plan_query.json"]) {
      const result = WorkspaceSubscriptionPricesSchema.safeParse(fixture(name));
      expect(result.success, name).toBe(true);
      if (!result.success) continue;
      expect(result.data.month.intervalCount).toBe(1);
      expect(result.data.year.currency).toBe("vnd");
    }
    const business = WorkspaceSubscriptionPricesSchema.parse(
      fixture("prices_plan_query.json"),
    );
    expect(business.month.unitAmount).toBe(349000);
  });

  it("POST checkout-sessions returns the address of the order page", () => {
    const body = fixture("checkout_session.json");
    const result = CreateWorkspaceSubscriptionCheckoutResponseSchema.safeParse(body);
    expect(result.success).toBe(true);
    if (!result.success) return;
    expect(result.data.url).toBe(
      "https://app.example.com/acme/billing/orders/66666666-6666-4666-8666-666666666666",
    );
    expect(result.data.requestId).toBe(result.data.sessionId);
  });

  it("POST seats/purchase-preview has a lower-case currency", () => {
    const body = fixture("seat_purchase_preview.json");
    expect(WorkspaceSeatPurchasePreviewSchema.safeParse(body).success).toBe(true);
    expect(body.currency).toBe("vnd");
    expect(
      WorkspaceSeatPurchasePreviewSchema.safeParse({ ...body, currency: "VND" })
        .success,
    ).toBe(false);
  });

  it("POST seats/purchases returns a pending purchase", () => {
    const result = PurchaseWorkspaceSeatsResponseSchema.safeParse(
      fixture("seat_purchase.json"),
    );
    expect(result.success).toBe(true);
    if (!result.success) return;
    expect(result.data.status).toBe("pending");
    expect(result.data.currency).toBe("vnd");
  });

  it("POST seats/reconcile is a noop", () => {
    const result = WorkspaceSubscriptionSeatReconcileResultSchema.safeParse(
      fixture("seat_reconcile.json"),
    );
    expect(result.success).toBe(true);
    if (!result.success) return;
    expect(result.data.action).toBe("noop");
  });
});

describe("Appendix A constraints the schemas still enforce on the platform's bodies", () => {
  it("refuses a seat capacity whose version is not positive", () => {
    const body = fixture("summary_free.json");
    body.seat_capacity.version = 0;
    expect(WorkspaceSubscriptionSummarySchema.safeParse(body).success).toBe(false);
  });

  it("refuses a pending_quantity that is not positive", () => {
    const body = fixture("summary_paid_pending_purchase.json");
    body.seat_capacity.pending_quantity = 0;
    expect(WorkspaceSubscriptionSummarySchema.safeParse(body).success).toBe(false);
  });

  it("leaves paymentProvider out when the server does not send it", () => {
    const body = fixture("summary_free.json");
    delete body.payment_provider;
    const result = WorkspaceSubscriptionSummarySchema.safeParse(body);
    expect(result.success).toBe(true);
    if (!result.success) return;
    expect("paymentProvider" in result.data).toBe(false);
  });

  it("refuses a payment_provider that is not a string", () => {
    const body = fixture("summary_free.json");
    body.payment_provider = 1;
    expect(WorkspaceSubscriptionSummarySchema.safeParse(body).success).toBe(false);
  });
});

describe("hosted URL scheme", () => {
  const session = (url: string) => ({
    request_id: "r",
    session_id: "s",
    url,
  });
  const accepts = (url: string) =>
    CreateWorkspaceSubscriptionCheckoutResponseSchema.safeParse(session(url))
      .success;

  it("accepts https for any host", () => {
    expect(accepts("https://checkout.stripe.com/c/pay")).toBe(true);
    expect(accepts("https://app.example.com/acme/billing/orders/1")).toBe(true);
  });

  it("accepts http only for localhost and 127.0.0.1", () => {
    expect(accepts("http://localhost:3000/acme/billing/orders/1")).toBe(true);
    expect(accepts("http://localhost/acme/billing/orders/1")).toBe(true);
    expect(accepts("http://127.0.0.1:3000/acme/billing/orders/1")).toBe(true);
  });

  it("refuses http to any other host, including look-alikes", () => {
    expect(accepts("http://app.example.com/orders/1")).toBe(false);
    expect(accepts("http://localhost.example.com/orders/1")).toBe(false);
    expect(accepts("http://127.0.0.1.example.com/orders/1")).toBe(false);
    expect(accepts("http://localhost@evil.example.com/orders/1")).toBe(false);
    expect(accepts("http://evil.example.com/?next=http://localhost/")).toBe(false);
    expect(accepts("http://[::1]:3000/orders/1")).toBe(false);
    expect(accepts("http://0.0.0.0:3000/orders/1")).toBe(false);
  });

  it("refuses other schemes", () => {
    expect(accepts("javascript:alert(1)")).toBe(false);
    expect(accepts("ftp://localhost/orders/1")).toBe(false);
    expect(accepts("//app.example.com/orders/1")).toBe(false);
    expect(accepts("/acme/billing/orders/1")).toBe(false);
    expect(accepts("")).toBe(false);
  });

  it("applies the same rule to the portal URL", () => {
    expect(
      CreateWorkspaceSubscriptionPortalResponseSchema.safeParse({
        url: "http://localhost:3000/portal",
      }).success,
    ).toBe(true);
    expect(
      CreateWorkspaceSubscriptionPortalResponseSchema.safeParse({
        url: "http://evil.example.com/portal",
      }).success,
    ).toBe(false);
  });
});
