import type { ClipDTO } from "@loomarr/api/models/clipDTO";

type PinClipDialogProps = {
  // The clip to pin into a channel — undefined closes the dialog (same open/close idiom as
  // ClipTagDialog, an inline Card rather than a fixed overlay).
  clip?: ClipDTO;
  onClose: () => void;
};

export type { PinClipDialogProps };
