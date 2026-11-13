import { Action, BrandMark, type Density, playerChrome, Text } from "@loomarr/design-system";
import { View } from "react-native";

// The full-picture states (§9 mock "error" and "off-air"): nothing to play, so the message takes the
// picture's place, with the way on — Retry for a stream that stopped — and the way out.

interface PlayerCoverProps {
  /** `CH 7 · Name`; omitted when no channel is tuned (the channels themselves did not load). */
  channelLabel?: string;
  density: Density;
  kind: "error" | "off-air";
  /** The error's own words, when the player has them; the off-air sentence otherwise. */
  message?: string;
  /** The way out; omitted on a television, where Back leaves. */
  onLeave?: () => void;
  /** The way out's words: "Leave", or where it goes ("Change server"). */
  leaveLabel?: string;
  onRetry?: () => void;
  /** Retry's words: "Retry channel", or "Try again" when the channels themselves did not load. */
  retryLabel?: string;
  title: string;
}

const PlayerCover = ({
  channelLabel,
  density,
  kind,
  leaveLabel = "Leave",
  message,
  onLeave,
  onRetry,
  retryLabel = "Retry channel",
  title,
}: PlayerCoverProps) => (
  <View
    role={kind === "error" ? "alert" : "status"}
    style={{
      alignItems: "center",
      backgroundColor: kind === "error" ? playerChrome.surface : playerChrome.discInk,
      bottom: 0,
      gap: 8,
      justifyContent: "center",
      left: 0,
      paddingHorizontal: 24,
      position: "absolute",
      right: 0,
      top: 0,
      zIndex: 35,
    }}
  >
    <BrandMark contained={false} decorative size={4} />
    <Text density={density} strong textAlign="center" textRole="reading" tone="primary">
      {title}
    </Text>
    {message ? (
      <Text density={density} textAlign="center" textRole="compact" tone="secondary">
        {message}
      </Text>
    ) : null}
    {channelLabel ? (
      <Text density={density} textRole="mono" tone="muted">
        {channelLabel}
      </Text>
    ) : null}
    <View style={{ flexDirection: "row", gap: 8, marginTop: 8 }}>
      {onRetry ? (
        // A remote's focus has nowhere else to be on a cover, so Retry claims it and OK retries. It is
        // white, as the play key is: inside the player amber is focus only (§9.6).
        <Action density={density} hasTVPreferredFocus={density === "tv"} onPress={onRetry} variant="disc">
          {retryLabel}
        </Action>
      ) : null}
      {onLeave ? (
        <Action density={density} onPress={onLeave} variant="outline">
          {leaveLabel}
        </Action>
      ) : null}
    </View>
  </View>
);

export type { PlayerCoverProps };
export { PlayerCover };
