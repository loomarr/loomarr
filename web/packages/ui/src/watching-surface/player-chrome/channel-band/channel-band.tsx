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
import { Animated, type LayoutChangeEvent, Pressable, View } from "react-native";

import { nativeFocus, webOnly } from "../flat";
import type { PlayerBandData, PlayerLiveState, PlayerSchedule } from "../player-chrome.type";
import { PreviewPlaybar } from "../preview-playbar";

// The band (§9.2, variant J, layer 2): identity and what is on. The channel block (CH over the
// number, in the channel's decade colour) beside the band: the channel's name, Live and the clock;
// Now with the typed title and the episode line. On desktop and television the film strip is the
// band's foot, the full width of the band, under the block too; on a phone a thin progress line and
// Next. The channel is named once, here. The same band is the station-ID card.

interface ChannelBandProps {
  band: PlayerBandData;
  density: Density;
  formatTime: (ms: number) => string;
  /** Drawn as the station-ID card. */
  ident?: boolean;
  live: PlayerLiveState;
  onGoLive: () => void;
  /** The phone held upright (§9.7): the title may take two lines. */
  portrait?: boolean;
  /**
   * The film strip, the band's foot on desktop and television (§9.2). Without it the band draws the
   * progress line and Next, as a phone's does.
   */
  schedule?: PlayerSchedule | undefined;
  /** Television: the strip's blocks take remote focus, and focus opens a block's card. */
  tv?: boolean;
}

/** The channel block's width, in px (§9.2): a phone's, then desktop's and a television's. */
const BLOCK_WIDTH = { touch: 56, wide: 96 } as const;

/** The band's widest, beside the control cluster (§9.2). */
const BAND_MAX_WIDTH = 960;

// The warm flip (§9.4): the seven bars run once along the band's top edge, about 340 ms, then go.
const SWEEP_MS = 340;

/**
 * The seven brand bars as an edge: a sheet's top, and the station-ID card's. With `sweepKey` they are
 * the warm flip's edge instead: drawn only while they run once along the band's top whenever the key
 * changes (a channel flip). The first draw and reduced motion leave nothing.
 */
const BarsEdge = ({ height, sweepKey }: { height: number; sweepKey?: string }) => {
  const reduced = useReducedMotionPreference();
  const wipe = useRef(new Animated.Value(sweepKey === undefined ? 1 : 0)).current;
  const fade = useRef(new Animated.Value(sweepKey === undefined ? 1 : 0)).current;
  const first = useRef(true);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the key is the trigger
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    if (sweepKey === undefined || reduced) return;
    wipe.setValue(0);
    fade.setValue(1);
    Animated.sequence([
      Animated.timing(wipe, { duration: SWEEP_MS * 0.55, toValue: 1, useNativeDriver: false }),
      Animated.timing(fade, { duration: SWEEP_MS * 0.45, toValue: 0, useNativeDriver: false }),
    ]).start();
  }, [sweepKey]);
  return (
    <Animated.View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      pointerEvents="none"
      style={{
        flexDirection: "row",
        height,
        opacity: fade,
        overflow: "hidden",
        width: wipe.interpolate({ inputRange: [0, 1], outputRange: ["0%", "100%"] }),
        ...(sweepKey === undefined ? {} : { left: 0, position: "absolute", top: 0, zIndex: 2 }),
      }}
      testID={sweepKey === undefined ? undefined : "band-flip-edge"}
    >
      {brandChroma.map((color) => (
        <View key={color} style={{ backgroundColor: color, flex: 1 }} />
      ))}
    </Animated.View>
  );
};

/**
 * The channel block: `CH` over the number, in the channel's decade colour (amber without one). It is
 * the only place a channel's own colour is wider than a hairline. Where the film strip runs beneath
 * it, its lower corner is square.
 */
