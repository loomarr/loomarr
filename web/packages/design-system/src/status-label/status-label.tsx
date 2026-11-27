import { useTheme } from "@tamagui/core";
import type { ReactNode } from "react";
import { Pressable, View } from "react-native";
import { useFocusLook } from "../focus-look";
import { StatusDot, type StatusTone } from "../primitives";
import { Text } from "../text";

type StatusLabelProps = {
  /** The sentence-case word: "Enabled", "Admin", "Set via environment". */
  children: ReactNode;
  /**
   * Makes the label a button that does something about the state it names ("Overriding LOOMARR_PORT"
   * hands the key back). It reads as a button, takes a focus stop and draws the one amber outline.
   */
  onPress?: () => void;
  /**
   * Names the button when the visible word alone is not the action. It must start with the visible
   * word (WCAG 2.5.3, axe `label-content-name-mismatch`): "Overriding LOOMARR_PORT: hand Port back".
   */
  "aria-label"?: string;
  tone?: StatusTone;
};

const Body = ({ children, tone }: { children: ReactNode; tone: StatusTone }) => (
  <View style={{ alignItems: "center", alignSelf: "flex-start", flexDirection: "row", gap: 6 }}>
    <StatusDot label="" size="compact" tone={tone} />
    <Text textRole="compact" tone="secondary">
      {children}
    </Text>
  </View>
);

const PressableBody = ({
  "aria-label": ariaLabel,
  children,
  onPress,
  tone,
}: StatusLabelProps & { onPress: () => void; tone: StatusTone }) => {
  const theme = useTheme();
  const focus = useFocusLook();
  return (
    <Pressable
      accessibilityLabel={ariaLabel}
      accessibilityRole="button"
      onBlur={focus.onBlur}
      onFocus={focus.onFocus}
      onPress={onPress}
      style={{
        alignSelf: "flex-start",
        borderRadius: 999,
        // The amber outline is the focus look; it sits outside the box so nothing beside it moves.
        outlineColor: theme.actionFocus.val,
        outlineOffset: 2,
        outlineStyle: "solid",
        outlineWidth: focus.focused ? 2 : 0,
      }}
    >
      <Body tone={tone}>{children}</Body>
    </Pressable>
  );
};

/**
 * Quiet status: a small dot and a sentence-case word, with no fill. Where a filled chip shouts
 * (ADMIN, ENABLED, DEFAULT), this reads as part of the row. The word carries the meaning; the dot
 * only tints it, so it is hidden from assistive tech. Given `onPress` it is a button.
 */
const StatusLabel = ({ children, onPress, tone = "off", ...rest }: StatusLabelProps) =>
  onPress ? (
    <PressableBody onPress={onPress} tone={tone} {...rest}>
      {children}
    </PressableBody>
  ) : (
    <Body tone={tone}>{children}</Body>
  );

export type { StatusLabelProps };
export { StatusLabel };
