"use client";

import { ReadonlyContent } from "../../editor/readonly-content";

const HAS_LINK = /\]\([^)\s]+\)/;

/**
 * A wakeup instruction as saved. Plain text stays plain; one carrying an
 * attachment link (an image pasted into the box) renders as Markdown so the
 * image shows instead of its source.
 */
export function WakeupInstructionText({ text, className }: { text: string; className: string }) {
  if (HAS_LINK.test(text)) return <ReadonlyContent content={text} className={className} />;
  return <p className={`whitespace-pre-wrap break-words ${className}`}>{text}</p>;
}
