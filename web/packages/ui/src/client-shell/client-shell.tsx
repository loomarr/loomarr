import { Action, BrandLockup, Screen, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { View } from "react-native";

import type { ClientDestination, ClientShellProps } from "./client-shell.type";

const destinations: ReadonlyArray<{ label: string; value: ClientDestination }> = [
  { label: "Watching", value: "watching" },
  { label: "Guide", value: "guide" },
  { label: "Surf", value: "surf" },
];

const ClientShell = ({ active, density, onDisconnect, onNavigate, serverName }: ClientShellProps) => {
  const [confirmingDisconnect, setConfirmingDisconnect] = useState(false);
  const [disconnecting, setDisconnecting] = useState(false);
  const [disconnectError, setDisconnectError] = useState(false);
  const disconnect = async () => {
    setDisconnecting(true);
    setDisconnectError(false);
    try {
      await onDisconnect();
    } catch {
      setDisconnectError(true);
      setDisconnecting(false);
    }
  };

  return (
    <Screen density={density} gap="$section">
      <View style={{ alignItems: "center", flexDirection: "row", justifyContent: "space-between" }}>
        <BrandLockup size={density === "tv" ? "large" : "medium"} />
        <View style={{ alignItems: "flex-end", gap: density === "tv" ? 12 : 8 }}>
          <Text density={density} textRole="metadata">
            {serverName ? `Connected to ${serverName}` : "Connected"}
          </Text>
          {!confirmingDisconnect ? (
            <Action
              accessibilityRole="button"
              density={density}
              onPress={() => setConfirmingDisconnect(true)}
              tone="secondary"
            >
              Disconnect device
            </Action>
          ) : null}
        </View>
      </View>
      {confirmingDisconnect ? (
        <Surface flex={1} gap="$section" justifyContent="center" level="overlay">
          <Text density={density} textRole="title">
            Disconnect this device?
          </Text>
          <Text density={density} maxWidth={720} textRole="body">
            Loomarr will revoke this device’s credential on {serverName ?? "the connected server"}. You can
            pair it again later.
          </Text>
          {disconnectError ? (
            <Text density={density} textRole="metadata">
              Loomarr couldn’t disconnect this device. Check the connection and try again.
            </Text>
          ) : null}
          <View style={{ flexDirection: density === "touch" ? "column" : "row", gap: 12 }}>
            <Action
              accessibilityRole="button"
              density={density}
              disabled={disconnecting}
              hasTVPreferredFocus={density === "tv"}
              onPress={() => {
                setConfirmingDisconnect(false);
                setDisconnectError(false);
              }}
              tone="secondary"
            >
              Keep connected
            </Action>
            <Action
              accessibilityRole="button"
              density={density}
              disabled={disconnecting}
              onPress={() => void disconnect()}
              tone="danger"
            >
              {disconnecting ? "Disconnecting…" : "Disconnect"}
            </Action>
          </View>
        </Surface>
      ) : (
        <Surface flex={1} gap="$control" justifyContent="center" level="canvas">
          <Text density={density} textRole="display">
            {destinations.find((item) => item.value === active)?.label}
          </Text>
          <Text density={density} maxWidth={720} textRole="body">
            Your paired client is ready. Guide and playback arrive through the same shared shell without
            changing device authority.
          </Text>
        </Surface>
      )}
      {!confirmingDisconnect ? (
        <View
          style={{ flexDirection: density === "touch" ? "column" : "row", gap: density === "tv" ? 24 : 12 }}
        >
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
      ) : null}
    </Screen>
  );
};

export { ClientShell };
