import { isWeb, useTheme, View } from "@tamagui/core";
import { useId, useState } from "react";
import { Platform, Pressable, TextInput } from "react-native";

import { useFocusLook } from "../../focus-look";
import { Icon } from "../../icon";
import { icons } from "../../icons";
import { Text } from "../../text";
import {
  type Density,
  nativeFontFace,
  semanticRadius,
  semanticSpace,
  semanticTargets,
  typography,
} from "../../tokens";
import { boxFocusRing } from "../box-focus-ring";
import type { FieldProps } from "./field.type";

// A password Field's button: the label names what a press does, so it flips with the state.
const RevealButton = ({
  density,
  disabled,
  onPress,
  revealed,
}: {
  density: Density;
  disabled: boolean;
  onPress: () => void;
  revealed: boolean;
}) => {
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityLabel={revealed ? "Hide password" : "Show password"}
      accessibilityRole="button"
      aria-pressed={revealed}
      disabled={disabled}
      {...focusHandlers}
      onPress={onPress}
      style={{
        alignItems: "center",
        bottom: 0,
        justifyContent: "center",
        // The ring around the glyph is the one focus look. Chrome's `auto` ring ignores a zero
        // width, so the style is named too.
        outlineStyle: "solid",
        outlineWidth: 0,
        position: "absolute",
        right: 0,
        top: 0,
        width: semanticTargets[density],
      }}
    >
      <View borderRadius="control" padding={2} {...boxFocusRing(focused, density)}>
        <Icon
          decorative
          glyph={revealed ? icons.conceal : icons.reveal}
          size={density === "tv" ? "control" : "compact"}
          tone={focused ? "content" : "muted"}
        />
      </View>
    </Pressable>
  );
};

// React Native Web's TextInput stops every keydown at the input (its "#612"), so a dialog or menu
// listening on the document never hears Escape from inside a Field and stays open. The stopped
// event cannot be resumed, so a fresh Escape is raised from the input's parent. This package has no
// DOM lib, so the slice of the DOM it touches is typed here.
type KeyTarget = { parentElement: { dispatchEvent(event: unknown): boolean } | null };
const KeyboardEventCtor = (globalThis as { KeyboardEvent?: new (type: string, init: object) => unknown })
  .KeyboardEvent;

const reraiseEscape = (event: { nativeEvent: { key: string }; target: unknown }) => {
  if (!isWeb || event.nativeEvent.key !== "Escape" || !KeyboardEventCtor) return;
  (event.target as KeyTarget).parentElement?.dispatchEvent(
    new KeyboardEventCtor("keydown", { bubbles: true, cancelable: true, key: "Escape" }),
  );
};

const Field = ({
  accessibilityLabel,
  density = "pointer",
  description,
  disabled = false,
  error,
  id,
  invalid = false,
  label,
  onBlur,
  onFocus,
  onKeyPress,
  secureTextEntry = false,
  style,
  ...props
}: FieldProps) => {
  const theme = useTheme();
  const { focused, ...focusHandlers } = useFocusLook({ onBlur, onFocus });
  const generatedId = useId();
  const inputId = id ?? generatedId;
  // The line under the input, error before help, is the input's accessible description. Ids derive
  // from the input's, so two fields never share one.
  const messageId = `${inputId}-${error ? "error" : "description"}`;
  const hasMessage = Boolean(error || description);
  // A caller's own aria-describedby (outside RN's TextInput types) stays, ahead of the message's.
  const callerDescribedBy = (props as { "aria-describedby"?: string })["aria-describedby"];
  const describedBy = [callerDescribedBy, messageId].filter(Boolean).join(" ");
  const [revealed, setRevealed] = useState(false);
  const hasError = invalid || Boolean(error);
  const borderColor = hasError
    ? theme.stateDanger.val
    : focused
      ? theme.actionFocus.val
      : theme.borderControl.val;

  return (
    <View gap="inline" width="100%">
      {label ? (
        <Text
          as={isWeb ? "label" : undefined}
          density={density}
          textRole="label"
          {...(isWeb ? { htmlFor: inputId } : {})}
        >
          {label}
        </Text>
      ) : null}
      <View position="relative" width="100%">
        <TextInput
          {...props}
          {...(hasMessage ? ({ "aria-describedby": describedBy } as object) : {})}
          accessibilityLabel={accessibilityLabel ?? label}
          accessibilityState={{ disabled }}
          aria-disabled={disabled || undefined}
          aria-invalid={hasError || undefined}
          editable={!disabled}
          id={inputId}
          onKeyPress={(event) => {
            onKeyPress?.(event);
            reraiseEscape(event);
          }}
          secureTextEntry={secureTextEntry && !revealed}
          {...focusHandlers}
          placeholderTextColor={theme.contentMuted.val}
          style={[
            {
              backgroundColor: disabled ? theme.surfaceRaised.val : theme.surfaceCanvas.val,
              borderColor,
              borderRadius: semanticRadius.control,
              borderWidth: focused ? 3 : 2,
              color: theme.contentPrimary.val,
              // A raw TextInput: Tamagui's per-weight face swap doesn't reach it, so name the file.
              // The body face: what someone types, and a placeholder, are words, and mono is for
              // channel numbers and clocks (#2778, #3412).
              fontFamily: isWeb
                ? typography.family.body.web
                : nativeFontFace("body", Platform.OS === "android" ? "android" : "ios"),
              fontSize: typography[density].body.size,
              minHeight: semanticTargets[density],
              opacity: disabled ? 0.62 : 1,
              // The amber border is the focus look; the browser would add its own ring outside it.
              outlineStyle: "solid",
              outlineWidth: 0,
              paddingHorizontal: semanticSpace.control,
              paddingRight: secureTextEntry ? semanticTargets[density] : semanticSpace.control,
              // A multiline Field is a text area: the text starts at the top-left corner, inset as it
              // is from the sides, rather than centred in a box sized for one line.
              ...(props.multiline
                ? {
                    lineHeight: typography[density].body.lineHeight,
                    paddingVertical: semanticSpace.inline,
                    textAlignVertical: "top" as const,
                  }
                : {}),
            },
            style,
          ]}
        />
        {secureTextEntry ? (
          <RevealButton
            density={density}
            disabled={disabled}
            onPress={() => setRevealed((shown) => !shown)}
            revealed={revealed}
          />
        ) : null}
      </View>
      {error ? (
        <Text aria-live="polite" density={density} id={messageId} textRole="hint" tone="danger">
          {error}
        </Text>
      ) : description ? (
        <Text density={density} id={messageId} textRole="hint" tone="secondary">
          {description}
        </Text>
      ) : null}
    </View>
  );
};

export { Field };
