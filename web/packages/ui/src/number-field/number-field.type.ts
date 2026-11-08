// Named field by field rather than as the design-system Field's props: those are React Native's
// TextInput props, and Storybook's docgen cannot follow a prop type into react-native.
interface NumberFieldProps {
  accessibilityLabel?: string;
  amount: number;
  "aria-describedby"?: string;
  "aria-labelledby"?: string;
  disabled?: boolean;
  id?: string;
  invalid?: boolean;
  label?: string;
  onAmount: (amount: number) => void;
  onBlur?: () => void;
  placeholder?: string;
}

export type { NumberFieldProps };
