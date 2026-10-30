import { type Density, playerChrome, semanticColors, Text } from "@loomarr/design-system";
import { View } from "react-native";

import { gradientOr, panelGlass, webOnly } from "../glass";
import { PlaybarStill } from "../playbar-still";
import type { PlaybarBreak, PlaybarProgramme } from "../player-chrome.type";

// The preview card (§9.3, pointer only), over the segment it describes. A programme: its still as
// the hero with the title set on it, when it starts, chips and two lines of synopsis. A break: every
// spot with its thumbnail, name, `brand · era` or kind, and duration, the one on air marked ON NOW.

const CARD_WIDTH = 352;

interface PreviewCardProps {
  density: Density;
  formatTime: (ms: number) => string;
  /** The live edge: what a programme's "IN 16 MIN" counts from. */
  nowMs: number;
  /** The spot on air, when the playhead is inside this break. */
  onNowSpot?: number;
  segment: PlaybarBreak | PlaybarProgramme;
}

const badgeOf = (segment: PlaybarProgramme, nowMs: number) => {
  if (segment.startMs > nowMs) return `IN ${Math.max(1, Math.round((segment.startMs - nowMs) / 60_000))} MIN`;
  return segment.stopMs > nowMs ? "ON NOW" : "EARLIER";
};

const ProgrammeCard = ({
  density,
  formatTime,
  nowMs,
  segment,
}: Omit<PreviewCardProps, "segment"> & { segment: PlaybarProgramme }) => (
  <View>
    <View style={{ aspectRatio: 16 / 9, width: "100%" }}>
      <PlaybarStill style={{ height: "100%", position: "absolute", width: "100%" }} uri={segment.stillUri} />
      <View
        style={{
          bottom: 0,
          left: 0,
          position: "absolute",
          right: 0,
          top: 0,
          ...gradientOr(
            `linear-gradient(0deg, ${playerChrome.panelBottom} 0%, rgba(11, 12, 14, 0.25) 55%, transparent 100%)`,
            semanticColors.artwork.scrim,
          ),
        }}
      />
      <View
        style={{
          backgroundColor: playerChrome.signal,
          borderRadius: 999,
          left: 10,
          paddingHorizontal: 8,
          paddingVertical: 2,
          position: "absolute",
          top: 10,
        }}
      >
        <Text density={density} textRole="cardChannelNumber" tone="inverse">
          {badgeOf(segment, nowMs)}
        </Text>
      </View>
      <View style={{ bottom: 10, left: 14, position: "absolute", right: 14 }}>
        <Text density={density} textRole="cardTime" tone="secondary">
          {`${formatTime(segment.startMs)} – ${formatTime(segment.stopMs)}`}
        </Text>
        <Text density={density} numberOfLines={2} strong textRole="headline" tone="primary">
          {segment.name}
        </Text>
      </View>
    </View>
    <View style={{ gap: 10, paddingBottom: 14, paddingHorizontal: 14, paddingTop: 8 }}>
      {segment.chips.length > 0 ? (
        <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6 }}>
          {segment.chips.map((chip) => (
            <View
              key={chip}
              style={{
                backgroundColor: playerChrome.glassBottom,
                borderColor: playerChrome.glassRing,
                borderRadius: 6,
                borderWidth: 1,
                paddingHorizontal: 6,
                paddingVertical: 1,
              }}
            >
              <Text density={density} textRole="cardMeta" tone="secondary">
                {chip}
              </Text>
            </View>
          ))}
        </View>
      ) : null}
      {segment.synopsis ? (
        <Text density={density} numberOfLines={2} textRole="cardMeta" tone="secondary">
          {segment.synopsis}
        </Text>
      ) : null}
    </View>
  </View>
);

const BreakCard = ({
  density,
  formatTime,
  onNowSpot,
  segment,
}: Omit<PreviewCardProps, "segment" | "nowMs"> & { segment: PlaybarBreak }) => (
  <View>
    <View
      style={{
        borderBottomColor: playerChrome.glassRing,
        borderBottomWidth: 1,
        paddingHorizontal: 14,
        paddingVertical: 10,
        ...webOnly({ backgroundImage: `linear-gradient(90deg, ${playerChrome.breakTint}, transparent)` }),
      }}
    >
      <Text density={density} strong textRole="cardLabel" tone="info">
        {`Break · ${formatTime(segment.startMs)} · ${Math.max(1, Math.round((segment.stopMs - segment.startMs) / 60_000))} min`}
      </Text>
    </View>
    <View style={{ gap: 4, padding: 8 }}>
      {segment.spots.map((spot, i) => (
        <View
          key={spot.id}
          style={{
            alignItems: "center",
            backgroundColor: i === onNowSpot ? playerChrome.breakTint : "transparent",
            borderColor: i === onNowSpot ? playerChrome.breakRing : "transparent",
            borderRadius: 8,
            borderWidth: 1,
            flexDirection: "row",
            gap: 12,
            padding: 6,
          }}
        >
          <PlaybarStill style={{ aspectRatio: 16 / 9, borderRadius: 6, width: 80 }} uri={spot.stillUri} />
          <View style={{ flex: 1, minWidth: 0 }}>
            <Text density={density} numberOfLines={1} textRole="cardMeta" tone="primary">
              {spot.name}
            </Text>
            {spot.detail ? (
              <Text density={density} numberOfLines={1} textRole="guideMeta" tone="secondary">
                {spot.detail}
              </Text>
            ) : null}
          </View>
          <Text density={density} textRole="guideMeta" tone={i === onNowSpot ? "info" : "secondary"}>
            {i === onNowSpot ? "ON NOW" : spot.durationLabel}
          </Text>
        </View>
      ))}
    </View>
  </View>
);

const PreviewCard = (props: PreviewCardProps) => (
  <View
    style={{
      borderRadius: 12,
      overflow: "hidden",
      width: CARD_WIDTH,
      ...panelGlass(playerChrome.shadowCard),
    }}
  >
    {props.segment.kind === "break" ? (
      <BreakCard {...props} segment={props.segment} />
    ) : (
      <ProgrammeCard {...props} segment={props.segment} />
    )}
  </View>
);

export type { PreviewCardProps };
export { CARD_WIDTH, PreviewCard };
