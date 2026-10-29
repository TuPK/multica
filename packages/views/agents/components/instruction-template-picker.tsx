"use client";

import { useState } from "react";
import { ChevronDown } from "@multica/ui/icons";
import {
  AGENT_INSTRUCTION_TEMPLATES,
  type AgentInstructionTemplate,
} from "@multica/core/agents";
import { Button } from "@multica/ui/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { useT } from "../../i18n";

/**
 * Starts the Instructions field from a built-in template. Replacing text the
 * user already wrote needs a confirmation, since the template overwrites it.
 */
export function InstructionTemplatePicker({
  currentInstructions,
  onSelect,
}: {
  currentInstructions: string;
  onSelect: (instructions: string) => void;
}) {
  const { t } = useT("agents");
  const [pending, setPending] = useState<AgentInstructionTemplate | null>(null);

  const apply = (template: AgentInstructionTemplate) => {
    if (currentInstructions.trim()) {
      setPending(template);
      return;
    }
    onSelect(template.instructions);
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="outline" size="sm">
              {t(($) => $.creation_studio.instruction_templates.trigger)}
              <ChevronDown aria-hidden="true" className="size-3.5" />
            </Button>
          }
        />
        <DropdownMenuContent align="end" className="w-72">
          {AGENT_INSTRUCTION_TEMPLATES.map((template) => (
            <DropdownMenuItem
              key={template.id}
              onClick={() => apply(template)}
              className="flex-col items-start gap-0.5"
            >
              <span className="text-body font-medium">
                {t(
                  ($) =>
                    $.creation_studio.instruction_templates.items[template.id]
                      .name,
                )}
              </span>
              <span className="text-caption text-muted-foreground">
                {t(
                  ($) =>
                    $.creation_studio.instruction_templates.items[template.id]
                      .description,
                )}
              </span>
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t(($) => $.creation_studio.instruction_templates.replace_title)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                ($) =>
                  $.creation_studio.instruction_templates.replace_description,
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>
              {t(($) => $.creation_studio.instruction_templates.replace_cancel)}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pending) onSelect(pending.instructions);
                setPending(null);
              }}
            >
              {t(
                ($) => $.creation_studio.instruction_templates.replace_confirm,
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
