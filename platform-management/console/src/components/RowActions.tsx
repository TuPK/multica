import type { ReactNode } from "react";
import { Button } from "@astryxdesign/core/Button";
import { DropdownMenu } from "@astryxdesign/core/DropdownMenu";
import { t } from "../i18n";
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
                key={t(action.label)}
                label={`${t(action.label)} ${label}`}
                icon={<Icon className="size-3.5" aria-hidden="true" />}
                isIconOnly
                tooltip={action.title ?? action.label}
                variant={action.destructive ? "destructive" : "ghost"}
                size="sm"
                isDisabled={action.disabled || action.busy}
                isLoading={action.busy}
                onClick={action.onSelect}
              />
            );
          }
          return (
            <Button
              key={t(action.label)}
              label={t(action.label)}
              variant={action.destructive ? "destructive" : action.primary ? "primary" : "secondary"}
              size="sm"
              isDisabled={action.disabled || action.busy}
              isLoading={action.busy}
              onClick={action.onSelect}
            />
          );
        })}
      </div>
    );
  }

  return (
    <div className="flex items-center justify-end">
      <DropdownMenu
        button={{
          label: t("Actions for {0}", label),
          icon: <MoreHorizontal className="size-4" aria-hidden="true" />,
          isIconOnly: true,
          variant: "ghost",
          size: "sm",
        }}
        alignment="end"
        menuWidth={176}
        items={actions.map((action) => {
          const Icon = action.icon;
          return {
            id: t(action.label),
            label: t(action.label),
            icon: Icon ? <Icon className="size-3.5" aria-hidden="true" /> : undefined,
            variant: action.destructive ? "destructive" : "default",
            isDisabled: action.disabled || action.busy,
            onClick: action.onSelect,
          };
        })}
      />
    </div>
  );
}

// Width for an actions column: wide enough for inline buttons when the sidebar
// is collapsed, a single menu trigger when it is expanded.
export function useRowActionsWidth(inlineWidth: number): number {
  return useSidebarCollapsed() ? inlineWidth : MENU_TRIGGER_WIDTH;
}

const MENU_TRIGGER_WIDTH = 72;
