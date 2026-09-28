import {
  type ChannelIdentHue,
  channelIdentHue,
  formatGuideTime,
  formatGuideTimeRange,
  type GuideAiringLayout,
  type GuideChannelLayout,
  type GuideHealthState,
  type GuideNavigationDirection,
  type GuideSelection,
  guideAiringLabel,
  guideChannelForTypeahead,
  guideChannelState,
  guideSelectionForChannel,
  monogramOf,
} from "@loomarr/core/guide";
import { brandChroma, Surface, Text, type TextTone } from "@loomarr/design-system";
import { memo, type ReactNode, type RefObject, useEffect, useRef, useState } from "react";
import { type LayoutChangeEvent, Platform, Pressable, View } from "react-native";

import type { GuideGridProps } from "./guide-grid.type";
import { GuideRows } from "./guide-rows";

// The web mock's time grid (#1659, decision N7): a 260 px channel column beside a timeline of
// blocks, one 56 px row per channel. GuideRows virtualises the rows per platform. The keyboard
// model is a roving tabindex: the selected block is the grid's one Tab stop, arrow keys move it
// through the guide controller, and typing a channel number or name jumps to that channel.

const RAIL = 260;
const ROW = 56;
const RULER = 30;
const hourMs = 3_600_000;

const arrowDirection: Record<string, GuideNavigationDirection> = {
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
  ArrowUp: "up",
};
// Keys typed within this gap build one query ("1", "12"); a longer pause starts a new one.
const TYPEAHEAD_MS = 800;

// Label thresholds from the mock, in pixels of block width.
const LABEL_MIN = 74;
const META_MIN = 132;
const CLIP_LABEL_MIN = 54;

const hueHex: Record<ChannelIdentHue, string> = {
  tune: brandChroma[3],
  suggest: brandChroma[4],
  signal: brandChroma[0],
  onair: brandChroma[5],
};

const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
};

// The row chip names what is wrong or in progress; a healthy channel shows none. The mock's
// words where it has them, and the shipped web grid's for the two it doesn't draw.
const healthChip: Record<GuideHealthState, { label: string; tone: TextTone }> = {
  "pending-slots": { label: "Still downloading", tone: "info" },
  drift: { label: "Updating", tone: "signal" },
  paused: { label: "Paused", tone: "muted" },
  creating: { label: "Creating", tone: "muted" },
  error: { label: "Error", tone: "danger" },
};

const clipFill = {
  commercial: "$guideClipSignal",
  psa: "$guideClipSignal",
  trailer: "$guideClipSignal",
  bumper: "$guideClipTune",
  station_id: "$guideClipSuggest",
  interstitial: "$guideClipPlain",
} as const;

// The hatch the mock lays behind a monogram. CSS on web; native keeps the flat tint.
const hatch = "repeating-linear-gradient(135deg, transparent 0 3px, rgba(255, 255, 255, 0.05) 3px 6px)";

const Ident = ({ name, number }: { name: string; number: number }) => {
  const hex = hueHex[channelIdentHue(number)];
  return (
    <View
      aria-hidden
      style={[
        {
          alignItems: "center",
          backgroundColor: withAlpha(hex, 0.08),
          borderColor: withAlpha(hex, 0.3),
          borderRadius: 5,
          borderWidth: 1,
          height: 30,
          justifyContent: "center",
          width: 30,
        },
        Platform.OS === "web" ? ({ backgroundImage: hatch } as object) : null,
      ]}
    >
      {/* The hue is per channel, so it can't be a text tone. */}
      <Text style={{ color: hex }} textRole="guideIdent">
        {monogramOf(name)}
      </Text>
    </View>
  );
};

const selectionOf = (airing: GuideAiringLayout): GuideSelection => ({
  anchorMs: airing.source.startMs + (airing.source.stopMs - airing.source.startMs) / 2,
  channelId: airing.channelId,
  scheduleBlockId: airing.scheduleBlockId,
});

type BlockProps = {
  airing: GuideAiringLayout;
  /** Set when a key moved the selection here: the block takes focus once it mounts. */
  focusPending?: RefObject<boolean>;
  /** The grid's one Tab stop (roving tabindex). */
  focusable: boolean;
  onHover?: (selection: GuideSelection | undefined) => void;
  onOpen?: () => void;
  onSelect?: (selection: GuideSelection) => void;
  px: number;
  selected: boolean;
  timezone?: string;
};

