"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  COMPANY_SLOTS,
  useCreateCompanyFloor,
  useDeleteCompanyFloor,
  useUpdateCompanyFloor,
  type CompanyFloor,
} from "@multica/core/company";
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
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { useT } from "../../i18n";

/** Creates a department (floor) when `floor` is null, otherwise renames `floor`. */
export function FloorNameDialog({
  floor,
  onClose,
  onSaved,
}: {
  floor: CompanyFloor | null;
  onClose: () => void;
  onSaved: (floor: CompanyFloor) => void;
}) {
  const { t } = useT("company");
  const { t: tc } = useT("common");
  const create = useCreateCompanyFloor();
  const update = useUpdateCompanyFloor();
  const [name, setName] = useState(floor?.name ?? "");
  const pending = create.isPending || update.isPending;
  const trimmed = name.trim();

  const submit = () => {
    if (!trimmed || pending) return;
    const done = (saved: CompanyFloor) => {
      onSaved(saved);
      onClose();
    };
    const failed = () => toast.error(t(($) => $.department_save_failed));
    if (floor) {
      update.mutate({ id: floor.id, name: trimmed }, { onSuccess: done, onError: failed });
    } else {
      create.mutate({ name: trimmed }, { onSuccess: done, onError: failed });
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>
            {floor ? t(($) => $.department_rename_title) : t(($) => $.department_create_title)}
          </DialogTitle>
        </DialogHeader>
        <form
          className="flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Label htmlFor="company-floor-name">{t(($) => $.department_name)}</Label>
          <Input
            id="company-floor-name"
            value={name}
            maxLength={60}
            autoFocus
            onChange={(event) => setName(event.target.value)}
          />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {tc(($) => $.cancel)}
            </Button>
            <Button type="submit" disabled={!trimmed || pending}>
              {tc(($) => $.save)}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Confirms a floor delete from either the org chart or the company tabs. The
 * floor's rooms are removed and its squads lose their rooms but are kept. The
 * dialog closes only after the server confirms the delete.
 */
export function FloorDeleteDialog({
  floor,
  seatedCount,
  onClose,
  onDeleted,
}: {
  floor: CompanyFloor;
  /** Live squads seated on the floor, from the squad list the caller has loaded. */
  seatedCount: number;
  onClose: () => void;
  onDeleted: (floor: CompanyFloor) => void;
}) {
  const { t } = useT("issues");
  const { t: tc } = useT("common");
  const remove = useDeleteCompanyFloor();

  const confirm = () =>
    remove.mutate(floor.id, {
      onSuccess: () => onDeleted(floor),
      onError: () => toast.error(t(($) => $.company.floor_delete_failed)),
    });

  return (
    <AlertDialog open onOpenChange={(open) => !open && !remove.isPending && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t(($) => $.company.floor_delete_title, { name: floor.name })}</AlertDialogTitle>
          <AlertDialogDescription>
            {t(($) => $.company.floor_delete_rooms, { rooms: COMPANY_SLOTS })}
            {seatedCount > 0 ? ` ${t(($) => $.company.floor_delete_squads, { count: seatedCount })}` : null}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={remove.isPending}>{tc(($) => $.cancel)}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled={remove.isPending} onClick={confirm}>
            {t(($) => $.company.floor_delete)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
