import { z } from "zod";
import {
  ScheduledPriceChangeSchema,
  WorkspaceOrderEinvoiceSchema,
} from "../api/schemas";
import type {
  WorkspaceOrder,
  WorkspaceOrderBank,
  WorkspaceOrderBuyer,
  WorkspaceOrderUsage,
  WorkspaceOrderUsageLine,
  WorkspacePlan,
  WorkspacePlanAudience,
  WorkspaceUsageRate,
} from "./workspace-orders-types";

// Like the subscription schemas, these have no synthetic fallback: a body that
// does not match parses to null and the caller renders "unavailable". `.loose()`
// keeps unknown keys, so a newer server does not break an older client.

const nonNegativeInt = z.number().int().nonnegative();

const PLAN_AUDIENCES: readonly WorkspacePlanAudience[] = [
  "individual",
  "business",
  "any",
];

// A missing or unknown audience is treated as "any": the server still enforces
// audience on purchase (409 plan_audience_mismatch), so this only shapes the picker.
function parsePlanAudience(value: string | null): WorkspacePlanAudience {
  return PLAN_AUDIENCES.find((audience) => audience === value) ?? "any";
}

export const WorkspacePlanSchema = z
  .object({
    key: z.string().min(1),
    name: z.string(),
    currency: z.string().min(1),
    seat_price_month: nonNegativeInt.nullable().default(null),
    seat_price_year: nonNegativeInt.nullable().default(null),
    seats_fixed: nonNegativeInt.nullable().default(null),
    min_seats: nonNegativeInt.nullable().default(null),
    trial_days: nonNegativeInt.default(0),
    audience: z.string().nullable().default(null),
    billing_model: z.string().default("seat"),
    usage_rates: z
      .record(
        z.string(),
        z.object({ per: nonNegativeInt, price: nonNegativeInt }).loose(),
      )
      .default({}),
    effective_discount_pct: z.number().int().min(0).max(100).default(0),
    scheduled_price_change: ScheduledPriceChangeSchema,
  })
  .loose()
  .transform(
    (value): WorkspacePlan => ({
      key: value.key,
      name: value.name,
      currency: value.currency,
      seatPriceMonth: value.seat_price_month,
      seatPriceYear: value.seat_price_year,
      seatsFixed: value.seats_fixed,
      minSeats: value.min_seats,
      trialDays: value.trial_days,
      audience: parsePlanAudience(value.audience),
      // An unknown model reads as seat, which has no usage rates to show.
      billingModel: value.billing_model === "usage" ? "usage" : "seat",
      usageRates: Object.fromEntries(
        Object.entries(value.usage_rates).map(([meter, rate]): [string, WorkspaceUsageRate] => [
          meter,
          { per: rate.per, price: rate.price },
        ]),
      ),
      effectiveDiscountPct: value.effective_discount_pct,
      scheduledPriceChange: value.scheduled_price_change,
    }),
  );

export const WorkspacePlansResponseSchema = z
  .object({ plans: z.array(WorkspacePlanSchema) })
  .loose()
  .transform((value): WorkspacePlan[] => value.plans);

const BuyerSchema = z
  .object({
    name: z.string().default(""),
    tax_code: z.string().default(""),
    address: z.string().default(""),
    invoice_email: z.string().default(""),
  })
  .loose()
  .transform(
    (value): WorkspaceOrderBuyer => ({
      name: value.name,
      taxCode: value.tax_code,
      address: value.address,
      invoiceEmail: value.invoice_email,
    }),
  );

const BankSchema = z
  .object({
    bank_name: z.string().default(""),
    account_name: z.string().default(""),
    account_number: z.string().default(""),
  })
  .loose()
  .transform(
    (value): WorkspaceOrderBank => ({
      bankName: value.bank_name,
      accountName: value.account_name,
      accountNumber: value.account_number,
    }),
  );

const EMPTY_BUYER = { name: "", tax_code: "", address: "", invoice_email: "" };
const EMPTY_BANK = { bank_name: "", account_name: "", account_number: "" };

export const WorkspaceOrderSchema = z
  .object({
    id: z.string().min(1),
    workspace_id: z.string().min(1),
    kind: z.string(),
    plan_key: z.string(),
    interval: z.string(),
    seats: nonNegativeInt,
    amount: nonNegativeInt,
    vat_amount: nonNegativeInt.default(0),
    total_amount: nonNegativeInt.optional(),
    currency: z.string().min(1),
    // Open strings: a newer server may add a kind or status, and it must read
    // as unknown rather than be taken for a known one.
    status: z.string(),
    provider: z.string().default(""),
    transfer_code: z.string(),
    bank_details: BankSchema.prefault(EMPTY_BANK),
    buyer: BuyerSchema.prefault(EMPTY_BUYER),
    period_start: z.string().nullable().optional(),
    period_end: z.string().nullable().optional(),
    // Null for a usage invoice: it does not expire (spec 7b.3 rule 11).
    expires_at: z.string().nullable().optional(),
    paid_at: z.string().nullable().optional(),
    created_at: z.string().default(""),
    einvoice: WorkspaceOrderEinvoiceSchema,
  })
  .loose()
  .transform(
    (value): WorkspaceOrder => ({
      id: value.id,
      workspaceId: value.workspace_id,
      kind: value.kind,
      planKey: value.plan_key,
      interval: value.interval,
      seats: value.seats,
      amount: value.amount,
      vatAmount: value.vat_amount,
      totalAmount: value.total_amount ?? value.amount + value.vat_amount,
      currency: value.currency,
      status: value.status,
      provider: value.provider,
      transferCode: value.transfer_code,
      bankDetails: value.bank_details,
      buyer: value.buyer,
      periodStart: value.period_start ?? null,
      periodEnd: value.period_end ?? null,
      expiresAt: value.expires_at ?? null,
      paidAt: value.paid_at ?? null,
      createdAt: value.created_at,
      einvoice: value.einvoice,
    }),
  );

export const WorkspaceOrdersResponseSchema = z
  .object({ orders: z.array(WorkspaceOrderSchema) })
  .loose()
  .transform((value): WorkspaceOrder[] => value.orders);

const OrderUsageLineSchema = z
  .object({
    day: z.string().min(1),
    meter: z.string().min(1),
    quantity: nonNegativeInt,
    unit_price: nonNegativeInt,
    per: z.number().int().positive(),
    discount_pct: z.number().int().min(0).max(100),
    net_amount: nonNegativeInt,
    note: z.string().default(""),
  })
  .loose()
  .transform(
    (value): WorkspaceOrderUsageLine => ({
      day: value.day,
      meter: value.meter,
      quantity: value.quantity,
      unitPrice: value.unit_price,
      per: value.per,
      discountPct: value.discount_pct,
      netAmount: value.net_amount,
      note: value.note,
    }),
  );

export const WorkspaceOrderUsageSchema = z
  .object({
    order_id: z.string().min(1),
    lines: z.array(OrderUsageLineSchema),
  })
  .loose()
  .transform(
    (value): WorkspaceOrderUsage => ({
      orderId: value.order_id,
      lines: value.lines,
    }),
  );
