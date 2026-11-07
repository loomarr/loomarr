import type { PairingState } from "@loomarr/core/pairing";
import type { ServerDiscoverySnapshot } from "@loomarr/core/server-discovery";
import {
  Action,
  ActivityIndicator,
  Badge,
  BrandLockup,
  BrandMark,
  Field,
  QrCode,
  Screen,
  ScrollFrame,
  Surface,
  semanticSpace,
  Text,
} from "@loomarr/design-system";
import { type ComponentRef, useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { BackHandler, Keyboard, KeyboardAvoidingView, Platform, type ScrollView } from "react-native";

import type { ApprovalBrowser, PairingShellProps } from "./pairing-shell.type";
import { PhoneApproval } from "./phone-approval";
import { serverAddressLabel } from "./server-address";
import { ServerRow } from "./server-row";

const emptyDiscovery: ServerDiscoverySnapshot = { servers: [], status: "unavailable" };
const subscribeToNothing = () => () => undefined;
const readEmptyDiscovery = () => emptyDiscovery;

/**
 * A typo'd address is rejected before any request (#1966), so it isn't a failed connection: the
 * server-choice screen stays up with the text kept in its field and the message under it.
 */
const isInvalidAddress = (state: PairingState) =>
  state.status === "failed" && state.kind === "invalid-address";

/**
 * Whether Back leaves this pairing screen for server choice: the code screen and a failed connection
 * both offer "another server". The other screens are a root, where Back leaves the app. An Apple TV
 * hands its Menu key to the app only while this holds, so Menu still goes Home from server choice.
 * A rejected address is already on server choice, so it is not one of them.
 */
const pairingBackAvailable = (state: PairingState) =>
  state.status === "awaiting-approval" || (state.status === "failed" && !isInvalidAddress(state));

const PairingShell = ({
  allowServerEntry = false,
  approvalBrowser,
  density,
  discovery,
  discoveryForeground = false,
  initialServerUrl,
  renderPaired,
  session,
}: PairingShellProps) => {
  const state = useSyncExternalStore(session.subscribe, session.snapshot, session.snapshot);
  const discoveryState = useSyncExternalStore(
    discovery?.subscribe ?? subscribeToNothing,
    discovery?.snapshot ?? readEmptyDiscovery,
    discovery?.snapshot ?? readEmptyDiscovery,
  );
  const [serverUrl, setServerUrl] = useState(initialServerUrl ?? "");
  const [manualEntry, setManualEntry] = useState(!discovery);
  const [, setTick] = useState(0);
  // A phone's keyboard covers the bottom of the page, where Continue is: when it opens, the page
  // (already lifted above it) scrolls to its end so the field and Continue both show.
  const page = useRef<ComponentRef<typeof ScrollView>>(null);
  useEffect(() => {
    if (density !== "touch") return undefined;
    const subscription = Keyboard.addListener("keyboardDidShow", () =>
      page.current?.scrollToEnd({ animated: true }),
    );
    return () => subscription.remove();
  }, [density]);

  useEffect(() => {
    void session.initialize(initialServerUrl);
    return () => session.stop();
  }, [initialServerUrl, session]);
  useEffect(() => {
    if (state.status !== "awaiting-approval") return;
    const timer = setInterval(() => setTick((value) => value + 1), 1_000);
    return () => clearInterval(timer);
  }, [state.status]);
  // Leaving approval (paired, failed, or another server) closes the browser sheet, so approving on
  // `/pair` lands the viewer straight in the app. A silent code refresh stays in approval and keeps it.
  useEffect(() => {
    if (state.status !== "awaiting-approval" || !approvalBrowser) return undefined;
    return () => approvalBrowser.close();
  }, [approvalBrowser, state.status]);
  const invalidAddress = isInvalidAddress(state);
  // The rejected text goes back into the field, so the viewer fixes it rather than retypes it.
  const rejectedInput = state.status === "failed" ? state.input : undefined;
  useEffect(() => {
    if (rejectedInput !== undefined) setServerUrl(rejectedInput);
  }, [rejectedInput]);
  const choosingServer = state.status === "needs-server" || invalidAddress;
  useEffect(() => {
    if (!choosingServer || !discovery || !discoveryForeground) {
      discovery?.stop();
      return undefined;
    }
    discovery.start();
    return () => discovery.stop();
  }, [choosingServer, discovery, discoveryForeground]);
  const chooseAnotherServer = useCallback(() => {
    setManualEntry(false);
    session.chooseServer();
  }, [session]);
  const backAvailable = pairingBackAvailable(state);
  useEffect(() => {
    if (!backAvailable) return undefined;
    const subscription = BackHandler.addEventListener("hardwareBackPress", () => {
      chooseAnotherServer();
      return true;
    });
    return () => subscription.remove();
  }, [backAvailable, chooseAnotherServer]);

  if (state.status === "paired") return renderPaired(state);
  const content = pairingContent(
    state,
    density,
    session,
    allowServerEntry,
    serverUrl,
    setServerUrl,
    discovery ? discoveryState : undefined,
    manualEntry,
    setManualEntry,
    chooseAnotherServer,
    approvalBrowser,
  );
  const awaitingApproval = state.status === "awaiting-approval";
  const body = (
    <>
      {awaitingApproval ? (
        density === "tv" ? (
          <BrandMark contained={false} decorative size={24} width={320} />
        ) : (
          <BrandLockup orientation="horizontal" size="medium" />
        )
      ) : null}
      <Surface
        // Spread: an explicit `borderRadius={undefined}` strips the level's own radius in Tamagui 3.
        {...(density === "tv" && awaitingApproval ? { borderRadius: 8 } : {})}
        gap={density === "tv" && awaitingApproval ? 0 : "section"}
        maxWidth={density === "tv" && awaitingApproval ? 760 : 620}
        padding="section"
        width="100%"
        level={density === "tv" && awaitingApproval ? "raised" : "overlay"}
      >
        {awaitingApproval ? null : <BrandLockup size={density === "tv" ? "large" : "medium"} />}
        {content}
      </Surface>
    </>
  );
  // A phone's keyboard would cover the server field's Continue: the page lifts above it and scrolls.
  if (density === "touch")
    return (
      <Screen density={density}>
        <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={{ flex: 1 }}>
          <ScrollFrame
            contentContainerStyle={{
              alignItems: "center",
              gap: semanticSpace.section,
              justifyContent: "center",
            }}
            density={density}
            ref={page}
          >
            {body}
          </ScrollFrame>
        </KeyboardAvoidingView>
      </Screen>
    );
  return (
    <Screen alignItems="center" density={density} gap="section" justifyContent="center">
      {body}
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
  discovery: ServerDiscoverySnapshot | undefined,
  manualEntry: boolean,
  setManualEntry: (value: boolean) => void,
  chooseAnotherServer: () => void,
  approvalBrowser: ApprovalBrowser | undefined,
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
  if (state.status === "needs-server" || isInvalidAddress(state)) {
    const addressError = state.status === "failed" ? state.message : undefined;
    return (
      <>
        <Text density={density} textRole="title">
          Find your Loomarr server
        </Text>
        <Text density={density} textRole="body">
          {`Choose a server on this network. You’ll approve this ${density === "tv" ? "TV" : "device"} on the next screen.`}
        </Text>
        {discovery?.servers.map((server, index) =>
          density === "touch" ? (
            <ServerRow key={server.id} onPress={() => void session.pair(server.url)} server={server} />
          ) : (
            <Action
              density={density}
              hasTVPreferredFocus={density === "tv" && index === 0}
              key={server.id}
              onPress={() => void session.pair(server.url)}
            >
              {`${discovery.servers.length === 1 ? `Connect to ${server.name}` : server.name}\n${serverAddressLabel(server.url)}`}
            </Action>
          ),
        )}
        {discovery?.servers.length === 0 && discovery.status === "searching" ? (
          <Text accessibilityLiveRegion="polite" density={density} textRole="metadata">
            Searching this network…
          </Text>
        ) : null}
        {discovery?.error ? (
          // On TV, amber is reserved for focus and the primary action (#1909 Q0); the notice offers
          // manual entry right below it, so it reads as information, not a warning.
          <Text
            accessibilityLiveRegion="polite"
            density={density}
            textRole="metadata"
            tone={density === "tv" ? undefined : "warning"}
          >
            {discovery.error}
          </Text>
        ) : null}
        {allowServerEntry ? (
          manualEntry || addressError ? (
            <>
              <Field
                accessibilityLabel="Loomarr server address"
                autoCapitalize="none"
                autoCorrect={false}
                density={density}
                error={addressError}
                hasTVPreferredFocus={density === "tv"}
                keyboardType="url"
                onChangeText={setServerUrl}
                onSubmitEditing={() => void session.pair(serverUrl)}
                placeholder="http://loomarr.local:8080"
                returnKeyType="go"
                value={serverUrl}
              />
              <Action density={density} onPress={() => void session.pair(serverUrl)}>
                Continue
              </Action>
            </>
          ) : (
            <Action
              density={density}
              hasTVPreferredFocus={density === "tv" && discovery?.servers.length === 0}
              onPress={() => setManualEntry(true)}
              tone="secondary"
            >
              Enter address manually
            </Action>
          )
        ) : (
          <Text density={density} textRole="metadata">
            Server selection is unavailable in this client.
          </Text>
        )}
      </>
    );
  }
  if (state.status === "awaiting-approval") {
    const seconds = Math.max(0, Math.ceil((state.expiresAtMs - Date.now()) / 1_000));
    // Maintainer decision (#1817, pairing states): urgent under 60s, danger + semibold.
    const urgent = seconds < 60;
    const countdownLabel = `Expires in ${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
    if (density === "touch" && approvalBrowser)
      return (
        <PhoneApproval
          browserName={approvalBrowser.name}
          countdownLabel={countdownLabel}
          onApprove={() => approvalBrowser.open(state.verificationUriComplete)}
          onChooseServer={chooseAnotherServer}
          state={state}
          urgent={urgent}
        />
      );
    return (
      <>
        {density === "tv" ? (
          <Text density={density} textRole="title">
            Approve this TV
          </Text>
        ) : null}
        <Surface
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection={density === "tv" ? "row" : "column"}
          gap={density === "tv" ? 0 : "section"}
          width="100%"
        >
          {/* The halves share the TV's row. In a phone's column, flex's zero basis would give each no
              height on native, so the QR, the code and the button drew over each other. */}
          <Surface
            alignItems="center"
            backgroundColor="transparent"
            borderWidth={0}
            flex={density === "tv" ? 1 : undefined}
            gap="control"
          >
            <Text density={density} textRole={density === "tv" ? "section" : "title"}>
              {density === "tv" ? "SCAN QR CODE" : "Scan QR Code"}
            </Text>
            <QrCode showBrandMark size={density === "tv" ? 150 : 180} value={state.verificationUriComplete} />
          </Surface>
          <Surface
            alignSelf="stretch"
            backgroundColor="borderDecorative"
            borderWidth={0}
            height={density === "tv" ? "auto" : 1}
            minHeight={density === "tv" ? 210 : 1}
            width={density === "tv" ? 1 : "100%"}
          />
          <Surface
            alignItems="center"
            backgroundColor="transparent"
            borderWidth={0}
            flex={density === "tv" ? 1 : undefined}
            gap="control"
            justifyContent="flex-start"
          >
            <Text density={density} textRole={density === "tv" ? "section" : "title"}>
              {density === "tv" ? "VISIT WEBSITE" : "Visit Website"}
            </Text>
            <Text
              density={density}
              numberOfLines={1}
              textRole={density === "tv" ? "metadata" : "time"}
              tone={density === "tv" ? "primary" : undefined}
              userSelect="text"
            >
              {state.verificationUri}
            </Text>
            <Surface
              alignItems="center"
              backgroundColor="transparent"
              borderWidth={0}
              gap="inline"
              padding={density === "tv" ? 0 : "control"}
              width="100%"
            >
              {density === "tv" ? null : (
                <Text density={density} textRole="metadata">
                  PAIRING CODE
                </Text>
              )}
              <Text
                accessibilityLabel={`Pairing code ${state.userCode}`}
                density={density}
                textRole={density === "tv" ? "code" : "channelNumber"}
                // A display numeral, the brand moment that keeps amber (#1909 Q4).
                tone="signal"
                userSelect="text"
              >
                {state.userCode}
              </Text>
              {state.refreshedFromExpired ? (
                <Text accessibilityLiveRegion="polite" density={density} textRole="metadata" tone="muted">
                  New code — the last one expired.
                </Text>
              ) : null}
            </Surface>
          </Surface>
        </Surface>
        <Surface
          backgroundColor="borderDecorative"
          borderWidth={0}
          height={1}
          marginVertical={density === "tv" ? 20 : 0}
          width="100%"
        />
        <Surface
          alignItems="center"
          backgroundColor="transparent"
          borderWidth={0}
          gap={density === "tv" ? 12 : "control"}
        >
          {urgent ? (
            <Badge accessibilityLiveRegion="polite" density={density} tone="danger">
              {countdownLabel}
            </Badge>
          ) : (
            <Text accessibilityLiveRegion="polite" density={density} textRole="metadata">
              {countdownLabel}
            </Text>
          )}
          {/* "Get a new code" replaces the code being typed on another device, so one stray click
              must not land on it: focus starts on the harmless "Use a different server" (#10). */}
          <Action density={density} onPress={() => void session.pair(state.serverUrl)} tone="secondary">
            Get a new code
          </Action>
          <Action
            density={density}
            hasTVPreferredFocus={density === "tv"}
            onPress={chooseAnotherServer}
            tone="secondary"
          >
            Use a different server
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
        <Action density={density} onPress={chooseAnotherServer} tone="secondary">
          Choose another server
        </Action>
      </>
    );
  return null;
};

export { PairingShell, pairingBackAvailable };
