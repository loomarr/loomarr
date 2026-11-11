import {
  type Density,
  Icon,
  type IconName,
  icons,
  playerChrome,
  Text,
  useFocusLook,
} from "@loomarr/design-system";
import { useState } from "react";
import { Pressable, View } from "react-native";

import { nativeFocus, webOnly } from "../flat";
import type { PlaybarSegment, PlaybarWindow } from "../player-chrome.type";
import { PreviewPlaybar } from "../preview-playbar";

// The More sheet's body (§9.3): the schedule — what is on and coming, the past seekable — then
// everything the screen leaves out. Rows are quiet text actions; a row the caller leaves out (no
// channel to return to, nothing to start over) is not drawn.

interface PlayerMoreProps {
  channels?: { onPress: () => void };
  density: Density;
  formatTime: (ms: number) => string;
  /** Go live, behind live. */
  goLive?: { detail: string; onPress: () => void };
  info?: { onPress: () => void };
  /** Picture: the quality on air ("Auto · 1080p"). */
  picture?: { detail: string; onPress: () => void };
  /** Playback details: the technical panel (§9.5). */
  playbackDetails?: { onPress: () => void };
  /** Picture in picture, where the browser offers it. */
  pictureInPicture?: { onPress: () => void };
  /** The schedule's blocks and window; omitted while it hasn't loaded. */
  schedule?: {
    onSeek?: (wallClockMs: number) => void;
    playheadMs: number;
    segments: readonly PlaybarSegment[];
    window: PlaybarWindow;
  };
  /** Back to the programme's start, while it began inside the window. */
  startOver?: { detail: string; onPress: () => void };
}

const MoreRow = ({
  detail,
  density,
  icon,
  label,
  onPress,
}: {
  density: Density;
  detail?: string;
  icon: IconName;
  label: string;
  onPress: () => void;
}) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      // A row with a detail is named by what it shows ("Start over 8:00 PM"): a name that leaves
      // the detail out does not match the visible label (WCAG 2.5.3).
      accessibilityLabel={detail ? undefined : label}
      accessibilityRole="button"
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: hovered ? playerChrome.surfaceRaised : "transparent",
        borderRadius: 6,
        flexDirection: "row",
        gap: 16,
        minHeight: 56,
        paddingHorizontal: 16,
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease",
        }),
        ...nativeFocus(focused),
      }}
    >
      <Icon color={playerChrome.text} decorative glyph={icons[icon]} size="default" />
      <Text density={density} numberOfLines={1} style={{ flex: 1 }} textRole="body" tone="primary">
        {label}
      </Text>
      {detail ? (
        <Text density={density} numberOfLines={1} textRole="compact" tone="muted">
          {detail}
        </Text>
      ) : null}
    </Pressable>
  );
};

const PlayerMore = ({
  channels,
  density,
  formatTime,
  goLive,
  info,
  picture,
  pictureInPicture,
  playbackDetails,
  schedule,
  startOver,
}: PlayerMoreProps) => (
  <View style={{ gap: 24 }}>
    {schedule ? (
      <View style={{ gap: 16 }}>
        <Text density={density} textRole="label" tone="secondary">
          Schedule
        </Text>
        <PreviewPlaybar
          density="touch"
          formatTime={formatTime}
          onSeek={schedule.onSeek}
          playheadMs={schedule.playheadMs}
          pointer={false}
          segments={schedule.segments}
          window={schedule.window}
        />
      </View>
    ) : null}
    <View>
      {startOver ? (
        <MoreRow
          density={density}
          detail={startOver.detail}
          icon="startOver"
          label="Start over"
          onPress={startOver.onPress}
        />
      ) : null}
      {goLive ? (
        <MoreRow
          density={density}
          detail={goLive.detail}
          icon="play"
          label="Go live"
          onPress={goLive.onPress}
        />
      ) : null}
      {picture ? (
        <MoreRow
          density={density}
          detail={picture.detail}
          icon="settings"
          label="Picture"
          onPress={picture.onPress}
        />
      ) : null}
      {playbackDetails ? (
        <MoreRow density={density} icon="info" label="Playback details" onPress={playbackDetails.onPress} />
      ) : null}
      {pictureInPicture ? (
        <MoreRow
          density={density}
          icon="external"
          label="Picture in picture"
          onPress={pictureInPicture.onPress}
        />
      ) : null}
      {channels ? (
        <MoreRow density={density} icon="list" label="Channels" onPress={channels.onPress} />
      ) : null}
      {info ? <MoreRow density={density} icon="info" label="Info" onPress={info.onPress} /> : null}
    </View>
  </View>
);

export type { PlayerMoreProps };
export { PlayerMore };
