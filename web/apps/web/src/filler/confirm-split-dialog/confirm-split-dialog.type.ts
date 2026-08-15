import type { ClipDTO } from "@loomarr/api";

interface ConfirmSplitDialogProps {
  // The clip awaiting confirmation, or undefined when nothing is pending. ⚠ Nullable rather than
  // the dialog being conditionally mounted: the page resolves it from the CURRENT catalog page, so
  // a clip that vanishes under a filter closes the dialog instead of confirming a stale identity.
  clip?: ClipDTO;
  onConfirm: () => void;
  onClose: () => void;
}

export type { ConfirmSplitDialogProps };
