import { Action, BottomSheet, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";

import { confirmDisconnect as confirmNatively } from "./confirm-disconnect";
import type { DeviceSheetProps } from "./device-sheet.type";

const roleLabel = { admin: "Admin", member: "Member" } as const;

// host[:port] of a server URL. A string edit, not `new URL`: React Native's URL leaves `host` unimplemented.
const serverAddress = (serverUrl: string) =>
  serverUrl.replace(/^[a-z][a-z\d+.-]*:\/\//i, "").replace(/[/?#].*$/, "");

// One fact, read by VoiceOver as a single "label, value" stop in the order they are drawn.
const Row = ({ label, value }: { label: string; value: string }) => (
  <Surface
    accessible
    aria-label={`${label}, ${value}`}
    backgroundColor="$transparent"
    borderWidth={0}
    gap={2}
  >
    <Text density="touch" textRole="metadata" tone="secondary">
      {label}
    </Text>
    <Text density="touch" textRole="body">
      {value}
    </Text>
  </Surface>
);

/**
 * The phone's "this device" sheet (opened from a tab header's account button): who is signed in, which
 * server and build this is, and Disconnect. Disconnecting is confirmed natively and then runs the
 * host's `onDisconnect`, which sends the device back to choosing a server.
 */
const DeviceSheet = ({
  account,
  appVersion,
  confirmDisconnect = confirmNatively,
  onDisconnect,
  onDismiss,
  serverUrl,
  serverVersion,
}: DeviceSheetProps) => {
  const [disconnecting, setDisconnecting] = useState(false);
  const [failed, setFailed] = useState(false);
  const address = serverAddress(serverUrl);
  const disconnect = () => {
    setDisconnecting(true);
    setFailed(false);
    // A success unmounts the sheet (the device returns to server choice), so only a failure sets state.
    void Promise.resolve(onDisconnect()).catch(() => {
      setFailed(true);
      setDisconnecting(false);
    });
  };
  return (
    <BottomSheet accessibilityLabel="Device and account" onDismiss={onDismiss}>
      <Text accessibilityRole="header" density="touch" textRole="title">
        This device
      </Text>
      {account ? <Row label="Signed in as" value={`${account.name}, ${roleLabel[account.role]}`} /> : null}
      <Row label="Server" value={address} />
      {serverVersion ? <Row label="Server version" value={serverVersion} /> : null}
      {appVersion ? <Row label="App version" value={appVersion} /> : null}
      {failed ? (
        <Text density="touch" textRole="metadata" tone="danger">
          {`Loomarr couldn't reach ${address}. Try again.`}
        </Text>
      ) : null}
      <Action
        accessibilityRole="button"
        density="touch"
        disabled={disconnecting}
        onPress={() => confirmDisconnect(address, disconnect)}
        tone="danger"
      >
        {disconnecting ? "Disconnecting…" : failed ? "Try disconnecting again" : "Disconnect"}
      </Action>
    </BottomSheet>
  );
};

export { DeviceSheet };
