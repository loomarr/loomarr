import type { Density } from "@loomarr/design-system";
import { Action, ActivityIndicator, Surface, Text } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import type { ReactNode } from "react";
import { View } from "react-native";

import { ChannelIdentity } from "../identity";

interface WatchingSurfaceProps {
  density: Density;
  loadError?: string;
  onChannelDown: () => void;
  onChannelUp: () => void;
  onOpenGuide: () => void;
  onOpenSurf: () => void;
  onPrevious: () => void;
  onRetry: () => void;
  player: ReactNode;
  snapshot: PlayerSnapshot;
}

const WatchingSurface = ({
  density,
  loadError,
  onChannelDown,
  onChannelUp,
  onOpenGuide,
  onOpenSurf,
  onPrevious,
  onRetry,
  player,
  snapshot,
}: WatchingSurfaceProps) => {
  const message =
    loadError ??
    (snapshot.status === "empty"
      ? "No playable channels are on this Loomarr yet."
      : snapshot.status === "failed"
        ? snapshot.error
        : undefined);
  return (
    <View style={{ backgroundColor: "#000", flex: 1 }}>
      <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>{player}</View>
      {!snapshot.channel && !message ? (
        <View style={{ alignItems: "center", flex: 1, justifyContent: "center" }}>
          <ActivityIndicator
            accessibilityLabel="Loading channels"
            size={density === "tv" ? "tv" : density === "touch" ? "touch" : "default"}
          />
        </View>
      ) : null}
      <Surface
        backgroundColor="$surfaceOverlay"
        borderBottomLeftRadius={0}
        borderBottomRightRadius={0}
        bottom={0}
        gap="$control"
        left={0}
        padding="$section"
        position="absolute"
        right={0}
      >
        {snapshot.channel ? (
          <ChannelIdentity
            channel={{
              channelLogoState: "missing",
              channelName: snapshot.channel.name,
              channelNumber: String(snapshot.channel.number),
            }}
            density={density}
          />
        ) : null}
        {snapshot.status === "tuning" ? (
          <Text accessibilityLiveRegion="polite" density={density} textRole="metadata">
            Tuning…
          </Text>
        ) : null}
        {message ? (
          <Text accessibilityLiveRegion="polite" density={density} textRole="body" tone="danger">
            {message}
          </Text>
        ) : null}
        <View style={{ flexDirection: "row", gap: density === "tv" ? 16 : 8 }}>
          <Action density={density} icon="previous" onPress={onPrevious} tone="secondary">
            Previous
          </Action>
          <Action density={density} onPress={onChannelDown} tone="secondary">
            Channel −
          </Action>
          <Action density={density} icon="guide" onPress={onOpenGuide} tone="secondary">
            Guide
          </Action>
          <Action density={density} icon="channels" onPress={onOpenSurf} tone="secondary">
            Surf
          </Action>
          <Action density={density} onPress={onChannelUp} tone="secondary">
            Channel +
          </Action>
          {message ? (
            <Action density={density} onPress={onRetry} tone="primary">
              Retry
            </Action>
          ) : null}
        </View>
      </Surface>
    </View>
  );
};

export type { WatchingSurfaceProps };
export { WatchingSurface };