const ChannelBlock = ({
  decade,
  density,
  number,
  squareFoot = false,
  width,
}: {
  decade?: number | undefined;
  density: Density;
  number: string;
  squareFoot?: boolean;
  width: number;
}) => {
  const fill = decadeFillColor(decade);
  const tone = fill ? "onDecade" : "inverse";
  // A phone's block is 56px: three digits at the full size fill it edge to edge, so a long number
  // steps down to the guide's number size and keeps a margin.
  const numberRole = width === BLOCK_WIDTH.touch && number.length > 2 ? "guideNumber" : "bandNumber";
  const text = density === "tv" ? "pointer" : density;
  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={{
        alignItems: "center",
        backgroundColor: fill ?? playerChrome.signal,
        borderBottomLeftRadius: squareFoot ? 0 : 12,
        borderTopLeftRadius: 12,
        gap: width === BLOCK_WIDTH.wide ? 8 : 4,
        justifyContent: "center",
        width,
      }}
      testID="band-channel-block"
    >
      <Text density={text} shout textRole="label" tone={tone} tracking={0.2}>
        CH
      </Text>
      <Text density={text} testID="band-channel-number" textRole={numberRole} tone={tone}>
        {number}
      </Text>
    </View>
  );
};

/** The clock: the time in mono, its AM/PM smaller and quieter. */
const Clock = ({ density, label }: { density: Density; label: string }) => {
  const [time, meridiem] = label.split(" ");
  return (
    <View style={{ alignItems: "baseline", flexDirection: "row", gap: 4 }}>
      <Text density={density} strong textRole="time" tone="primary">
        {time}
      </Text>
      {meridiem ? (
        <Text density={density} textRole="mono" tone="muted">
          {meridiem}
        </Text>
      ) : null}
    </View>
  );
};

/** Live stays red: a dot and the word. */
const LiveMark = ({ density }: { density: Density }) => (
  <View style={{ alignItems: "center", flexDirection: "row", gap: 8 }}>
    <View style={{ backgroundColor: playerChrome.live, borderRadius: 4, height: 8, width: 8 }} />
    <Text density={density} strong textRole="label" tone="live">
      Live
    </Text>
  </View>
);

/** Go live, behind live: an outline key in the live colour (#2777). */
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
        borderRadius: 8,
        borderWidth: 1,
        flexDirection: "row",
        gap: 8,
        height: density === "touch" ? 36 : 32,
        paddingHorizontal: 12,
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease",
        }),
        ...nativeFocus(focused),
      }}
    >
      <View style={{ backgroundColor: playerChrome.live, borderRadius: 4, height: 8, width: 8 }} />
      <Text density={density} strong textRole="label" tone="live">
        Go live
      </Text>
    </Pressable>
  );
};

/** "Now" and "Next": small plain labels in a fixed gutter, not boxed chips (§9.6). */
const Gutter = ({ children, density, width }: { children?: string; density: Density; width: number }) => (
  <View style={{ width }}>
    {children ? (
      <Text density={density} textRole="cardMeta" tone="muted">
        {children}
      </Text>
    ) : null}
  </View>
);

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
      minWidth: 0,
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

const Row = ({ children, gap = 16 }: { children: ReactNode; gap?: number }) => (
  <View style={{ alignItems: "baseline", flexDirection: "row", gap, minWidth: 0 }}>{children}</View>
);

