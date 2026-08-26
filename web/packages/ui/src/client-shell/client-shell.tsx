import { Action, BrandLockup, Screen, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { View } from "react-native";

import { ClientNavigation, clientDestinationLabel } from "../client-navigation";
import { ModalOverlay } from "../overlay";
import type { ClientShellProps } from "./client-shell.type";

const ClientShell = ({
  active,
  children,
  density,
  onDisconnect,
  onNavigate,
  serverName,
}: ClientShellProps) => {
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
  const cancelDisconnect = () => {
    if (disconnecting) return;
    setConfirmingDisconnect(false);
    setDisconnectError(false);
  };

  return (
    <Screen density={density} gap={density === "tv" ? "$control" : "$section"}>
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
      <Surface flex={1} gap="$control" justifyContent="center" level="canvas">
        {children ?? (
          <>
            <Text density={density} textRole="display">
              {clientDestinationLabel(active)}
            </Text>
            <Text density={density} maxWidth={720} textRole="body">
              Your paired client is ready. Guide and playback arrive through the same shared shell without
              changing device authority.
            </Text>
          </>
        )}
      </Surface>
      <ClientNavigation active={active} density={density} onNavigate={onNavigate} />
      <ModalOverlay
        actions={[
          {
            disabled: disconnecting,
            label: "Keep connected",
            onPress: cancelDisconnect,
            preferredFocus: true,
            tone: "secondary",
          },
          {
            disabled: disconnecting,
            label: disconnecting ? "Disconnecting…" : "Disconnect",
            onPress: () => void disconnect(),
            tone: "danger",
          },
        ]}
        density={density}
        description={`Loomarr will revoke this device’s credential on ${serverName ?? "the connected server"}. You can pair it again later.`}
        dismissible={!disconnecting}
        onDismiss={cancelDisconnect}
        title="Disconnect this device?"
        visible={confirmingDisconnect}
      >
        {disconnectError ? (
          <Text density={density} textRole="metadata" tone="danger">
            Loomarr couldn’t disconnect this device. Check the connection and try again.
          </Text>
        ) : null}
      </ModalOverlay>
    </Screen>
  );
};

export { ClientShell };
