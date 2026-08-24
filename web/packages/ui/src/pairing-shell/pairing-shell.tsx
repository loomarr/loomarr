import type { PairingState } from "@loomarr/core/pairing";
import {
  Action,
  ActivityIndicator,
  BrandLockup,
  BrandMark,
  Field,
  QrCode,
  Screen,
  Surface,
  Text,
} from "@loomarr/design-system";
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
  const awaitingApproval = state.status === "awaiting-approval";
  return (
    <Screen alignItems="center" density={density} gap="$section" justifyContent="center">
      {awaitingApproval ? (
        <BrandMark
          contained={false}
          decorative
          size={density === "tv" ? 14 : 10}
          width={density === "tv" ? 200 : 140}
        />
      ) : null}
      <Surface
        gap="$section"
        level="overlay"
        maxWidth={density === "tv" ? 1040 : 620}
        padding="$section"
        width="100%"
      >
        {awaitingApproval ? null : <BrandLockup size={density === "tv" ? "large" : "medium"} />}
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
        <Surface
          backgroundColor="$transparent"
          borderWidth={0}
          flexDirection={density === "tv" ? "row" : "column"}
          gap="$section"
          width="100%"
        >
          <Surface alignItems="center" backgroundColor="$transparent" borderWidth={0} flex={1} gap="$control">
            <Text density={density} textRole="title">
              Scan QR Code
            </Text>
            <QrCode size={density === "tv" ? 220 : 180} value={state.verificationUriComplete} />
          </Surface>
          <Surface
            alignSelf="stretch"
            backgroundColor="$borderDecorative"
            borderWidth={0}
            height={density === "tv" ? "auto" : 1}
            minHeight={density === "tv" ? 220 : 1}
            width={density === "tv" ? 1 : "100%"}
          />
          <Surface
            alignItems="center"
            backgroundColor="$transparent"
            borderWidth={0}
            flex={1}
            gap="$control"
            justifyContent="flex-start"
          >
            <Text density={density} textRole="title">
              Visit Website
            </Text>
            <Text density={density} numberOfLines={1} selectable textRole="time">
              {state.verificationUri}
            </Text>
            <Surface alignItems="center" gap="$inline" level="focus" padding="$control" width="100%">
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
          </Surface>
        </Surface>
        <Surface backgroundColor="$borderDecorative" borderWidth={0} height={1} width="100%" />
        <Surface alignItems="center" backgroundColor="$transparent" borderWidth={0} gap="$control">
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
        </Surface>
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
