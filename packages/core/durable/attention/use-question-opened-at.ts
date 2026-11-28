import { useState } from "react";

/**
 * Captures the epoch millisecond timestamp when a question card first mounts,
 * remaining constant across re-renders until unmount.
 */
export function useQuestionOpenedAt(): number {
  return useState(() => Date.now())[0];
}
