import { Icon, icons, Surface, Text } from "@loomarr/design-system";
import { Platform, Pressable } from "react-native";

import type { ScreenHeaderProps } from "./screen-header.type";

// A phone screen's title row, the same on every tab. iPhone (#1659 native mock 5d): the large title at
// 30/700, 6 pt under the status bar and 10 above what follows. Android (5f): a 64 dp app bar with a
// 24/600 title. The host's 16 pt gutter sets the sides. Each action (at most two) is a plain 44 pt icon
// button in the content colour, never a filled amber one: amber is for focus and the primary action.
const ScreenHeader = ({ actions, title }: ScreenHeaderProps) => {
  const android = Platform.OS === "android";
  return (
    <Surface
      alignItems="center"
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection="row"
      gap="$inline"
      minHeight={android ? 64 : undefined}
      paddingBottom={android ? 0 : 10}
      paddingTop={android ? 0 : 6}
    >
      {/* A long title or a large Dynamic Type size wraps rather than cutting the screen's heading. */}
      <Text accessibilityRole="header" density="touch" flex={1} textRole={android ? "title" : "screenTitle"}>
        {title}
      </Text>
      {actions?.map((action, index) => (
        <Pressable
          accessibilityLabel={action.accessibilityLabel}
          accessibilityRole="button"
          hitSlop={4}
          key={action.accessibilityLabel}
          onPress={action.onPress}
          style={({ pressed }) => ({
            alignItems: "center",
            height: 44,
            justifyContent: "center",
            // The last glyph, not its 44 pt target, sits on the gutter.
            marginRight: index === actions.length - 1 ? -11 : 0,
            opacity: pressed ? 0.6 : 1,
            width: 44,
          })}
        >
          <Icon decorative glyph={icons[action.icon]} size="navigation" tone="content" />
        </Pressable>
      ))}
    </Surface>
  );
};

export { ScreenHeader };
