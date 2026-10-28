"use client";

import { useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { useT } from "../../i18n";

export interface LocalPathMachine {
  daemonId: string;
  label: string;
}

// POSIX absolute, Windows drive-absolute, or UNC share. The daemon re-checks the
// path on its own machine; this only catches the obvious mistakes before a save.
const ABSOLUTE_PATH = /^(\/|[A-Za-z]:[\\/]|\\\\)/;

export function isAbsoluteLocalPath(value: string): boolean {
  return ABSOLUTE_PATH.test(value);
}

/**
 * Web-side entry for a local_directory: the browser cannot see the filesystem of
 * the machine that will run the agent, so the user names the machine (one of the
 * workspace's online daemons) and types the absolute path on it.
 */
export function LocalPathForm({
  machines,
  onSubmit,
}: {
  machines: LocalPathMachine[];
  onSubmit: (path: string, daemonId: string) => void;
}) {
  const { t } = useT("projects");
  const [path, setPath] = useState("");
  const [picked, setPicked] = useState<string | null>(null);

  // Fall back to the first machine when the chosen one dropped offline or the
  // list has not loaded yet.
  const selected =
    machines.find((m) => m.daemonId === picked)?.daemonId ??
    machines[0]?.daemonId ??
    "";
  const trimmed = path.trim();
  const showInvalid = trimmed !== "" && !isAbsoluteLocalPath(trimmed);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!trimmed || !selected || !isAbsoluteLocalPath(trimmed)) return;
    onSubmit(trimmed, selected);
    setPath("");
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-1.5 pt-1 border-t">
      <div className="px-2 pt-1 text-micro font-medium text-muted-foreground">
        {t(($) => $.resources.local_path_heading)}
      </div>
      {machines.length === 0 ? (
        <p className="px-2 text-micro text-muted-foreground">
          {t(($) => $.resources.local_path_no_machines)}
        </p>
      ) : (
        <>
          <Select
            items={machines.map((m) => ({ value: m.daemonId, label: m.label }))}
            value={selected}
            onValueChange={(next) => next && setPicked(next)}
          >
            <SelectTrigger
              size="sm"
              aria-label={t(($) => $.resources.local_path_machine_label)}
              className="w-full"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {machines.map((m) => (
                <SelectItem key={m.daemonId} value={m.daemonId}>
                  {m.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex items-center gap-1.5">
            <input
              type="text"
              value={path}
              onChange={(e) => setPath(e.target.value)}
              aria-label={t(($) => $.resources.local_path_heading)}
              placeholder={t(($) => $.resources.local_path_placeholder)}
              className="flex-1 min-w-0 bg-transparent text-caption px-2 py-1 outline-none placeholder:text-muted-foreground"
            />
            <Button
              type="submit"
              size="sm"
              variant="ghost"
              className="h-6 px-2 text-caption"
              disabled={!trimmed || !selected}
            >
              {t(($) => $.resources.url_submit)}
            </Button>
          </div>
          {showInvalid && (
            <p className="px-2 text-micro text-destructive">
              {t(($) => $.resources.local_validate_not_absolute)}
            </p>
          )}
        </>
      )}
    </form>
  );
}
