import type { ComponentProps } from "react";
import type { TextInput } from "react-native";

interface SearchFieldProps {
  disabled?: boolean;
  // Names the input. Printed above the box ("Search people") unless `labelHidden`, for a search
  // whose place on the page already says what it searches.
  label: string;
  labelHidden?: boolean;
  onChangeText: (query: string) => void;
  // A key the box should hand on, such as ArrowDown into the results below it.
  onKeyPress?: ComponentProps<typeof TextInput>["onKeyPress"];
  placeholder?: string;
  value: string;
}

export type { SearchFieldProps };
