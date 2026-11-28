// Named field by field rather than as the design-system Field's props: those are React Native's
// TextInput props, and Storybook's docgen cannot follow a prop type into react-native.
interface YearFieldProps {
  // Overrides the label as the box's name, for rows of identical "From year" boxes.
  accessibilityLabel?: string;
  disabled?: boolean;
  id?: string;
  label?: string;
  onBlur?: () => void;
  onChangeText: (text: string) => void;
  placeholder?: string;
  value: string;
}

export type { YearFieldProps };
