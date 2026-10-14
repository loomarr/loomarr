interface IntentTemplate {
  label: string;
  value: string;
}

interface IntentInputProps {
  value: string;
  onValueChange: (value: string) => void;
  onSubmit?: () => void;
  templates?: IntentTemplate[];
  submitting?: boolean;
  placeholder?: string;
  className?: string;
}

export type { IntentInputProps, IntentTemplate };
