import { useState } from "react";
import { Calendar } from "@multica/ui/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { CalendarDays, Check } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";

/** Formats a local Date as YYYY-MM-DD without UTC timezone shift. */
function formatDateOnly(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

/** Parses a YYYY-MM-DD string into a local Date, or undefined when blank/invalid. */
function parseDateOnly(value: string): Date | undefined {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
  if (!match) return undefined;
  const year = Number(match[1]);
  const month = Number(match[2]) - 1;
  const day = Number(match[3]);
  const parsed = new Date(year, month, day);
  if (
    parsed.getFullYear() !== year ||
    parsed.getMonth() !== month ||
    parsed.getDate() !== day
  ) {
    return undefined;
  }
  return parsed;
}

interface DatePickerFieldProps {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  emptyLabel?: string;
  disabled?: boolean;
  className?: string;
}

/**
 * Shared calendar date field built on @multica/ui Popover + Calendar.
 * Keeps trigger height (32px) aligned with Input and SelectTrigger on filter rows.
 */
export function DatePickerField({
  id,
  value,
  onChange,
  placeholder = "Select date",
  emptyLabel = "Any date",
  disabled = false,
  className,
}: DatePickerFieldProps) {
  const [open, setOpen] = useState(false);
  const selected = parseDateOnly(value);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        id={id}
        type="button"
        disabled={disabled}
        className={cn(
          "flex h-8 w-full min-w-0 items-center justify-between gap-2 rounded-lg border border-input bg-transparent px-2.5 py-1 text-left text-body transition-colors outline-none hover:bg-surface-hover focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-input/50 disabled:opacity-50 dark:bg-input/30 dark:hover:bg-input/50",
          className,
        )}
      >
        <span
          className={cn(
            "truncate tabular-nums",
            selected ? "text-foreground" : "text-muted-foreground",
          )}
        >
          {selected ? value : placeholder}
        </span>
        <CalendarDays
          className="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto p-0">
        <button
          type="button"
          onClick={() => {
            onChange("");
            setOpen(false);
          }}
          className="flex w-full items-center gap-3 border-b border-surface-border px-3 py-2 text-left text-body transition-colors hover:bg-surface-hover"
        >
          <span className="flex min-w-0 flex-1 items-center gap-2 text-muted-foreground">
            {emptyLabel}
          </span>
          <Check
            className={cn(
              "size-3.5 shrink-0 text-muted-foreground",
              selected && "invisible",
            )}
            aria-hidden="true"
          />
        </button>
        <Calendar
          mode="single"
          selected={selected}
          defaultMonth={selected}
          onSelect={(day) => {
            onChange(day ? formatDateOnly(day) : "");
            setOpen(false);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}