const Block = ({
  airing,
  focusPending,
  focusable,
  onHover,
  onOpen,
  onSelect,
  px,
  selected,
  timezone,
}: BlockProps) => {
  const ref = useRef<View>(null);
  const [focused, setFocused] = useState(false);
  const a = airing.source;
  const kind = a.kind;
  const pending = kind === "pending";
  const pod = kind === "filler";
  const airingNow = airing.isOnNow && (kind === "program" || kind === "flex");
  const when = formatGuideTimeRange(a.startMs, a.stopMs, timezone);
  const hasSeries = kind === "program" && Boolean(a.series) && Boolean(a.title.trim());
  const entries = a.pod?.entries ?? [];
  const podTotal = entries.reduce((n, e) => n + (e.durationMs || 0), 0) || 1;

  useEffect(() => {
    if (selected && focusPending?.current) {
      focusPending.current = false;
      ref.current?.focus();
    }
  }, [focusPending, selected]);

  return (
    <Pressable
      accessibilityLabel={`${guideAiringLabel(a)}, ${when}`}
      accessibilityRole="button"
      // tabIndex, not focusable: react-native-web ignores focusable on a Pressable.
      tabIndex={focusable ? 0 : -1}
      onBlur={() => setFocused(false)}
      onFocus={() => {
        setFocused(true);
        if (!selected) onSelect?.(selectionOf(airing));
      }}
      onHoverIn={() => onHover?.(selectionOf(airing))}
      onHoverOut={() => onHover?.(undefined)}
      onPress={onOpen}
      ref={ref}
      style={{
        bottom: 6,
        left: `${airing.startRatio * 100}%`,
        position: "absolute",
        top: 6,
        width: `${airing.widthRatio * 100}%`,
      }}
    >
      <Surface
        backgroundColor={
          pod
            ? "$guideBreakFill"
            : pending
              ? "$guidePendingFill"
              : airingNow
                ? "$stateAiringSurface"
                : "$surfaceElevated"
        }
        borderColor={
          focused
            ? "$actionFocus"
            : pod || airingNow
              ? "$borderAiring"
              : pending
                ? "$guidePendingBorder"
                : "$borderDecorative"
        }
        borderLeftColor={
          pod ? "$transparent" : pending ? "$stateInfo" : airingNow ? "$actionPrimary" : "$guideBlockAccent"
        }
        borderLeftWidth={2}
        borderRadius={4}
        borderStyle={pending ? "dashed" : "solid"}
        borderWidth={1}
        flex={1}
        gap={1}
        justifyContent="center"
        overflow="hidden"
        paddingHorizontal={pod ? 2 : 8}
        paddingVertical={pod ? 2 : 4}
      >
        {pod ? (
          <View style={{ flexDirection: "row", gap: 1, height: "100%" }}>
            {entries.map((entry, i) => {
              const share = (entry.durationMs || 0) / podTotal;
              return (
                <Surface
                  backgroundColor={clipFill[entry.kind]}
                  borderRadius={2}
                  borderWidth={0}
                  flex={share}
                  justifyContent="center"
                  // Position is identity inside a break: one clip may legitimately repeat.
                  // biome-ignore lint/suspicious/noArrayIndexKey: position is identity in a pod
                  key={i}
                  minWidth={0}
                  overflow="hidden"
                  paddingHorizontal={3}
                >
                  {px * share > CLIP_LABEL_MIN ? (
                    <Text numberOfLines={1} textAlign="left" textRole="guideClip">
                      {entry.name}
                    </Text>
                  ) : null}
                </Surface>
              );
            })}
          </View>
        ) : px > LABEL_MIN ? (
          // textAlign: a role=button element centres its text on web, and the mock's labels are
          // flush left.
          <>
            {hasSeries ? (
              <Text numberOfLines={1} textAlign="left" textRole="guideLabel" textTransform="uppercase">
                {a.series}
              </Text>
            ) : null}
            <Text
              numberOfLines={1}
              textAlign="left"
              textRole="cardLabel"
              tone={pending || kind === "flex" ? "muted" : "primary"}
            >
              {hasSeries ? a.title : guideAiringLabel(a)}
            </Text>
            {!hasSeries && px >= META_MIN ? (
              <Text numberOfLines={1} textAlign="left" textRole="guideMeta">
                {when}
              </Text>
            ) : null}
          </>
        ) : null}
      </Surface>
    </Pressable>
  );
};

