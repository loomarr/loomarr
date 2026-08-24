import { Action, BrandLockup, Screen, Surface, Text } from "@loomarr/design-system";
import { View } from "react-native";

import type { ClientDestination, ClientShellProps } from "./client-shell.type";

const destinations: ReadonlyArray<{ label: string; value: ClientDestination }> = [
  { label: "Watching", value: "watching" },
  { label: "Guide", value: "guide" },
  { label: "Surf", value: "surf" },
];

const ClientShell = ({ active, density, onNavigate, serverName }: ClientShellProps) => (
  <Screen density={density} gap="$section">
    <View style={{ alignItems: "center", flexDirection: "row", justifyContent: "space-between" }}>
      <BrandLockup size={density === "tv" ? "large" : "medium"} />
      <Text density={density} textRole="metadata">
        {serverName ? `Connected to ${serverName}` : "Connected"}
      </Text>
    </View>
    <Surface flex={1} gap="$control" justifyContent="center" level="canvas">
      <Text density={density} textRole="display">
        {destinations.find((item) => item.value === active)?.label}
      </Text>
      <Text density={density} maxWidth={720} textRole="body">
        Your paired client is ready. Guide and playback arrive through the same shared shell without changing
        device authority.
      </Text>
    </Surface>
    <View style={{ flexDirection: density === "touch" ? "column" : "row", gap: density === "tv" ? 24 : 12 }}>
      {destinations.map((destination) => (
        <Action
          accessibilityRole="button"
          density={density}
          hasTVPreferredFocus={density === "tv" && active === destination.value}
          key={destination.value}
          onPress={() => onNavigate(destination.value)}
          tone={active === destination.value ? "primary" : "secondary"}
        >
          {destination.label}
        </Action>
      ))}
    </View>
  </Screen>
);

export { ClientShell };
