import { Icon, icons, StatusDot, Surface, Text } from "@loomarr/design-system";
import { Pressable } from "react-native";

import type { ServerRowProps } from "./server-row.type";

// A found server on a phone: an iOS list row (name over a grey host, a disclosure chevron), the way
// iOS lists Wi-Fi networks, rather than a primary button whose label had to carry both lines
// (maintainer, 2026-10-13). The whole 64 pt row is the target and connects.
const ServerRow = ({ onPress, server }: ServerRowProps) => {
  const host = server.url.replace(/^https?:\/\//i, "");
  return (
    <Pressable
      accessibilityHint="Connects and shows a code to approve this device"
      accessibilityLabel={`${server.name}, ${host}`}
      accessibilityRole="button"
      onPress={onPress}
      style={({ pressed }) => ({ opacity: pressed ? 0.82 : 1, width: "100%" })}
    >
      <Surface
        alignItems="center"
        flexDirection="row"
        gap="$control"
        level="elevated"
        minHeight={64}
        paddingHorizontal="$control"
        paddingVertical="$inline"
      >
        <StatusDot label="Found on this network" tone="ok" />
        <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={2}>
          <Text density="touch" numberOfLines={1} textRole="label">
            {server.name}
          </Text>
          <Text density="touch" numberOfLines={1} textRole="metadata" tone="muted">
            {host}
          </Text>
        </Surface>
        <Icon decorative glyph={icons.disclosure} size="default" tone="secondary" />
      </Surface>
    </Pressable>
  );
};

export { ServerRow };
