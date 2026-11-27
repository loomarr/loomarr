// Named field by field rather than as the design-system Field's props: those are React Native's
// TextInput props, and Storybook's docgen cannot follow a prop type into react-native.
interface CommitFieldProps {
  accessibilityLabel?: string;
  autoCapitalize?: "characters" | "none" | "sentences" | "words";
  disabled?: boolean;
  id?: string;
  // The text the box opens with. Remount the box (a `key` on the stored value) to reseed it.
  initial: string;
  keyboardType?: "decimal-pad" | "default" | "number-pad";
  label?: string;
  maxLength?: number;
  // The draft, handed over once the box loses focus.
  onCommit: (text: string) => void;
  placeholder?: string;
  // Why a draft cannot be handed over, in words the box shows under it; undefined when it can.
  validate?: (text: string) => string | undefined;
  // A fixed width for a short value; without it the box fills its container.
  width?: number;
}

export type { CommitFieldProps };
