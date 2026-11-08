interface SearchFieldProps {
  // For a search that is the only thing its panel opened to do, such as a model picker.
  autoFocus?: boolean;
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
