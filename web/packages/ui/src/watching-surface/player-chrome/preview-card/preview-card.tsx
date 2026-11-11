import { type Density, playerChrome, Text } from "@loomarr/design-system";
import { useState } from "react";
import { Image, PixelRatio, View } from "react-native";

import { logoSize, rungFor } from "../art";
import { PlaybarStill } from "../playbar-still";
import type { PlaybarBreak, PlaybarProgramme } from "../player-chrome.type";

// The preview card (§9.2, variant J): what pointing at a block of the film strip (or moving a
// remote's focus to it) opens, 12px clear of the band. A programme: its fanart as the hero, with the
// title's logo set on it (sized by area, over a flat ink wash) when there is one; then its name, when
// it starts or until when it runs, its facts and two lines of synopsis. A break: every spot with its
// thumbnail, name, `brand · era` or kind, and duration, the one on air marked.

const CARD_WIDTH = 320;

interface PreviewCardProps {
  density: Density;
  formatTime: (ms: number) => string;
  /** The whole synopsis (Info, the on-air programme's card opened in full). */
  full?: boolean;
  /** The live edge: on now reads `Until`, later reads `Starts`. */
  nowMs: number;
  /** The spot on air, when the playhead is inside this break. */
  onNowSpot?: number | undefined;
  segment: PlaybarBreak | PlaybarProgramme;
}

/** The hero: the fanart, and the title's logo on it under a flat ink wash. */
const Hero = ({ density, segment }: { density: Density; segment: PlaybarProgramme }) => {
  const [failed, setFailed] = useState<string | undefined>(undefined);
  const logo = segment.logo && failed !== segment.logo.src ? segment.logo : undefined;
  const size = logo ? logoSize(logo, density) : undefined;
  return (
    <View style={{ height: 148, justifyContent: "flex-end", padding: 14 }}>
      <PlaybarStill
        style={{ height: "100%", position: "absolute", width: "100%" }}
        {...(segment.stillUri ? { uri: segment.stillUri } : {})}
      />
      {logo && size ? (
        <>
          <View
            style={{
              backgroundColor: "rgba(11, 12, 14, 0.42)",
              bottom: 0,
              left: 0,
              position: "absolute",
              right: 0,
              top: 0,
            }}
          />
          <Image
            accessibilityLabel={segment.name}
            onError={() => setFailed(logo.src)}
            resizeMode="contain"
            source={{ uri: rungFor(logo, size.width, PixelRatio.get()) }}
            style={{
              height: Math.min(64, size.height),
              width: (Math.min(64, size.height) * size.width) / size.height,
            }}
          />
        </>
      ) : null}
    </View>
  );
};

const ProgrammeCard = ({
  density,
  formatTime,
  full = false,
  nowMs,
  segment,
}: Omit<PreviewCardProps, "segment"> & { segment: PlaybarProgramme }) => {
  const onNow = segment.startMs <= nowMs && nowMs < segment.stopMs;
  return (
    <View>
      <Hero density={density} segment={segment} />
      <View style={{ gap: 6, paddingBottom: 14, paddingHorizontal: 14, paddingTop: 12 }}>
        <View
          style={{ alignItems: "baseline", flexDirection: "row", gap: 8, justifyContent: "space-between" }}
        >
          <Text
            density={density}
            numberOfLines={1}
            strong
            style={{ flexShrink: 1, minWidth: 0 }}
            textRole="cardTitle"
            tone="primary"
          >
            {segment.name}
          </Text>
          <Text density={density} style={{ flexShrink: 0 }} textRole="compact" tone="secondary">
            {onNow ? `Until ${formatTime(segment.stopMs)}` : `Starts ${formatTime(segment.startMs)}`}
          </Text>
        </View>
        {segment.chips.length > 0 ? (
          <Text density={density} numberOfLines={1} textRole="label" tone="secondary">
            {segment.chips.join(" · ")}
          </Text>
        ) : null}
        {segment.synopsis ? (
          <Text density={density} {...(full ? {} : { numberOfLines: 2 })} textRole="label" tone="secondary">
            {segment.synopsis}
          </Text>
        ) : null}
      </View>
    </View>
  );
};

const BreakCard = ({
  density,
  formatTime,
  onNowSpot,
  segment,
}: Omit<PreviewCardProps, "segment" | "nowMs"> & { segment: PlaybarBreak }) => (
  <View>
    <View
      style={{
        backgroundColor: playerChrome.breakTint,
        borderBottomColor: playerChrome.hairline,
        borderBottomWidth: 1,
        paddingHorizontal: 14,
        paddingVertical: 10,
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
          <PlaybarStill
            style={{ aspectRatio: 16 / 9, borderRadius: 6, width: 80 }}
            {...(spot.stillUri ? { uri: spot.stillUri } : {})}
          />
          <View style={{ flex: 1, minWidth: 0 }}>
            <Text density={density} numberOfLines={1} textRole="cardMeta" tone="primary">
              {spot.name}
            </Text>
            {spot.detail ? (
              <Text density={density} numberOfLines={1} textRole="cardMeta" tone="secondary">
                {spot.detail}
              </Text>
            ) : null}
          </View>
          <Text density={density} textRole="cardMeta" tone={i === onNowSpot ? "info" : "secondary"}>
            {i === onNowSpot ? "On now" : spot.durationLabel}
          </Text>
        </View>
      ))}
    </View>
  </View>
);

const PreviewCard = (props: PreviewCardProps) => (
  <View
    style={{
      backgroundColor: playerChrome.band,
      borderColor: playerChrome.hairline,
      borderRadius: 8,
      borderWidth: 1,
      overflow: "hidden",
      width: CARD_WIDTH,
    }}
    testID="preview-card"
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
