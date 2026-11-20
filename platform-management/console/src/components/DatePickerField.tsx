import { useState } from "react";
import { Button } from "@astryxdesign/core/Button";
import { Calendar, type ISODateString } from "@astryxdesign/core/Calendar";
import { Popover } from "@astryxdesign/core/Popover";
import { CalendarDays, Check } from "@multica/ui/icons";
import { t } from "../i18n";

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
 * Shared calendar date field built on the Astryx Popover + Calendar.
 * Keeps trigger height (32px) aligned with TextInput and Selector on filter rows.
 */
export function DatePickerField({
  id,
  value,
  onChange,
  placeholder = t("Select date"),
  emptyLabel = t("Any date"),
  disabled = false,
  className,
}: DatePickerFieldProps) {
  const [open, setOpen] = useState(false);
  const selected = parseDateOnly(value);
  const selectedIso = selected ? (formatDateOnly(selected) as ISODateString) : null;

  return (
    <Popover
      isOpen={open}
      onOpenChange={setOpen}
      content={
        <div className="flex flex-col">
          <Button
            label={emptyLabel}
            variant="ghost"
            width="100%"
            endContent={selected ? undefined : <Check className="size-3.5" aria-hidden="true" />}
            onClick={() => {
              onChange("");
              setOpen(false);
            }}
          />
          <Calendar
            mode="single"
            value={selectedIso}
            focusDate={selectedIso ?? undefined}
            onChange={(day) => {
              onChange(day);
              setOpen(false);
            }}
          />
        </div>
      }
    >
      {(triggerProps) => (
        <Button
          {...triggerProps}
          id={id}
          label={selected ? value : placeholder}
          variant="secondary"
          size="md"
          width="100%"
          className={className}
          isDisabled={disabled}
          endContent={<CalendarDays className="size-4 shrink-0" aria-hidden="true" />}
        />
      )}
    </Popover>
  );
}
