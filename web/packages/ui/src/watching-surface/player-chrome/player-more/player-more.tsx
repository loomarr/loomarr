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

import type { PlaybarSegment, PlaybarWindow } from "../player-chrome.type";
import { playerRow, rowRole } from "../player-menu";
import { PreviewPlaybar } from "../preview-playbar";

// More (§9.3, variant J): only what has no key of its own. On a phone, which has no film strip in its
// band, the schedule comes first — what is on and coming, the past seekable. Then the rows: an icon,
// the words, what it is set to, and a chevron when it opens something. A row the caller leaves out
// (no channel to return to, nothing to start over) is not drawn.

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
  /** Picture in picture, where the platform has it and no key shows it. */
  pictureInPicture?: { onPress: () => void };
  /** The schedule's blocks and window, on a phone; omitted where the band has the strip. */
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
  opens = false,
}: {
  density: Density;
  detail?: string;
  icon: IconName;
  label: string;
  onPress: () => void;
  /** It opens another page or panel (a chevron); otherwise it just does the thing. */
  opens?: boolean;
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
      style={playerRow(density, { focused, hovered })}
    >
      <Icon
        color={playerChrome.textSecondary}
        decorative
        glyph={icons[icon]}
        size={density === "pointer" ? "compact" : "default"}
      />
      <Text
        density={density}
        numberOfLines={1}
        style={{ flex: 1 }}
        textRole={rowRole(density)}
        tone="primary"
      >
        {label}
      </Text>
      {detail ? (
        <Text density={density} numberOfLines={1} textRole="cardMeta" tone="muted">
          {detail}
        </Text>
      ) : null}
      {opens ? (
        <Icon color={playerChrome.textMuted} decorative glyph={icons.disclosure} size="compact" />
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
  <View style={{ gap: 16 }}>
    {schedule ? (
      <View style={{ gap: 16, paddingHorizontal: 12, paddingTop: 4 }}>
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
    <View style={{ gap: 2 }}>
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
      {channels ? (
        <MoreRow density={density} icon="list" label="Channels" onPress={channels.onPress} opens />
      ) : null}
      {info ? <MoreRow density={density} icon="info" label="Info" onPress={info.onPress} opens /> : null}
      {picture ? (
        <MoreRow
          density={density}
          detail={picture.detail}
          icon="settings"
          label="Picture"
          onPress={picture.onPress}
          opens
        />
      ) : null}
      {playbackDetails ? (
        <MoreRow
          density={density}
          icon="activity"
          label="Playback details"
          onPress={playbackDetails.onPress}
          opens
        />
      ) : null}
      {pictureInPicture ? (
        <MoreRow
          density={density}
          icon="pictureInPicture"
          label="Picture in picture"
          onPress={pictureInPicture.onPress}
        />
      ) : null}
    </View>
  </View>
);

export type { PlayerMoreProps };
export { PlayerMore };
