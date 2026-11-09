import {
  brandChroma,
  type Density,
  decadeFillColor,
  playerChrome,
  Text,
  useFocusLook,
} from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { Pressable, View } from "react-native";

import { webOnly } from "../flat";
import type { PlayerBandData, PlayerLiveState } from "../player-chrome.type";

// The band (§9.2, layer 2): identity. A full-height channel block — CH over the number — beside a
// broadcast band under a seven-bar edge: the channel's name, Live and the clock; NOW with the
// programme's title and episode (in a break, the title and `Back in n min`); one progress line over
// the programme's own time slot; NEXT as text. The channel is named once, here. The same band is the
// station-ID card: the block, the station's name with its era and place, and the programme on air.

interface ChannelBandProps {
  band: PlayerBandData;
  density: Density;
  formatTime: (ms: number) => string;
  /** Drawn as the station-ID card. */
  ident?: boolean;
  live: PlayerLiveState;
  onGoLive: () => void;
}

/** The seven brand bars as an edge: the band's top, and each sheet's top. */
const BarsEdge = ({ height }: { height: number }) => (
  <View
    accessibilityElementsHidden
    importantForAccessibility="no-hide-descendants"
    style={{ flexDirection: "row", height }}
  >
    {brandChroma.map((color) => (
      <View key={color} style={{ backgroundColor: color, flex: 1 }} />
    ))}
  </View>
);

/**
 * The channel block: `CH` over the number, in the channel's decade colour (amber without one). It is
 * the only place a channel's own colour is wider than a hairline.
 */
const ChannelBlock = ({
  decade,
  density,
  number,
  width,
}: {
  decade?: number | undefined;
  density: Density;
  number: string;
  width: number;
}) => {
  const fill = decadeFillColor(decade);
  const tone = fill ? "onDecade" : "inverse";
  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={{
        alignItems: "center",
        backgroundColor: fill ?? playerChrome.signal,
        justifyContent: "center",
        width,
      }}
    >
      <Text density={density} shout textRole="guideIdent" tone={tone} tracking={0.2}>
        CH
      </Text>
      <Text density={density} textRole="channelNumber" tone={tone}>
        {number}
      </Text>
    </View>
  );
};

/** The small NOW / NEXT chips: the one place for letter-spaced mono capitals. */
const Chip = ({ children, density, solid }: { children: string; density: Density; solid: boolean }) => (
  <View
    style={{
      alignSelf: "flex-start",
      backgroundColor: solid ? playerChrome.text : "transparent",
      borderColor: solid ? playerChrome.text : "rgba(255, 255, 255, 0.4)",
      borderRadius: 2,
      borderWidth: 1,
      paddingHorizontal: 8,
      paddingVertical: 4,
    }}
  >
    <Text
      density={density}
      shout
      textRole="guideIdent"
      tone={solid ? "inverse" : "secondary"}
      tracking={0.16}
    >
      {children}
    </Text>
  </View>
);

/** Live stays red: a dot and the word. */
const LiveMark = ({ density }: { density: Density }) => (
  <View style={{ alignItems: "center", flexDirection: "row", gap: 8 }}>
    <View style={{ backgroundColor: playerChrome.live, borderRadius: 4, height: 8, width: 8 }} />
    <Text density={density} strong textRole="label" tone="live">
      Live
    </Text>
  </View>
);

/** Go live, behind live: a secondary-outline action in the live colour (#2777). */
const GoLive = ({ density, onPress }: { density: Density; onPress: () => void }) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <Pressable
      accessibilityRole="button"
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onPress}
      style={{
        alignItems: "center",
        backgroundColor: hovered ? playerChrome.surfaceHover : "transparent",
        borderColor: playerChrome.live,
        borderRadius: 4,
        borderWidth: 1,
        flexDirection: "row",
        gap: 8,
        height: density === "touch" ? 40 : 32,
        paddingHorizontal: 12,
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease",
        }),
      }}
    >
      <View style={{ backgroundColor: playerChrome.live, borderRadius: 4, height: 8, width: 8 }} />
      <Text density={density} strong textRole="label" tone="primary">
        Go live
      </Text>
    </Pressable>
  );
};

/** The programme's time slot: white on a faint track; behind live, a red tick marks live. */
const Progress = ({ liveProgress, progress }: { liveProgress?: number | undefined; progress: number }) => (
  <View
    style={{
      backgroundColor: playerChrome.track,
      borderRadius: 2,
      flex: 1,
      height: 4,
      justifyContent: "center",
    }}
  >
    {liveProgress === undefined ? null : (
      <View
        style={{
          backgroundColor: "rgba(255, 255, 255, 0.32)",
          borderRadius: 2,
          height: 4,
          position: "absolute",
          width: `${liveProgress * 100}%`,
        }}
      />
    )}
    <View
      style={{
        backgroundColor: playerChrome.text,
        borderRadius: 2,
        height: 4,
        position: "absolute",
        width: `${progress * 100}%`,
      }}
    />
    {liveProgress === undefined ? null : (
      <View
        style={{
          backgroundColor: playerChrome.live,
          height: 12,
          left: `${liveProgress * 100}%`,
          position: "absolute",
          width: 2,
        }}
      />
    )}
  </View>
);

