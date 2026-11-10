import {
  brandChroma,
  type Density,
  decadeFillColor,
  playerChrome,
  Text,
  useFocusLook,
  useReducedMotionPreference,
} from "@loomarr/design-system";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { Animated, Image, PixelRatio, Pressable, View } from "react-native";

import { logoSize, type PlayerArt, rungFor } from "../art";
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
  /**
   * The phone held upright (§9.7): a narrower channel block and tighter padding, the title on up to two
   * lines, the progress line on a row of its own, and the next time never wrapping. The portrait
   * chrome sets it; the desktop and landscape band are unchanged.
   */
  portrait?: boolean;
}

/** The channel block's width, in px: the phone's, the portrait phone's, the desktop's. */
const BLOCK_WIDTH = { compact: 72, portrait: 56, wide: 144 } as const;

// The warm flip's sweep (§9.4): the seven bars run once along the band's edge, in 340 ms, and nowhere
// else. A cover the colour of the plate uncovers them left to right; no transform, no full-screen bars.
const SWEEP_MS = 340;

/**
 * The seven brand bars as an edge: the band's top, and each sheet's top. With `sweepKey` the bars run
 * once along the edge whenever the key changes (a channel flip); the first draw and reduced motion
 * (a plain crossfade is the picture's own) leave them still.
 */
const BarsEdge = ({ height, sweepKey }: { height: number; sweepKey?: string }) => {
  const reduced = useReducedMotionPreference();
  const cover = useRef(new Animated.Value(0)).current;
  const first = useRef(true);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the key is the trigger
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    if (sweepKey === undefined || reduced) return;
    cover.setValue(1);
    Animated.timing(cover, { duration: SWEEP_MS, toValue: 0, useNativeDriver: false }).start();
  }, [sweepKey]);
  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={{ flexDirection: "row", height }}
    >
      {brandChroma.map((color) => (
        <View key={color} style={{ backgroundColor: color, flex: 1 }} />
      ))}
      {sweepKey === undefined ? null : (
        <Animated.View
          pointerEvents="none"
          style={{
            backgroundColor: playerChrome.surface,
            bottom: 0,
            position: "absolute",
            right: 0,
            top: 0,
            width: cover.interpolate({ inputRange: [0, 1], outputRange: ["0%", "100%"] }),
          }}
        />
      )}
    </View>
  );
};

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
  // The portrait block is 56px: three digits at the full size (about 50px) fill it edge to edge, so
  // a long number steps down to the guide's number size and keeps a margin.
  const numberRole = width === BLOCK_WIDTH.portrait && number.length > 2 ? "guideNumber" : "channelNumber";
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
      <Text density={density} shout textRole="label" tone={tone} tracking={0.2}>
        CH
      </Text>
      <Text density={density} testID="band-channel-number" textRole={numberRole} tone={tone}>
        {number}
      </Text>
    </View>
  );
};

