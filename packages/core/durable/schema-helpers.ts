import { z } from "zod";

// Lenient building blocks for the durable schemas (see parseWithFallback). A missing field
// takes its default, an unknown enum value degrades to a safe one, and a bad item in a list
// is dropped alone instead of failing the whole page.

export const str = z.string().catch("").default("");
export const nullableStr = z.string().nullable().catch(null).default(null);
export const bool = z.boolean().catch(false).default(false);
export const count = z.coerce.number().catch(0).default(0);

/** A string enum that reads any value outside `values` as `fallback`. */
export function enumOr<const T extends readonly [string, ...string[]]>(values: T, fallback: T[number]) {
  return z.enum(values).catch(fallback).default(fallback);
}

/** A list read item by item: a missing or non-list value is empty, a bad item is dropped. */
export function listOfItems<T>(schema: z.ZodType<T>) {
  return z.unknown().transform((value) =>
    Array.isArray(value)
      ? value.flatMap((item) => {
          const parsed = schema.safeParse(item);
          return parsed.success ? [parsed.data] : [];
        })
      : [],
  );
}
