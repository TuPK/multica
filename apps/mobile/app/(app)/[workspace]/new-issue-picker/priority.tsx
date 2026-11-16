/**
 * Priority picker route for the in-progress new-issue draft. See ./status.tsx.
 */
import { router } from "expo-router";
import { PriorityPickerBody } from "@/components/issue/pickers/priority-picker-body";
import { useNewIssueDraftStore } from "@/data/stores/new-issue-draft-store";
import { WriteAccessGuard } from "@/components/workspace/write-access-notice";

export default function NewIssuePriorityPickerRoute() {
  return (
    <WriteAccessGuard>
      <NewIssuePriorityPickerContent />
    </WriteAccessGuard>
  );
}

function NewIssuePriorityPickerContent() {
  const priority = useNewIssueDraftStore((s) => s.priority);
  const setPriority = useNewIssueDraftStore((s) => s.setPriority);

  return (
    <PriorityPickerBody
      value={priority}
      onChange={(next) => {
        setPriority(next);
        router.back();
      }}
    />
  );
}