/** The small NOW / NEXT chips: the one place for letter-spaced mono capitals. */
const Chip = ({
  children,
  density,
  solid,
  tight = false,
}: {
  children: string;
  density: Density;
  solid: boolean;
  /**
   * Portrait: less side padding. The touch NEXT chip is 67px at full padding and sat in the 56px
   * column against the next time; widening the column instead takes the room from the title.
   */
  tight?: boolean;
}) => (
  <View
    testID="band-chip"
    style={{
      alignSelf: "flex-start",
      backgroundColor: solid ? playerChrome.text : "transparent",
      borderColor: solid ? playerChrome.text : "rgba(255, 255, 255, 0.4)",
      borderRadius: 2,
      borderWidth: 1,
      paddingHorizontal: tight ? 4 : 8,
      paddingVertical: 4,
    }}
  >
    <Text density={density} shout textRole="label" tone={solid ? "inverse" : "secondary"} tracking={0.16}>
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

/**
 * The programme's title: its clear logo in place of the text, sized by area so a wide wordmark and a
 * square logo carry the same weight (§9.2), with the text as the logo's accessible name and as the
 * fallback when there is no logo or it will not load.
 */
const BandTitle = ({
  density,
  lines = 1,
  logo,
  title,
}: {
  density: Density;
  /** How many lines the text title may take before the ellipsis. */
  lines?: number;
  logo: PlayerArt | undefined;
  title: string;
}) => {
  const [failedSrc, setFailedSrc] = useState<string | undefined>(undefined);
  if (logo && failedSrc !== logo.src) {
    const size = logoSize(logo, density);
    return (
      <Image
        accessibilityLabel={title}
        accessibilityRole="image"
        onError={() => setFailedSrc(logo.src)}
        resizeMode="contain"
        source={{ uri: rungFor(logo, size.width, PixelRatio.get()) }}
        style={{ alignSelf: "flex-start", height: size.height, width: size.width }}
      />
    );
  }
  return (
    <Text
      density={density}
      numberOfLines={lines}
      strong
      testID="band-title"
      textRole={density === "touch" ? "title" : "display"}
      tone="primary"
    >
      {title}
    </Text>
  );
};

/** The programme's time slot: white on a faint track; behind live, a red tick marks live. */
const Progress = ({ liveProgress, progress }: { liveProgress?: number | undefined; progress: number }) => (
  <View
    testID="band-progress"
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

const Row = ({ children, chip, gap = 16 }: { children: ReactNode; chip: ReactNode; gap?: number }) => (
  <View style={{ alignItems: "center", flexDirection: "row", gap }}>
    <View style={{ width: 56 }}>{chip}</View>
    <View style={{ flex: 1, minWidth: 0 }}>{children}</View>
  </View>
);

const ChannelBand = ({
  band,
  density,
  formatTime,
  ident = false,
  live,
  onGoLive,
  portrait = false,
}: ChannelBandProps) => {
  const compact = density === "touch";
  const rowGap = portrait ? 12 : 16;
  const behind = live.mode !== "live";
  const hasSlot = band.startMs !== undefined && band.stopMs !== undefined;
  const station = ident && band.station ? ` · ${band.station}` : "";
  return (
    <View style={{ alignItems: "stretch", flexDirection: "row", maxWidth: 1000, minWidth: 0 }}>
      <ChannelBlock
        decade={band.decade}
        density={density}
        number={band.channelNumber}
        width={portrait ? BLOCK_WIDTH.portrait : compact ? BLOCK_WIDTH.compact : BLOCK_WIDTH.wide}
      />
      <View style={{ flex: 1, minWidth: 0 }}>
        <BarsEdge height={compact ? 8 : 10} sweepKey={band.channelNumber} />
        <View
          style={{
            backgroundColor: playerChrome.surface,
            borderBottomWidth: 1,
            borderColor: playerChrome.hairline,
            borderRightWidth: 1,
            flex: 1,
            gap: compact ? 8 : 16,
            padding: compact ? 16 : 24,
            paddingHorizontal: portrait ? 12 : compact ? 16 : 32,
          }}
        >
          {/* Portrait: the name takes the text column's whole width, Live and the clock under it. */}
          <View
            style={
              portrait
                ? { alignItems: "stretch", flexDirection: "column", gap: 8 }
                : { alignItems: "center", flexDirection: "row", gap: 16, justifyContent: "space-between" }
            }
          >
            <Text
              density={density}
              numberOfLines={1}
              style={{ flexShrink: 1 }}
              testID="band-channel-name"
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
                <Chip density={density} solid tight={portrait}>
                  NOW
                </Chip>
              }
              gap={rowGap}
            >
              <View style={{ gap: 4 }}>
                <BandTitle density={density} lines={portrait ? 2 : 1} logo={band.logo} title={band.title} />
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

          {ident || !hasSlot ? null : portrait ? (
            // Its own row, the text column's full width: the line, then the two clocks at its ends.
            <View style={{ gap: 8 }}>
              <View style={{ flexDirection: "row" }}>
                <Progress liveProgress={band.liveProgress} progress={band.progress ?? 0} />
              </View>
              <View style={{ flexDirection: "row", justifyContent: "space-between" }}>
                <Text density={density} textRole="time" tone="secondary">
                  {formatTime(band.startMs ?? 0)}
                </Text>
                <Text density={density} textRole="time" tone="secondary">
                  {formatTime(band.stopMs ?? 0)}
                </Text>
              </View>
            </View>
          ) : (
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
                <Chip density={density} solid={false} tight={portrait}>
                  NEXT
                </Chip>
              }
              gap={rowGap}
            >
              <View style={{ alignItems: "baseline", flexDirection: "row", gap: rowGap }}>
                {/* One unbroken token: the time keeps its width, the title takes the rest. */}
                <Text
                  density={density}
                  numberOfLines={1}
                  style={{ flexShrink: 0 }}
                  testID="band-next-time"
                  textRole="time"
                  tone="secondary"
                >
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
