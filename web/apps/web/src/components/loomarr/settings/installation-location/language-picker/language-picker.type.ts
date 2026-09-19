interface LanguagePickerProps {
  value: string;
  onChange: (language: string) => void;
  locked?: boolean;
  lockedLabel?: string;
}

export type { LanguagePickerProps };