const Row = ({ children, chip }: { children: ReactNode; chip: ReactNode }) => (
  <View style={{ alignItems: "center", flexDirection: "row", gap: 16 }}>
    <View style={{ width: 56 }}>{chip}</View>
    <View style={{ flex: 1, minWidth: 0 }}>{children}</View>
  </View>
);

const ChannelBand = ({ band, density, formatTime, ident = false, live, onGoLive }: ChannelBandProps) => {
  const compact = density === "touch";
  const behind = live.mode !== "live";
  const hasSlot = band.startMs !== undefined && band.stopMs !== undefined;
  const station = ident && band.station ? ` · ${band.station}` : "";
  return (
    <View style={{ alignItems: "stretch", flexDirection: "row", maxWidth: 1000, minWidth: 0 }}>
      <ChannelBlock
        decade={band.decade}
        density={density}
        number={band.channelNumber}
        width={compact ? 72 : 144}
      />
      <View style={{ flex: 1, minWidth: 0 }}>
        <BarsEdge height={compact ? 8 : 10} />
        <View
          style={{
            backgroundColor: playerChrome.surface,
            borderBottomWidth: 1,
            borderColor: playerChrome.hairline,
            borderRightWidth: 1,
            flex: 1,
            gap: compact ? 8 : 16,
            padding: compact ? 16 : 24,
            paddingHorizontal: compact ? 16 : 32,
          }}
        >
          <View
            style={{ alignItems: "center", flexDirection: "row", gap: 16, justifyContent: "space-between" }}
          >
            <Text
              density={density}
              numberOfLines={1}
              style={{ flexShrink: 1 }}
              textRole="label"
              tone="secondary"
            >
              {`${band.channelName}${station}`}
            </Text>
            {ident ? null : (
              <View style={{ alignItems: "center", flexDirection: "row", flexShrink: 0, gap: 16 }}>
                {behind ? <GoLive density={density} onPress={onGoLive} /> : <LiveMark density={density} />}
                {band.clockMs === undefined ? null : (
                  <Text density={density} textRole="time" tone="primary">
                    {formatTime(band.clockMs)}
                  </Text>
                )}
              </View>
            )}
          </View>

          {band.title ? (
            <Row
              chip={
                <Chip density={density} solid>
                  NOW
                </Chip>
              }
            >
              <View style={{ gap: 4 }}>
                <Text
                  density={density}
                  numberOfLines={1}
                  strong
                  textRole={compact ? "title" : "display"}
                  tone="primary"
                >
                  {band.title}
                </Text>
                {band.inBreak && band.breakMinutesLeft !== undefined ? (
                  <Text density={density} numberOfLines={1} strong textRole="compact" tone="info">
                    {`Back in ${band.breakMinutesLeft} min`}
                  </Text>
                ) : band.line ? (
                  <Text density={density} numberOfLines={1} textRole="compact" tone="secondary">
                    {band.line}
                  </Text>
                ) : null}
              </View>
            </Row>
          ) : null}

          {ident || !hasSlot ? null : (
            <Row chip={null}>
              <View style={{ alignItems: "center", flexDirection: "row", gap: 16 }}>
                <Text density={density} textRole="time" tone="secondary">
                  {formatTime(band.startMs ?? 0)}
                </Text>
                <Progress liveProgress={band.liveProgress} progress={band.progress ?? 0} />
                <Text density={density} textRole="time" tone="secondary">
                  {formatTime(band.stopMs ?? 0)}
                </Text>
                {band.behindLabel && !compact ? (
                  <Text density={density} textRole="time" tone="muted">
                    {band.behindLabel}
                  </Text>
                ) : null}
              </View>
            </Row>
          )}

          {ident || !band.next ? null : (
            <Row
              chip={
                <Chip density={density} solid={false}>
                  NEXT
                </Chip>
              }
            >
              <View style={{ alignItems: "baseline", flexDirection: "row", gap: 16 }}>
                <Text density={density} textRole="time" tone="secondary">
                  {formatTime(band.next.startMs)}
                </Text>
                <Text
                  density={density}
                  numberOfLines={1}
                  style={{ flexShrink: 1 }}
                  textRole="body"
                  tone="primary"
                >
                  {band.next.name}
                </Text>
              </View>
            </Row>
          )}
        </View>
      </View>
    </View>
  );
};

export type { ChannelBandProps };
export { BarsEdge, ChannelBand, ChannelBlock };
