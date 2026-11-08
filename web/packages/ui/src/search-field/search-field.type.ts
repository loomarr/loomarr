interface SearchFieldProps {
  disabled?: boolean;
  // Names the input. Printed above the box ("Search people") unless `labelHidden`, for a search
  // whose place on the page already says what it searches.
  label: string;
  labelHidden?: boolean;
  onChangeText: (query: string) => void;
  placeholder?: string;
  value: string;
}

export type { SearchFieldProps };
