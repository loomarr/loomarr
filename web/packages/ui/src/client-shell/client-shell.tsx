import { BrandLockup, Screen, Surface, Text } from "@loomarr/design-system";
import { View } from "react-native";

import { ClientNavigation, clientDestinationLabel } from "../client-navigation";
import { DeviceDisconnectAction } from "../device-disconnect";
import type { ClientShellProps } from "./client-shell.type";

const ClientShell = ({
  active,
  children,
  density,
  onDisconnect,
  onNavigate,
  serverName,
}: ClientShellProps) => {
  const navigation = <ClientNavigation active={active} density={density} onNavigate={onNavigate} />;
  // A phone docks its platform's tab bar under the content; pointer and TV keep the row inline.
  const phone = density === "touch";
  // A destination with its own screen (the phone's Guide and Watching) sets its own gutters.
  if (phone && children)
    return (
      <Screen density={density} flush footer={navigation}>
        {children}
      </Screen>
    );
  return (
    <Screen density={density} footer={phone ? navigation : undefined} gap="$section">
      <View style={{ alignItems: "center", flexDirection: "row", justifyContent: "space-between" }}>
        <BrandLockup size={density === "tv" ? "large" : "medium"} />
        <View style={{ alignItems: "flex-end", gap: density === "tv" ? 12 : 8 }}>
          <Text density={density} textRole="metadata">
            {serverName ? `Connected to ${serverName}` : "Connected"}
          </Text>
          <DeviceDisconnectAction density={density} onDisconnect={onDisconnect} serverName={serverName} />
        </View>
      </View>
      <Surface flex={1} gap="$control" justifyContent="center" level="canvas">
        <Text density={density} textRole="display">
          {clientDestinationLabel(active)}
        </Text>
        <Text density={density} maxWidth={720} textRole="body">
          Your paired client is ready. Guide and playback arrive through the same shared shell without
          changing device authority.
        </Text>
      </Surface>
      {phone ? null : navigation}
    </Screen>
  );
};

export { ClientShell };
