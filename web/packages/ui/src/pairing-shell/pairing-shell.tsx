import type { PairingState } from "@loomarr/core/pairing";
import { Action, ActivityIndicator, BrandLockup, Field, Screen, Surface, Text } from "@loomarr/design-system";
import { useEffect, useState, useSyncExternalStore } from "react";

import type { PairingShellProps } from "./pairing-shell.type";

const PairingShell = ({
  allowServerEntry = false,
  density,
  initialServerUrl,
  renderPaired,
  session,
}: PairingShellProps) => {
  const state = useSyncExternalStore(session.subscribe, session.snapshot, session.snapshot);
  const [serverUrl, setServerUrl] = useState(initialServerUrl ?? "");
  const [, setTick] = useState(0);

  useEffect(() => {
    void session.initialize(initialServerUrl);
    return () => session.stop();
  }, [initialServerUrl, session]);
  useEffect(() => {
    if (state.status !== "awaiting-approval") return;
    const timer = setInterval(() => setTick((value) => value + 1), 1_000);
    return () => clearInterval(timer);
  }, [state.status]);

  if (state.status === "paired") return renderPaired(state);
  const content = pairingContent(state, density, session, allowServerEntry, serverUrl, setServerUrl);
  return (
    <Screen alignItems="center" density={density} justifyContent="center">
      <Surface
        gap="$section"
        level="overlay"
        maxWidth={density === "tv" ? 920 : 620}
        padding="$section"
        width="100%"
      >
        <BrandLockup size={density === "tv" ? "large" : "medium"} />
        {content}
      </Surface>
    </Screen>
  );
};

const pairingContent = (
  state: PairingState,
  density: PairingShellProps["density"],
  session: PairingShellProps["session"],
  allowServerEntry: boolean,
  serverUrl: string,
  setServerUrl: (value: string) => void,
) => {
  if (state.status === "loading")
    return (
      <>
        <ActivityIndicator accessibilityLabel="Connecting to Loomarr" />
        <Text density={density} textRole="body">
          Connecting to Loomarr…
        </Text>
      </>
    );
  if (state.status === "needs-server")
    return (
      <>
        <Text density={density} textRole="title">
          Connect this device
        </Text>
        <Text density={density} textRole="body">
          Enter your Loomarr address to begin a secure, revocable device pairing.
        </Text>
        {allowServerEntry ? (
          <>
            <Field
              autoCapitalize="none"
              autoCorrect={false}
              density={density}
              onChangeText={setServerUrl}
              placeholder="https://loomarr.example"
              value={serverUrl}
            />
            <Action density={density} onPress={() => void session.pair(serverUrl)}>
              Continue
            </Action>
          </>
        ) : (
          <Text density={density} textRole="metadata">
            Set EXPO_PUBLIC_LOOMARR_URL for this TV build, then restart the app.
          </Text>
        )}
      </>
    );
  if (state.status === "awaiting-approval") {
    const seconds = Math.max(0, Math.ceil((state.expiresAtMs - Date.now()) / 1_000));
    return (
      <>
        <Text density={density} textRole="title">
          Pair this device
        </Text>
        <Text density={density} textRole="body">
          On a signed-in phone or computer, open:
        </Text>
        <Text density={density} selectable textRole="time">
          {state.verificationUri}
        </Text>
        <Surface alignItems="center" gap="$inline" level="focus" padding="$control">
          <Text density={density} textRole="metadata">
            PAIRING CODE
          </Text>
          <Text
            accessibilityLabel={`Pairing code ${state.userCode}`}
            density={density}
            selectable
            textRole="channelNumber"
          >
            {state.userCode}
          </Text>
        </Surface>
        <Text accessibilityLiveRegion="polite" density={density} textRole="metadata">
          Expires in {Math.floor(seconds / 60)}:{String(seconds % 60).padStart(2, "0")}
        </Text>
        <Action
          density={density}
          hasTVPreferredFocus={density === "tv"}
          onPress={() => void session.pair(state.serverUrl)}
          tone="secondary"
        >
          Get a new code
        </Action>
      </>
    );
  }
  if (state.status === "revoked")
    return (
      <>
        <Text density={density} textRole="title">
          This device was disconnected
        </Text>
        <Text density={density} textRole="body">
          Its credential is no longer valid. Pair it again to continue.
        </Text>
        <Action density={density} onPress={() => void session.pair(state.serverUrl)}>
          Pair again
        </Action>
      </>
    );
  if (state.status === "failed")
    return (
      <>
        <Text density={density} textRole="title">
          Couldn’t connect
        </Text>
        <Text density={density} textRole="body">
          {state.message}
        </Text>
        <Action density={density} onPress={() => void session.pair(state.serverUrl ?? serverUrl)}>
          Try again
        </Action>
      </>
    );
  return null;
};

export { PairingShell };
