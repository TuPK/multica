// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  ClearWorkspaceScheduledPlanChangeResponseSchema,
  CreateWorkspacePlanChangeResponseSchema,
  WorkspaceBillingProfileSchema,
  WorkspacePlanChangePreviewSchema,
  WorkspaceSubscriptionCancelStateSchema,
  WorkspaceSubscriptionSummarySchema,
} from "./schemas";
import { parseWithFallback } from "./schema";

// The platform's contract fixtures (see billing-platform-contract.test.ts).
function fixture(name: string): Record<string, any> {
  const url = new URL(
    `../../../platform-management/contract/testdata/${name}`,
    import.meta.url,
  );
  return JSON.parse(readFileSync(fileURLToPath(url), "utf8"));
}

// Every route's response goes through parseWithFallback, so a malformed body
// reads as null ("unavailable") and never as a successful management action.
function parseOrNull(raw: unknown, schema: Parameters<typeof parseWithFallback>[1]) {
  return parseWithFallback(raw, schema, null, { endpoint: "test" });
}

describe("malformed subscription management responses", () => {
  it("returns null for a preview whose mode is not one of the three quote modes", () => {
    const body = { ...fixture("plan_change_preview.json"), mode: "discount" };
    expect(parseOrNull(body, WorkspacePlanChangePreviewSchema)).toBeNull();
  });

  it("returns null for a preview without its amount", () => {
    const body = fixture("plan_change_preview.json");
    delete body.amount;
    expect(parseOrNull(body, WorkspacePlanChangePreviewSchema)).toBeNull();
  });

  it("returns null for an order without a request id", () => {
    const body = fixture("plan_change_order.json");
    delete body.request_id;
    expect(parseOrNull(body, CreateWorkspacePlanChangeResponseSchema)).toBeNull();
  });

  it("returns null for a scheduled answer whose version is not a number", () => {
    const body = fixture("plan_change_scheduled.json");
    body.version = "5";
    expect(parseOrNull(body, CreateWorkspacePlanChangeResponseSchema)).toBeNull();
  });

  it("returns null when the cleared answer still carries a pending change", () => {
    const body = fixture("plan_change_scheduled_cleared.json");
    body.pending_change = { plan_key: "pro" };
    expect(parseOrNull(body, ClearWorkspaceScheduledPlanChangeResponseSchema)).toBeNull();
  });

  it("returns null for a cancel answer with a non-boolean flag", () => {
    const body = fixture("cancel.json");
    body.cancel_at_period_end = "true";
    expect(parseOrNull(body, WorkspaceSubscriptionCancelStateSchema)).toBeNull();
  });

  it("returns null for a summary whose pending_change has an unknown interval", () => {
    const body = fixture("summary_paid_pending_purchase.json");
    body.pending_change = {
      plan_key: "pro",
      seats: 5,
      interval: "week",
      requested_at: "2026-08-15T10:30:45Z",
    };
    expect(parseOrNull(body, WorkspaceSubscriptionSummarySchema)).toBeNull();
  });

  it("returns null for a summary whose change_plan flag is not a boolean", () => {
    const body = fixture("summary_free.json");
    body.available_actions.change_plan = "yes";
    expect(parseOrNull(body, WorkspaceSubscriptionSummarySchema)).toBeNull();
  });
});

describe("responses without the management fields (Multica Cloud, older backends)", () => {
  // Multica Cloud sends none of pending_change, the three new actions, or
  // payment_provider. The page must still read the summary, with those
  // actions off and no pending change.
  function legacySummary() {
    const body = fixture("summary_free.json");
    delete body.pending_change;
    delete body.payment_provider;
    delete body.available_actions.change_plan;
    delete body.available_actions.cancel;
    delete body.available_actions.resume;
    return body;
  }

  it("still parses the summary and reads the missing actions as off", () => {
    const summary = parseOrNull(legacySummary(), WorkspaceSubscriptionSummarySchema) as
      | Record<string, any>
      | null;
    expect(summary).not.toBeNull();
    expect(summary?.pendingChange).toBeNull();
    expect(summary?.availableActions).toEqual({
      checkout: true,
      portal: false,
      purchaseSeats: false,
      changePlan: false,
      cancel: false,
      resume: false,
    });
    expect("paymentProvider" in (summary ?? {})).toBe(false);
  });
});

describe("billing profile", () => {
  const individual = {
    account_kind: "individual",
    legal_name: "",
    tax_code: "",
    address: "",
    invoice_email: "",
    verified: false,
    changed: false,
  };
  const business = {
    account_kind: "business",
    legal_name: "CONG TY TNHH MULTICA",
    tax_code: "0123456789-001",
    address: "1 Le Loi, Quan 1",
    invoice_email: "ke.toan@example.com",
    verified: true,
    changed: true,
  };

  it("reads an individual profile with empty company fields", () => {
    const result = WorkspaceBillingProfileSchema.safeParse(individual);
    expect(result.success).toBe(true);
    if (!result.success) return;
    expect(result.data).toEqual({
      accountKind: "individual",
      legalName: "",
      taxCode: "",
      address: "",
      invoiceEmail: "",
      verified: false,
    });
  });

  it("reads a business profile with its company fields and verification", () => {
    const result = WorkspaceBillingProfileSchema.safeParse(business);
    expect(result.success).toBe(true);
    if (!result.success) return;
    expect(result.data).toEqual({
      accountKind: "business",
      legalName: "CONG TY TNHH MULTICA",
      taxCode: "0123456789-001",
      address: "1 Le Loi, Quan 1",
      invoiceEmail: "ke.toan@example.com",
      verified: true,
    });
  });

  it("returns null for an account kind that is neither individual nor business", () => {
    expect(
      parseOrNull({ ...individual, account_kind: "enterprise" }, WorkspaceBillingProfileSchema),
    ).toBeNull();
  });

  it("returns null when verified is missing, rather than reading the profile as unverified", () => {
    const body: Record<string, unknown> = { ...business };
    delete body.verified;
    expect(parseOrNull(body, WorkspaceBillingProfileSchema)).toBeNull();
  });
});
