import { Action, type Density, Icon, icons, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

import { webOnly } from "../glass";

// The full-picture states (§9 mock "error" and "off-air"): nothing to play, so the message takes the
// picture's place, with the way on — Retry for a stream that stopped — and the way out.

interface PlayerCoverProps {
  channelLabel: string;
  density: Density;
  kind: "error" | "off-air";
  /** The error's own words, when the player has them; the off-air sentence otherwise. */
  message?: string;
  onLeave: () => void;
  onRetry?: () => void;
  title: string;
}

const PlayerCover = ({ channelLabel, density, kind, message, onLeave, onRetry, title }: PlayerCoverProps) => (
  <View
    role={kind === "error" ? "alert" : "status"}
    style={{
      alignItems: "center",
      backgroundColor: kind === "error" ? "rgba(11, 12, 14, 0.8)" : playerChrome.discInk,
      bottom: 0,
      gap: 8,
      justifyContent: "center",
      left: 0,
      paddingHorizontal: 24,
      position: "absolute",
      right: 0,
      top: 0,
      zIndex: 35,
      ...webOnly(kind === "error" ? { backdropFilter: "blur(4px)" } : {}),
    }}
  >
    {kind === "off-air" ? (
      <Icon color={playerChrome.textMuted} decorative glyph={icons.channels} size="tv" />
    ) : null}
    <Text density={density} strong textAlign="center" textRole="reading" tone="primary">
      {title}
    </Text>
    {message ? (
      <Text density={density} textAlign="center" textRole="compact" tone="secondary">
        {message}
      </Text>
    ) : null}
    <Text density={density} textRole="guideMeta" tone="muted">
      {channelLabel}
    </Text>
    <View style={{ flexDirection: "row", gap: 8, marginTop: 8 }}>
      {onRetry ? (
        <Action density={density} onPress={onRetry}>
          Retry channel
        </Action>
      ) : null}
      <Action density={density} onPress={onLeave} variant="outline">
        Leave
      </Action>
    </View>
  </View>
);

export type { PlayerCoverProps };
export { PlayerCover };
