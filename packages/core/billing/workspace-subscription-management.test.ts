// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "../api/client";
import {
  workspaceBillingProfileOptions,
  workspaceSubscriptionKeys,
} from "./workspace-subscription-queries";

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

function lastCall(): [string, RequestInit] {
  const call = vi.mocked(fetch).mock.calls.at(-1);
  return [String(call?.[0]), call?.[1] as RequestInit];
}

function headerOf(init: RequestInit, name: string): string | null {
  return new Headers(init.headers).get(name);
}

afterEach(() => {
  vi.unstubAllGlobals();
});

const client = () => new ApiClient("https://api.example.test");

const profile = {
  account_kind: "business",
  legal_name: "ACME",
  tax_code: "0123456789",
  address: "1 Le Loi",
  invoice_email: "ke.toan@example.com",
  verified: false,
  changed: true,
};

const cancelState = {
  cancel_at_period_end: true,
  status: "active",
  current_period_end: "2030-05-31T00:00:00Z",
  version: 5,
  changed: true,
};

describe("billing profile routes", () => {
  it("reads the profile from GET billing-profile", async () => {
    stubFetchJson(profile);
    const saved = await client().getWorkspaceBillingProfile();
    expect(saved).toEqual({
      accountKind: "business",
      legalName: "ACME",
      taxCode: "0123456789",
      address: "1 Le Loi",
      invoiceEmail: "ke.toan@example.com",
      verified: false,
    });
    const [url, init] = lastCall();
    expect(url).toBe("https://api.example.test/api/cloud-subscriptions/billing-profile");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("switches to an individual profile: PUT sends the account kind in snake case", async () => {
    stubFetchJson({ ...profile, account_kind: "individual" });
    const saved = await client().updateWorkspaceBillingProfile({
      accountKind: "individual",
      legalName: null,
      taxCode: null,
      address: null,
      invoiceEmail: null,
    });
    expect(saved?.accountKind).toBe("individual");
    const [url, init] = lastCall();
    expect(url).toBe("https://api.example.test/api/cloud-subscriptions/billing-profile");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({
      account_kind: "individual",
      legal_name: null,
      tax_code: null,
      address: null,
      invoice_email: null,
    });
  });

  it("returns null for a malformed profile answer", async () => {
    stubFetchJson({ account_kind: "enterprise" });
    await expect(client().getWorkspaceBillingProfile()).resolves.toBeNull();
    stubFetchJson({ ...profile, account_kind: "enterprise" });
    await expect(
      client().updateWorkspaceBillingProfile({ accountKind: "business" }),
    ).resolves.toBeNull();
  });

  it("throws on 403 for a member who may not manage billing", async () => {
    stubFetchJson({ error: "forbidden" }, 403);
    await expect(client().getWorkspaceBillingProfile()).rejects.toBeInstanceOf(ApiError);
  });
});

describe("plan change routes", () => {
  it("previews a change with plan_key, interval and seats in snake case", async () => {
    stubFetchJson({
      mode: "charge",
      plan_key: "unlimited",
      interval: "month",
      current_seats: 5,
      resulting_seats: 5,
      currency: "vnd",
      amount: 333333,
      vat_amount: 33333,
      effective_at: "2030-05-11T00:00:00Z",
      expected_version: 4,
      quoted_at: "2030-05-11T00:00:00Z",
    });
    const preview = await client().previewWorkspacePlanChange({
      planKey: "unlimited",
      interval: "month",
      seats: 5,
    });
    expect(preview).toMatchObject({ mode: "charge", amount: 333333, expectedVersion: 4 });
    const [url, init] = lastCall();
    expect(url).toBe(
      "https://api.example.test/api/cloud-subscriptions/plan-changes/preview",
    );
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({
      plan_key: "unlimited",
      interval: "month",
      seats: 5,
    });
  });

  it("returns null for a preview with an unknown mode", async () => {
    stubFetchJson({
      mode: "discount",
      plan_key: "pro",
      interval: "month",
      current_seats: 5,
      resulting_seats: 5,
      currency: "vnd",
      amount: 1,
      vat_amount: 0,
      effective_at: "2030-05-11T00:00:00Z",
      expected_version: 4,
      quoted_at: "2030-05-11T00:00:00Z",
    });
    await expect(
      client().previewWorkspacePlanChange({ planKey: "pro", interval: "month", seats: 5 }),
    ).resolves.toBeNull();
  });

  it("applies a quoted change: the accepted amount, the version and both idempotency keys", async () => {
    stubFetchJson({
      request_id: "eeeeeeee-0000-0000-0000-000000000001",
      kind: "change_plan",
      status: "pending",
      plan_key: "unlimited",
      interval: "month",
      seats: 5,
      currency: "vnd",
      amount: 333333,
      vat_amount: 33333,
      expires_at: "2030-05-18T00:00:00Z",
    });
    const result = await client().createWorkspacePlanChange({
      planKey: "unlimited",
      interval: "month",
      seats: 5,
      acceptedAmount: 333333,
      expectedVersion: 4,
      idempotencyKey: "k1",
    });
    expect(result).toMatchObject({ outcome: "order", status: "pending" });
    const [url, init] = lastCall();
    expect(url).toBe("https://api.example.test/api/cloud-subscriptions/plan-changes");
    expect(init.method).toBe("POST");
    expect(headerOf(init, "Idempotency-Key")).toBe("k1");
    expect(JSON.parse(String(init.body))).toEqual({
      plan_key: "unlimited",
      interval: "month",
      seats: 5,
      accepted_amount: 333333,
      expected_version: 4,
      idempotency_key: "k1",
    });
  });

  it("returns null for a malformed application answer", async () => {
    stubFetchJson({ ok: true });
    await expect(
      client().createWorkspacePlanChange({
        planKey: "pro",
        interval: "year",
        seats: 5,
        idempotencyKey: "k1",
      }),
    ).resolves.toBeNull();
  });

  it("throws on 409 order_pending", async () => {
    stubFetchJson(
      { error: "an order is pending", code: "order_pending", order_id: "o-1" },
      409,
    );
    await expect(
      client().createWorkspacePlanChange({
        planKey: "pro",
        interval: "year",
        seats: 5,
        idempotencyKey: "k2",
      }),
    ).rejects.toMatchObject({ status: 409 });
  });

  it("clears a scheduled change with DELETE and the idempotency header", async () => {
    stubFetchJson({ pending_change: null, version: 5, changed: true });
    const cleared = await client().cancelWorkspaceScheduledPlanChange("k3");
    expect(cleared).toEqual({ pendingChange: null, version: 5, changed: true });
    const [url, init] = lastCall();
    expect(url).toBe(
      "https://api.example.test/api/cloud-subscriptions/plan-changes/scheduled",
    );
    expect(init.method).toBe("DELETE");
    expect(headerOf(init, "Idempotency-Key")).toBe("k3");
  });
});

describe("cancel and resume routes", () => {
  it("cancels at the period end with POST cancel and the idempotency header", async () => {
    stubFetchJson(cancelState);
    const state = await client().cancelWorkspaceSubscription("k4");
    expect(state).toEqual({
      cancelAtPeriodEnd: true,
      status: "active",
      currentPeriodEnd: "2030-05-31T00:00:00Z",
      version: 5,
      changed: true,
    });
    const [url, init] = lastCall();
    expect(url).toBe("https://api.example.test/api/cloud-subscriptions/cancel");
    expect(init.method).toBe("POST");
    expect(headerOf(init, "Idempotency-Key")).toBe("k4");
  });

  it("resumes with POST resume and the idempotency header", async () => {
    stubFetchJson({ ...cancelState, cancel_at_period_end: false, version: 6 });
    const state = await client().resumeWorkspaceSubscription("k5");
    expect(state?.cancelAtPeriodEnd).toBe(false);
    const [url, init] = lastCall();
    expect(url).toBe("https://api.example.test/api/cloud-subscriptions/resume");
    expect(init.method).toBe("POST");
    expect(headerOf(init, "Idempotency-Key")).toBe("k5");
  });

  it("returns null for a malformed cancel answer", async () => {
    stubFetchJson({ cancel_at_period_end: "yes" });
    await expect(client().cancelWorkspaceSubscription("k6")).resolves.toBeNull();
  });
});

describe("query keys of the management reads", () => {
  it("the billing profile key includes the workspace id", () => {
    expect(workspaceSubscriptionKeys.billingProfile("ws-1")).toContain("ws-1");
    expect(workspaceSubscriptionKeys.billingProfile("ws-1")).not.toEqual(
      workspaceSubscriptionKeys.billingProfile("ws-2"),
    );
    expect(workspaceBillingProfileOptions("ws-1").queryKey).toContain("ws-1");
  });

  it("the billing profile sits under the subscription prefix, so one invalidation reaches it", () => {
    const prefix = workspaceSubscriptionKeys.all("ws-1");
    expect(
      workspaceSubscriptionKeys.billingProfile("ws-1").slice(0, prefix.length),
    ).toEqual(prefix);
  });

  it("does not query the billing profile without a workspace", () => {
    expect(workspaceBillingProfileOptions("").enabled).toBe(false);
    expect(workspaceBillingProfileOptions("ws-1").enabled).toBe(true);
  });
});