const nowLine = (ratio: number) => (
  <View
    pointerEvents="none"
    style={{
      backgroundColor: brandChroma[5],
      bottom: 0,
      left: `${ratio * 100}%`,
      position: "absolute",
      top: 0,
      width: 2,
    }}
  />
);

// Memoised: the web virtualiser re-renders on every scroll frame, and a row that stays in view
// shouldn't render again.
const Row = memo(
  ({
    channel,
    focusPending,
    nowRatio,
    onHover,
    onOpenChannel,
    onSelect,
    renderRowMenu,
    tabStop,
    timelineWidth,
    timezone,
  }: {
    channel: GuideChannelLayout;
    focusPending: RefObject<boolean>;
    nowRatio?: number;
    onHover?: (selection: GuideSelection | undefined) => void;
    onOpenChannel?: (channelId: string) => void;
    onSelect?: (selection: GuideSelection) => void;
    renderRowMenu?: (channel: GuideChannelLayout["source"]) => ReactNode;
    /** The block that is the grid's Tab stop, when it is in this row. */
    tabStop?: string;
    timelineWidth: number;
    timezone?: string;
  }) => {
    const { broadcast, health } = guideChannelState(channel.source);
    const chip = health ? healthChip[health] : undefined;
    return (
      <Surface
        backgroundColor="$transparent"
        borderBottomColor="$guideRowRule"
        borderBottomWidth={1}
        borderWidth={0}
        flexDirection="row"
        height={ROW + 1}
        // No fade for a channel that is off the air, unlike the mock: at the mock's 0.55 its text
        // fails WCAG contrast (2.4-3.8:1, #1705). The grey dot and the "Paused" chip mark it.
      >
        <Surface
          alignItems="center"
          backgroundColor="$surfaceCanvas"
          borderColor="$borderDecorative"
          borderRightWidth={1}
          borderWidth={0}
          flexDirection="row"
          gap={9}
          paddingLeft={12}
          paddingRight={4}
          width={RAIL}
        >
          <Ident name={channel.source.name} number={channel.source.number} />
          <Pressable
            accessibilityRole="button"
            // Out of the Tab order: Enter on any of the row's blocks opens the same channel, and
            // a hundred rows must not mean a hundred more Tab stops.
            tabIndex={-1}
            onPress={() => onOpenChannel?.(channel.source.channelId)}
            style={{ alignItems: "center", flex: 1, flexDirection: "row", gap: 9, minWidth: 0 }}
          >
            <Text textRole="guideNumber">{channel.source.number}</Text>
            <View style={{ alignItems: "flex-start", flex: 1, minWidth: 0 }}>
              <Text numberOfLines={1} textRole="cardLabel">
                {channel.source.name}
              </Text>
              {chip ? (
                <Text
                  marginTop={2}
                  numberOfLines={1}
                  textRole="guideLabel"
                  textTransform="uppercase"
                  tone={chip.tone}
                >
                  {chip.label}
                </Text>
              ) : null}
            </View>
          </Pressable>
          <Surface
            backgroundColor={
              broadcast === "live"
                ? "$guideOnAir"
                : broadcast === "reconciling"
                  ? "$guideReconciling"
                  : "$actionDisabled"
            }
            borderRadius="$round"
            borderWidth={0}
            height={8}
            width={8}
          />
          {/* The ⋯ slot: the page's channel menu until the maintainer mocks its contents. */}
          <View style={{ alignItems: "center", width: 28 }}>{renderRowMenu?.(channel.source)}</View>
        </Surface>
        <View style={{ flex: 1, height: ROW, overflow: "hidden", position: "relative" }}>
          {channel.airings.map((airing) => (
            <Block
              airing={airing}
              focusable={airing.scheduleBlockId === tabStop}
              focusPending={focusPending}
              key={airing.scheduleBlockId}
              onHover={onHover}
              onOpen={() => onOpenChannel?.(channel.source.channelId)}
              onSelect={onSelect}
              px={airing.widthRatio * timelineWidth}
              selected={airing.scheduleBlockId === tabStop}
              timezone={timezone}
            />
          ))}
          {nowRatio === undefined ? null : nowLine(nowRatio)}
        </View>
      </Surface>
    );
  },
);

