/**
 * Request header carrying the elapsed seconds since the user opened a decision
 * question (durable spec 7.2). Omitted when no valid open timestamp is given.
 */
export const ATTENTION_OPENED_HEADER = "X-Attention-Opened-At";

/**
 * Builds the attention-timing header map from the epoch millisecond timestamp
 * when the question was opened. Returns `{}` if `openedAtMs` is missing or not
 * finite so no empty or synthetic header is sent.
 */
export function attentionOpenedHeaders(
  openedAtMs: number | undefined,
  nowMs: number = Date.now(),
): Record<string, string> {
  if (openedAtMs === undefined || !Number.isFinite(openedAtMs)) {
    return {};
  }
  const seconds = Math.max(0, Math.floor((nowMs - openedAtMs) / 1000));
  return { [ATTENTION_OPENED_HEADER]: String(seconds) };
}
