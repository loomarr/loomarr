import type { ClipDTO } from "@loomarr/api/models/clipDTO";

interface ClipDetailsEditorProps {
  // Undefined while the list is refetching under an open dialog; the dialog renders
  // nothing rather than flashing empty fields.
  clip: ClipDTO | undefined;
  onClose?: () => void;
  onSaved?: () => void;
}

export type { ClipDetailsEditorProps };
