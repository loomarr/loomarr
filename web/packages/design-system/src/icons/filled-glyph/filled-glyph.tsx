import type { LucideIcon, LucideProps } from "lucide-react-native";

// A glyph drawn solid: the outline glyph with its shape filled in its own colour, so the filled
// form follows the tone, hover and theme of the outline one and the pair differ only by the fill.
const filled = (Outline: LucideIcon): LucideIcon => {
  const Solid = ({ color, ...props }: LucideProps) => <Outline color={color} fill={color} {...props} />;
  return Solid as LucideIcon;
};

export { filled };
