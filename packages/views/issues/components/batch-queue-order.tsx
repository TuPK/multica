"use client";

import { useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@multica/ui/components/ui/popover";
import { useT } from "../../i18n";
import { parseQueueOrder, sanitizeQueueOrder } from "./queue-order-input";
import { useQueueOrderApply } from "./use-queue-order-apply";

/** Numbers every selected backlog issue at once; renders nothing without one. */
export function BatchQueueOrder({
  issueIds,
  disabled,
  onApplied,
}: {
  issueIds: string[];
  disabled?: boolean;
  onApplied?: () => void;
}) {
  const { t } = useT("issues");
  const { apply, dialog, isPending } = useQueueOrderApply();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");

  if (issueIds.length === 0) return dialog;

  const submit = (order: number | null) => {
    setOpen(false);
    void apply(issueIds, order, { onDone: onApplied });
  };
  const parsed = parseQueueOrder(text);

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger render={<Button variant="ghost" size="sm" disabled={disabled || isPending} />}>
          {t(($) => $.queue_order.label)}
        </PopoverTrigger>
        <PopoverContent align="center" className="flex w-56 flex-col gap-2 p-2">
          <Input
            value={text}
            inputMode="numeric"
            autoComplete="off"
            aria-label={t(($) => $.queue_order.label)}
            placeholder={t(($) => $.queue_order.placeholder)}
            onChange={(e) => setText(sanitizeQueueOrder(e.target.value))}
            onKeyDown={(e) => {
              if (e.key === "Enter" && typeof parsed === "number") submit(parsed);
            }}
          />
          <div className="flex justify-end gap-1.5">
            <Button variant="ghost" size="sm" onClick={() => submit(null)}>
              {t(($) => $.queue_order.clear)}
            </Button>
            <Button size="sm" disabled={typeof parsed !== "number"} onClick={() => submit(parsed as number)}>
              {t(($) => $.queue_order.apply)}
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      {dialog}
    </>
  );
}
