interface LanguagePickerProps {
  value: string;
  options: readonly string[];
  onChange: (language: string) => void;
  locked?: boolean;
  lockedLabel?: string;
}

export type { LanguagePickerProps };
