import { useCallback, useState } from "react";

// useDeleteConfirm owns the arm→confirm→cancel state for an irreversible delete — the two-step
// gate ("Delete channel" arms it, "Delete permanently" executes) shared by the channels-list
// row menu AND the detail-page danger zone, so the confirm flow lives in ONE place instead of
// being reimplemented (and drifting) in each. It is HEADLESS: it holds no channel identity and
// renders nothing — each caller draws its own context-appropriate JSX (the row menu swallows
// clicks inside its <Link> and styles items as menu rows; the danger zone adds a purge
// checkbox). A plain confirm, not a typed-name gate (a household app shouldn't make you type a
// channel's exact name to delete it).
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
