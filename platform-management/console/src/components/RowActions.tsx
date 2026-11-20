import type { ReactNode } from "react";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { MoreHorizontal, type IconComponent } from "@multica/ui/icons";
import { useSidebarCollapsed } from "./sidebar-state";

export interface RowAction {
  label: string;
  onSelect: () => void;
  icon?: IconComponent;
  destructive?: boolean;
  primary?: boolean;
  disabled?: boolean;
  busy?: boolean;
  title?: string;
}

interface RowActionsProps {
  actions: RowAction[];
  label: string;
}

// Collapsed sidebar: each action is a visible button. Expanded sidebar: the
// table is narrow, so the actions collapse into a per-row "..." menu.
export function RowActions({ actions, label }: RowActionsProps): ReactNode {
  const collapsed = useSidebarCollapsed();

  if (actions.length === 0) return null;

  if (collapsed) {
    return (
      <div className="flex items-center justify-end gap-1">
        {actions.map((action) => {
          const Icon = action.icon;
          if (Icon) {
            return (
              <Button
                key={action.label}
                variant="ghost"
                size="icon-sm"
                title={action.title ?? action.label}
                aria-label={`${action.label} ${label}`}
                disabled={action.disabled || action.busy}
                aria-busy={action.busy || undefined}
                className={action.destructive ? "text-destructive hover:text-destructive" : undefined}
                onClick={action.onSelect}
              >
                <Icon className="size-3.5" aria-hidden="true" />
              </Button>
            );
          }
          return (
            <Button
              key={action.label}
              variant={action.destructive ? "destructive" : action.primary ? "default" : "outline"}
              size="sm"
              disabled={action.disabled || action.busy}
              aria-busy={action.busy || undefined}
              onClick={action.onSelect}
            >
              {action.label}
            </Button>
          );
        })}
      </div>
    );
  }

  return (
    <div className="flex items-center justify-end">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${label}`} />
          }
        >
          <MoreHorizontal className="size-4" aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-44">
          {actions.map((action) => {
            const Icon = action.icon;
            return (
              <DropdownMenuItem
                key={action.label}
                variant={action.destructive ? "destructive" : "default"}
                disabled={action.disabled || action.busy}
                title={action.title}
                onClick={action.onSelect}
              >
                {Icon ? <Icon className="size-3.5" aria-hidden="true" /> : null}
                {action.label}
              </DropdownMenuItem>
            );
          })}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

// Width for an actions column: wide enough for inline buttons when the sidebar
// is collapsed, a single menu trigger when it is expanded.
export function useRowActionsWidth(inlineWidth: number): number {
  return useSidebarCollapsed() ? inlineWidth : MENU_TRIGGER_WIDTH;
}

const MENU_TRIGGER_WIDTH = 56;
