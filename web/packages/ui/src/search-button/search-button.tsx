import {
  Icon,
  icons,
  Surface,
  semanticSpace,
  semanticTargets,
  Text,
  useFocusLook,
} from "@loomarr/design-system";
import { Pressable } from "react-native";

import type { SearchButtonProps } from "./search-button.type";

// SearchButton — the SearchField's press-only form: the same box (the Field's border, radius,
// height and padding), the magnifier in front and the placeholder's words, but a press opens the
// search instead of typing in place. For a finder that is its own surface (the shell's command
// search): the box says "search lives here" and the shortcut hint at the trailing edge says how to
// reach it without the pointer. The placeholder is the button's name, as it was on the rail.
const SearchButton = ({ hint, keyShortcuts, onPress, placeholder }: SearchButtonProps) => {
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityRole="button"
      {...(keyShortcuts ? ({ "aria-keyshortcuts": keyShortcuts } as object) : {})}
      {...focusHandlers}
      onPress={onPress}
      // The box's border is the focus look; the browser's own ring would draw outside it.
      style={{ outlineStyle: "solid", outlineWidth: 0, width: "100%" }}
    >
      <Surface
        alignItems="center"
        backgroundColor="surfaceCanvas"
        borderColor={focused ? "actionFocus" : "borderControl"}
        borderRadius="control"
        borderWidth={focused ? 3 : 2}
        flexDirection="row"
        gap={semanticSpace.inline}
        minHeight={semanticTargets.pointer}
        paddingHorizontal={semanticSpace.control}
      >
        <Icon decorative glyph={icons.search} size="compact" tone="muted" />
        <Text flexGrow={1} flexShrink={1} textAlign="left" textRole="body" tone="muted">
          {placeholder}
        </Text>
        {hint ? (
          <Text as="kbd" textRole="mono" tone="secondary">
            {hint}
          </Text>
        ) : null}
      </Surface>
    </Pressable>
  );
};

export { SearchButton };
