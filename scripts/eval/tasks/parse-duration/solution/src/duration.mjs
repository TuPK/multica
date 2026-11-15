const SECONDS = { d: 86400, h: 3600, m: 60, s: 1 };

export function parseDuration(text) {
  if (!/^(\d+[dhms])+$/.test(text)) throw new RangeError(`not a duration: ${JSON.stringify(text)}`);
  let total = 0;
  for (const [, n, unit] of text.matchAll(/(\d+)([dhms])/g)) total += Number(n) * SECONDS[unit];
  return total;
}
