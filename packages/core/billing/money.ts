// Minor-unit money formatting for platform billing (platform-management spec 7.4).
// An amount is an integer in minor units of one order currency; nothing here
// converts between currencies.

/** Fraction digits of each enabled currency, keyed by lower-case ISO 4217 code. */
export const CURRENCY_EXPONENTS: Readonly<Record<string, number>> = {
  vnd: 0,
  usd: 2,
  cad: 2,
};

/** ISO 4217 default for a code that is not in the table. */
const DEFAULT_EXPONENT = 2;

export function currencyExponent(code: string): number {
  return CURRENCY_EXPONENTS[code.trim().toLowerCase()] ?? DEFAULT_EXPONENT;
}

/**
 * Format `minor` units of `code`. A known code uses Intl currency style with the
 * table's fraction digits. An unknown code prints the amount with 2 digits
 * followed by the upper-case code. `locale` defaults to the runtime locale.
 */
export function formatMinorAmount(
  minor: number,
  code: string,
  locale?: string,
): string {
  const normalized = code.trim().toLowerCase();
  const digits = currencyExponent(normalized);
  const major = minor / 10 ** digits;
  if (normalized in CURRENCY_EXPONENTS) {
    try {
      return new Intl.NumberFormat(locale, {
        style: "currency",
        currency: normalized.toUpperCase(),
        minimumFractionDigits: digits,
        maximumFractionDigits: digits,
      }).format(major);
    } catch {
      // Fall through to the plain form.
    }
  }
  const plain = new Intl.NumberFormat(locale, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(major);
  return normalized ? `${plain} ${normalized.toUpperCase()}` : plain;
}
