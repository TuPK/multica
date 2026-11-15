// Plans and orders of the platform's billing (platform-management spec 5.8),
// served by `GET /api/workspaces/{id}/plans` and `/orders`. Amounts are whole
// units of the currency; VND has no decimals.

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
  expiresAt: string;
  paidAt: string | null;
  createdAt: string;
}

export interface UpdateWorkspaceOrderBuyerInput {
  orderId: string;
  buyer: WorkspaceOrderBuyer;
}
