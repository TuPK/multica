// Plans and orders of the platform's billing (platform-management spec 5.8),
// served by `GET /api/workspaces/{id}/plans` and `/orders`. Amounts are whole
// units of the currency; VND has no decimals.

import type { WorkspaceBillingModel } from "../types/billing";

/** Which workspaces may buy a plan. Unknown server values read as "any". */
export type WorkspacePlanAudience = "individual" | "business" | "any";

/** Scheduled price change disclosed to subscribers once notice is sent (spec 7b.4 rule 11). */
export interface ScheduledPriceChange {
  effectiveFrom: string;
  seatPriceMonth: number | null;
  seatPriceYear: number | null;
}

/** A public plan and its per-seat prices. A null price means the plan is not sold per seat. */
export interface WorkspacePlan {
  key: string;
  name: string;
  currency: string;
  seatPriceMonth: number | null;
  seatPriceYear: number | null;
  seatsFixed: number | null;
  minSeats: number | null;
  trialDays: number;
  audience: WorkspacePlanAudience;
  billingModel: WorkspaceBillingModel;
  /** Usage plans only: the price of `per` units of each meter, keyed by meter. */
  usageRates: Record<string, WorkspaceUsageRate>;
  /** The Business discount that applies to this workspace; 0 unless its profile is verified. */
  effectiveDiscountPct: number;
  /** Upcoming price change once notice has been sent to this workspace; null otherwise. */
  scheduledPriceChange: ScheduledPriceChange | null;
}

export interface WorkspaceUsageRate {
  per: number;
  price: number;
}

/** Who the invoice is made out to. Every field may be empty. */
export interface WorkspaceOrderBuyer {
  name: string;
  taxCode: string;
  address: string;
  invoiceEmail: string;
}

/** Where the customer sends the bank transfer. */
export interface WorkspaceOrderBank {
  bankName: string;
  accountName: string;
  accountNumber: string;
}

export interface WorkspaceOrder {
  id: string;
  workspaceId: string;
  /** `subscribe`, `change_plan`, `add_seats` or `renew`; an unknown value is kept as it came. */
  kind: string;
  planKey: string;
  /** `month` or `year`. */
  interval: string;
  /** For `add_seats`, the number of seats added. */
  seats: number;
  /** The price before VAT. */
  amount: number;
  vatAmount: number;
  /** What the customer transfers: the amount plus VAT. */
  totalAmount: number;
  currency: string;
  /** `pending`, `paid`, `expired`, `canceled` or `refunded`; an unknown value is kept as it came. */
  status: string;
  provider: string;
  /** To be written in the transfer note, so staff can match the payment to the order. */
  transferCode: string;
  bankDetails: WorkspaceOrderBank;
  buyer: WorkspaceOrderBuyer;
  periodStart: string | null;
  periodEnd: string | null;
  /** Null for a usage invoice, which stays payable until it is paid or written off. */
  expiresAt: string | null;
  paidAt: string | null;
  createdAt: string;
}

export interface UpdateWorkspaceOrderBuyerInput {
  orderId: string;
  buyer: WorkspaceOrderBuyer;
}

/** One day and meter of a usage invoice. Amounts are whole units; `netAmount` is after the discount. */
export interface WorkspaceOrderUsageLine {
  day: string;
  meter: string;
  quantity: number;
  unitPrice: number;
  per: number;
  discountPct: number;
  netAmount: number;
  note: string;
}

/** `GET /api/workspaces/{id}/orders/{orderId}/usage`. */
export interface WorkspaceOrderUsage {
  orderId: string;
  lines: WorkspaceOrderUsageLine[];
}