const ChannelBand = ({
  band,
  density,
  formatTime,
  ident = false,
  live,
  onGoLive,
  portrait = false,
  schedule,
  tv = false,
}: ChannelBandProps) => {
  // A television's band is desktop's band at ten feet: the same sizes in its 960 × 540 frame.
  const text: Density = density === "tv" ? "pointer" : density;
  const wide = text === "pointer";
  const behind = live.mode !== "live";
  const hasSlot = band.startMs !== undefined && band.stopMs !== undefined;
  const strip = wide && !ident && schedule !== undefined;
  const station = ident && band.station ? ` · ${band.station}` : "";
  const gutter = wide ? 40 : 36;
  const [headerHeight, setHeaderHeight] = useState(0);
  const minutesLeft =
    band.stopMs !== undefined && band.clockMs !== undefined
      ? Math.max(1, Math.round((band.stopMs - band.clockMs) / 60_000))
      : undefined;

  const header = (
    <View
      style={{
        backgroundColor: playerChrome.band,
        borderBottomRightRadius: strip ? 0 : 12,
        borderBottomWidth: strip ? 0 : 1,
        borderColor: playerChrome.hairline,
        borderRightWidth: 1,
        borderTopRightRadius: 12,
        borderTopWidth: 1,
        flex: 1,
        gap: wide ? 8 : 4,
        minWidth: 0,
        paddingHorizontal: wide ? 24 : 16,
        paddingVertical: wide ? 16 : 12,
      }}
    >
      <View
        style={{
          alignItems: "center",
          flexDirection: "row",
          gap: 16,
          justifyContent: "space-between",
          minHeight: 32,
        }}
      >
        <Text
          density={text}
          numberOfLines={1}
          strong
          style={{ flexShrink: 1 }}
          testID="band-channel-name"
          textRole="label"
          tone="primary"
        >
          {`${band.channelName}${station}`}
        </Text>
        {ident ? null : (
          <View style={{ alignItems: "center", flexDirection: "row", flexShrink: 0, gap: 16 }}>
            {behind && wide && band.behindLabel ? (
              <Text density={text} textRole="cardMeta" tone="muted">
                {band.behindLabel}
              </Text>
            ) : null}
            {behind ? <GoLive density={text} onPress={onGoLive} /> : <LiveMark density={text} />}
            {band.clockMs === undefined || (behind && !wide) ? null : (
              <Clock density={text} label={formatTime(band.clockMs)} />
            )}
          </View>
        )}
      </View>

      {band.title ? (
        <Row>
          <Gutter density={text} width={gutter}>
            Now
          </Gutter>
          <View
            style={
              wide
                ? { alignItems: "baseline", flex: 1, flexDirection: "row", gap: 16, minWidth: 0 }
                : { flex: 1, minWidth: 0 }
            }
          >
            {/* When the row is tight the episode line gives way first, then the title. */}
            <Text
              density={text}
              numberOfLines={portrait ? 2 : 1}
              strong
              style={{ flexShrink: 1, minWidth: 0 }}
              testID="band-title"
              textRole={wide ? "title" : "label"}
              tone="primary"
            >
              {band.title}
            </Text>
            {band.inBreak && band.breakMinutesLeft !== undefined ? (
              <Text
                density={text}
                numberOfLines={1}
                strong
                style={{ flexShrink: 100_000, minWidth: 0 }}
                textRole={wide ? "reading" : "cardMeta"}
                tone="info"
              >
                {`Back in ${band.breakMinutesLeft} min`}
              </Text>
            ) : band.line ? (
              <Text
                density={text}
                numberOfLines={1}
                style={{ flexShrink: 100_000, minWidth: 0 }}
                testID="band-line"
                textRole={wide ? "reading" : "cardMeta"}
                tone="secondary"
              >
                {band.line}
              </Text>
            ) : null}
            {strip && !band.inBreak && minutesLeft !== undefined ? (
              <Text
                density={text}
                numberOfLines={1}
                style={{ flexShrink: 0, marginLeft: "auto" }}
                textRole="reading"
                tone="secondary"
              >
                {`${minutesLeft} min left`}
              </Text>
            ) : null}
          </View>
        </Row>
      ) : ident ? null : (
        // A channel with nothing scheduled says so, with no Now, Next or strip.
        <View style={{ justifyContent: "center", minHeight: wide ? 36 : 24 }}>
          <Text density={text} textRole={wide ? "reading" : "cardMeta"} tone="secondary">
            Nothing scheduled
          </Text>
        </View>
      )}

      {ident || strip || !hasSlot || !band.title ? null : (
        <Row gap={wide ? 16 : 12}>
          <Gutter density={text} width={gutter} />
          <View style={{ alignItems: "center", flex: 1, flexDirection: "row", gap: 12, minWidth: 0 }}>
            <Text density={text} textRole="cardTime" tone="secondary">
              {formatTime(band.startMs ?? 0)}
            </Text>
            <Progress liveProgress={band.liveProgress} progress={band.progress ?? 0} />
            <Text density={text} textRole="cardTime" tone="secondary">
              {formatTime(band.stopMs ?? 0)}
            </Text>
          </View>
        </Row>
      )}

      {ident || strip || !band.next || !band.title ? null : (
        <Row gap={wide ? 16 : 12}>
          <Gutter density={text} width={gutter}>
            Next
          </Gutter>
          <View style={{ alignItems: "baseline", flex: 1, flexDirection: "row", gap: 12, minWidth: 0 }}>
            {/* One unbroken token: the time keeps its width, the title takes the rest. */}
            <Text
              density={text}
              numberOfLines={1}
              style={{ flexShrink: 0 }}
              testID="band-next-time"
              textRole="cardTime"
              tone="secondary"
            >
              {formatTime(band.next.startMs)}
            </Text>
            <Text
              density={text}
              numberOfLines={1}
              style={{ flexShrink: 1, minWidth: 0 }}
              textRole={wide ? "body" : "cardMeta"}
              tone="primary"
            >
              {band.next.name}
            </Text>
            {behind && !wide && !portrait && band.behindLabel ? (
              <Text
                density={text}
                numberOfLines={1}
                style={{ flexShrink: 0, marginLeft: "auto" }}
                textRole="cardMeta"
                tone="muted"
              >
                {band.behindLabel}
              </Text>
            ) : null}
          </View>
        </Row>
      )}
    </View>
  );

  return (
    <View style={{ maxWidth: BAND_MAX_WIDTH, minWidth: 0 }} testID="channel-band">
      <View
        onLayout={(e: LayoutChangeEvent) => setHeaderHeight(e.nativeEvent.layout.height)}
        style={{ alignItems: "stretch", flexDirection: "row", minWidth: 0 }}
      >
        <BarsEdge height={4} sweepKey={band.channelNumber} />
        <ChannelBlock
          decade={band.decade}
          density={density}
          number={band.channelNumber}
          squareFoot={strip}
          width={ident ? (wide ? 112 : 72) : wide ? BLOCK_WIDTH.wide : BLOCK_WIDTH.touch}
        />
        {header}
      </View>
      {strip && schedule ? (
        // The film strip is the band's foot, the full width of the band, under the block too. The
        // block on air carries no name: the row above has just said it. A block's card rises clear of
        // the header, so it covers the picture and not the band.
        <View
          style={{
            backgroundColor: playerChrome.band,
            borderBottomLeftRadius: 12,
            borderBottomRightRadius: 12,
            borderColor: playerChrome.hairline,
            borderTopWidth: 0,
            borderWidth: 1,
            paddingBottom: 12,
            paddingHorizontal: 16,
            paddingTop: 14,
          }}
        >
          <PreviewPlaybar
            cardLift={headerHeight}
            density={text}
            formatTime={formatTime}
            pinNow={schedule.info === true}
            playheadMs={schedule.playheadMs}
            pointer={!tv}
            quietNow
            segments={schedule.segments}
            tv={tv}
            window={schedule.window}
            {...(schedule.onSeek ? { onSeek: schedule.onSeek } : {})}
            {...(schedule.onSelect ? { onSelect: schedule.onSelect } : {})}
          />
        </View>
      ) : null}
    </View>
  );
};

export type { ChannelBandProps };
export { BarsEdge, ChannelBand, ChannelBlock };
