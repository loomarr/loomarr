interface SearchFieldProps {
  disabled?: boolean;
  // The visible label above the box ("Search people"); it also names the input.
  label: string;
  onChangeText: (query: string) => void;
  placeholder?: string;
  value: string;
}

export type { SearchFieldProps };
