// Minor-unit money formatting for the console (platform-management spec 7.4).
// The console is a separate app, so this mirrors packages/core/billing/money.ts.

/** Fraction digits of each enabled currency, keyed by lower-case ISO 4217 code. */
export const CURRENCY_EXPONENTS: Readonly<Record<string, number>> = {
  vnd: 0,
  usd: 2,
  cad: 2,
};

const DEFAULT_EXPONENT = 2;

export function currencyExponent(code: string): number {
  return CURRENCY_EXPONENTS[code.trim().toLowerCase()] ?? DEFAULT_EXPONENT;
}

/** Amount then upper-case code, e.g. "149,000 VND". Prints the amount alone when the code is empty. */
export function formatMinorAmount(minor: number, code: string): string {
  const normalized = code.trim().toUpperCase();
  // With no code there is no exponent to apply; print the integer as is.
  const digits = normalized ? currencyExponent(normalized) : 0;
  const amount = (minor / 10 ** digits).toLocaleString(undefined, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
  return normalized ? `${amount} ${normalized}` : amount;
}
