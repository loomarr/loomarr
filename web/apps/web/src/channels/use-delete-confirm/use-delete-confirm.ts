import { useCallback, useState } from "react";

// useDeleteConfirm owns the arm→confirm→cancel state for a consequential channel removal. The
// channels-list uses it for permanent deletion; the detail danger zone uses it for two explicit
// choices, stop-managing and delete-from-Loomarr-and-Tunarr. Keeping the headless state here means
// those callers can draw context-appropriate copy and styling without reimplementing the flow.
// It holds no channel identity and renders nothing. A plain confirm, not a typed-name gate (a
// household app shouldn't make you type a channel's exact name to proceed).
interface DeleteConfirm {
  // confirming: whether the second step (the execute button) is showing.
  confirming: boolean;
  // arm: show the confirm step. cancel: hide it. reset: alias for cancel, for close handlers.
  arm: () => void;
  cancel: () => void;
  reset: () => void;
}

const useDeleteConfirm = (): DeleteConfirm => {
  const [confirming, setConfirming] = useState(false);
  const arm = useCallback(() => setConfirming(true), []);
  const cancel = useCallback(() => setConfirming(false), []);
  return { confirming, arm, cancel, reset: cancel };
};

export type { DeleteConfirm };
export { useDeleteConfirm };
