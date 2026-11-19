import { formatMinorAmount } from "@multica/core/billing/money";

/** Format a minor-unit amount of one order currency; digits come from the shared table. */
export function formatStripeMinorAmount(
  amount: number,
  currency: string,
  locale: string,
): string | null {
  if (!Number.isSafeInteger(amount) || amount < 0) return null;
  if (!currency.trim()) return null;
  return formatMinorAmount(amount, currency, locale);
}
