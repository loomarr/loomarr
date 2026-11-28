interface SearchFieldProps {
  // For a search that is the only thing its panel opened to do, such as a model picker.
  autoFocus?: boolean;
  // Names the clear button that shows while there is a query ("Clear settings search"). Without it
  // there is no clear button, as for a filter whose results sit right beside it.
  clearLabel?: string;
  // The id of the results list the query drives, while there is one to point at.
  controls?: string;
  disabled?: boolean;
  // Names the input. Printed above the box ("Search people") unless `labelHidden`, for a search
  // whose place on the page already says what it searches.
  label: string;
  labelHidden?: boolean;
  // Down arrow from the box, for a finder whose results take focus next.
  onArrowDown?: () => void;
  onChangeText: (query: string) => void;
  placeholder?: string;
  value: string;
}

export type { SearchFieldProps };
