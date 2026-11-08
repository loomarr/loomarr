import {
  Field,
  Icon,
  icons,
  Surface,
  semanticSpace,
  semanticTargets,
  useFocusLook,
} from "@loomarr/design-system";
import { Pressable } from "react-native";

import type { SearchFieldProps } from "./search-field.type";

// The clear button inside the box's trailing edge: a press empties the query. The amber ring around
// the glyph is its one focus look, as on the password Field's reveal button.
const ClearButton = ({ label, onPress }: { label: string; onPress: () => void }) => {
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole="button"
      {...focusHandlers}
      onPress={onPress}
      style={{
        alignItems: "center",
        bottom: 0,
        justifyContent: "center",
        outlineStyle: "solid",
        outlineWidth: 0,
        position: "absolute",
        right: 0,
        width: semanticTargets.pointer,
        height: semanticTargets.pointer,
      }}
    >
      <Surface
        backgroundColor="transparent"
        borderColor={focused ? "actionFocus" : "transparent"}
        borderRadius="control"
        borderWidth={2}
        padding={2}
      >
        <Icon decorative glyph={icons.close} size="compact" tone={focused ? "content" : "muted"} />
      </Surface>
    </Pressable>
  );
};

// SearchField — a list filter's text box with the magnifier inside its leading edge (People,
// import, Settings, the filler catalog). The design-system Field, with its left padding opened up so
// the glyph sits in front of the text rather than over it. The glyph is decoration: the label names
// it, printed above the box or, with `labelHidden`, only announced. `inputMode="search"` makes the
// web input `type="search"`, so assistive tech announces a search box, and gives phones a Search key.
const SearchField = ({
  autoFocus,
  clearLabel,
  controls,
  disabled,
  label,
  labelHidden = false,
  onArrowDown,
  onChangeText,
  placeholder,
  value,
}: SearchFieldProps) => {
  const clearable = Boolean(clearLabel) && value !== "" && !disabled;
  return (
    <Surface backgroundColor="transparent" borderWidth={0} position="relative" width="100%">
      <Field
        accessibilityLabel={label}
        aria-controls={controls}
        autoCapitalize="none"
        autoCorrect={false}
        autoFocus={autoFocus}
        disabled={disabled}
        inputMode="search"
        label={labelHidden ? undefined : label}
        onChangeText={onChangeText}
        onKeyPress={(event) => {
          if (event.nativeEvent.key !== "ArrowDown" || !onArrowDown) return;
          event.preventDefault();
          onArrowDown();
        }}
        placeholder={placeholder}
        style={{
          paddingLeft: semanticSpace.control + 24,
          paddingRight: clearable ? semanticTargets.pointer : semanticSpace.control,
        }}
        value={value}
      />
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        bottom={0}
        height={semanticTargets.pointer}
        justifyContent="center"
        left={semanticSpace.control}
        pointerEvents="none"
        position="absolute"
      >
        <Icon decorative glyph={icons.search} size="compact" tone="muted" />
      </Surface>
      {clearable && clearLabel ? <ClearButton label={clearLabel} onPress={() => onChangeText("")} /> : null}
    </Surface>
  );
};

export { SearchField };