const GuideGrid = ({
  layout,
  nowMs,
  onHover,
  onMove,
  onOpenChannel,
  onSelect,
  renderRowMenu,
  selection,
}: GuideGridProps) => {
  const [timelineWidth, setTimelineWidth] = useState(0);
  const focusPending = useRef(false);
  const typed = useRef({ at: 0, query: "" });
  const span = Math.max(1, layout.toMs - layout.fromMs);
  const nowRatio = nowMs >= layout.fromMs && nowMs < layout.toMs ? (nowMs - layout.fromMs) / span : undefined;
  const firstHour = Math.ceil(layout.fromMs / hourMs) * hourMs;
  const ticks: number[] = [];
  for (let t = firstHour; t < layout.toMs; t += hourMs) ticks.push(t);

  const ruler = (
    <Surface
      backgroundColor="$surfaceCanvas"
      borderBottomColor="$borderDecorative"
      borderBottomWidth={1}
      borderWidth={0}
      flexDirection="row"
    >
      <Surface
        backgroundColor="$surfaceCanvas"
        borderColor="$borderDecorative"
        borderRightWidth={1}
        borderWidth={0}
        width={RAIL}
      />
      <View
        onLayout={(e: LayoutChangeEvent) => setTimelineWidth(e.nativeEvent.layout.width)}
        style={{ flex: 1, height: RULER, overflow: "hidden", position: "relative" }}
      >
        {ticks.map((t) => (
          <Surface
            backgroundColor="$transparent"
            borderColor="$borderDecorative"
            borderLeftWidth={1}
            borderWidth={0}
            bottom={0}
            justifyContent="center"
            key={t}
            left={`${((t - layout.fromMs) / span) * 100}%`}
            paddingLeft={7}
            position="absolute"
            top={0}
          >
            <Text numberOfLines={1} textRole="guideMeta">
              {formatGuideTime(t, layout.timezone)}
            </Text>
          </Surface>
        ))}
        {nowRatio === undefined ? null : (
          <>
            {nowLine(nowRatio)}
            <View
              style={{
                backgroundColor: brandChroma[5],
                borderRadius: 3,
                left: `${nowRatio * 100}%`,
                paddingHorizontal: 5,
                paddingVertical: 1,
                position: "absolute",
                top: 4,
                transform: [{ translateX: -20 }],
              }}
            >
              <Text textRole="guideNow" tone="inverse">
                {formatGuideTime(nowMs, layout.timezone)}
              </Text>
            </View>
          </>
        )}
      </View>
    </Surface>
  );

  // One Tab stop: the selected block, else the first block of the first row.
  const tabStop = selection
    ? selection
    : layout.channels[0]?.airings[0]
      ? selectionOf(layout.channels[0].airings[0])
      : undefined;
  const tabStopRow = tabStop
    ? layout.channels.findIndex((channel) => channel.source.channelId === tabStop.channelId)
    : -1;

  const onKey = (key: string, modified: boolean): boolean => {
    if (modified || !tabStop) return false;
    const direction = arrowDirection[key];
    if (direction) {
      const moved = onMove?.(direction);
      if (moved && !moved.boundary) focusPending.current = true;
      return true;
    }
    if (key.length !== 1 || !/\S/.test(key)) return false;
    const at = Date.now();
    const query = at - typed.current.at < TYPEAHEAD_MS ? typed.current.query + key : key;
    typed.current = { at, query };
    const channelId = guideChannelForTypeahead(layout, query, tabStop.channelId);
    const next = channelId ? guideSelectionForChannel(layout, channelId, tabStop.anchorMs) : undefined;
    if (next && next.scheduleBlockId !== tabStop.scheduleBlockId) {
      focusPending.current = true;
      onSelect?.(next);
    }
    return true;
  };

  return (
    <GuideRows
      channels={layout.channels}
      focusIndex={tabStopRow < 0 ? undefined : tabStopRow}
      header={ruler}
      headerHeight={RULER + 1}
      onKey={onKey}
      renderRow={(channel) => (
        <Row
          channel={channel}
          focusPending={focusPending}
          nowRatio={nowRatio}
          onOpenChannel={onOpenChannel}
          onHover={onHover}
          onSelect={onSelect}
          renderRowMenu={renderRowMenu}
          tabStop={channel.source.channelId === tabStop?.channelId ? tabStop.scheduleBlockId : undefined}
          timelineWidth={timelineWidth}
          timezone={layout.timezone}
        />
      )}
      rowHeight={ROW + 1}
    />
  );
};

export { GuideGrid };
