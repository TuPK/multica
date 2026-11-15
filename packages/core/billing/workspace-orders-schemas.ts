import { z } from "zod";
import type {
  WorkspaceOrder,
  WorkspaceOrderBank,
  WorkspaceOrderBuyer,
  WorkspacePlan,
} from "./workspace-orders-types";

// Like the subscription schemas, these have no synthetic fallback: a body that
// does not match parses to null and the caller renders "unavailable". `.loose()`
// keeps unknown keys, so a newer server does not break an older client.

const nonNegativeInt = z.number().int().nonnegative();

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
    expires_at: z.string().min(1),
    paid_at: z.string().nullable().optional(),
    created_at: z.string().default(""),
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
      expiresAt: value.expires_at,
      paidAt: value.paid_at ?? null,
      createdAt: value.created_at,
    }),
  );

export const WorkspaceOrdersResponseSchema = z
  .object({ orders: z.array(WorkspaceOrderSchema) })
  .loose()
  .transform((value): WorkspaceOrder[] => value.orders);
