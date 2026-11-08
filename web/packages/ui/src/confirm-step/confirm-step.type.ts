interface ConfirmStepProps {
  // The action's own name ("Remove device"), so the confirm says what it does rather than "OK".
  confirmLabel: string;
  disabled?: boolean;
  // What happens, and that it can't be taken back, in plain words.
  message: string;
  onCancel: () => void;
  onConfirm: () => void;
}

export type { ConfirmStepProps };
