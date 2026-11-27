import {
  formatGuideTimeRange,
  formatProgrammeTitle,
  type GuideAiringLayout,
  type GuideSelection,
} from "@loomarr/core/guide";
import { PreviewAnchor, Surface, Text, useFocusLook } from "@loomarr/design-system";
import { type ComponentRef, type ReactNode, type RefObject, useEffect, useRef } from "react";
import { Pressable, View } from "react-native";

// Non-programme label thresholds from the mock, in pixels of block width.
// Programme identity remains visible in dense timelines; Text clips each line to its block.
const LABEL_MIN = 74;
const META_MIN = 132;
const CLIP_LABEL_MIN = 54;

const clipFill = {
  commercial: "guideClipSignal",
  psa: "guideClipSignal",
  trailer: "guideClipSignal",
  bumper: "guideClipTune",
  station_id: "guideClipSuggest",
  interstitial: "guideClipPlain",
  political: "guideClipSignal",
  news: "guideClipPlain",
} as const;

const selectionOf = (airing: GuideAiringLayout): GuideSelection => ({
  anchorMs: airing.source.startMs + (airing.source.stopMs - airing.source.startMs) / 2,
  channelId: airing.channelId,
  scheduleBlockId: airing.scheduleBlockId,
});

type BlockProps = {
  airing: GuideAiringLayout;
  /** Set when a key moved the selection here: the block takes focus once it mounts. */
  focusPending?: RefObject<boolean>;
  /** Bumped when the host asks focus to come home: the Tab stop takes it, as a moved selection does. */
  focusTick?: number;
  /** The grid's one Tab stop (roving tabindex). */
  focusable: boolean;
  /** The channel is paused: nothing airs, so no block wears the airing tone. */
  offAir?: boolean;
  onOpen?: () => void;
  onSelect?: (selection: GuideSelection) => void;
  preview?: ReactNode;
  px: number;
  selected: boolean;
  timezone?: string;
};

const Block = ({
  airing,
  focusPending,
  focusTick,
  focusable,
  offAir = false,
  onOpen,
  onSelect,
  preview,
  px,
  selected,
  timezone,
}: BlockProps) => {
  const ref = useRef<ComponentRef<typeof View>>(null);
  const { focused, ...focusHandlers } = useFocusLook({
    onFocus: () => {
      if (!selected) onSelect?.(selectionOf(airing));
    },
  });
  const a = airing.source;
  const kind = a.kind;
  const pending = kind === "pending";
  const pod = kind === "filler";
  // A paused channel's current block is neutral (maintainer, 2026-09-28): amber means "on the air
  // now", and the row's grey dot and Paused chip already say it isn't.
  const airingNow = !offAir && airing.isOnNow && (kind === "program" || kind === "flex");
  const when = formatGuideTimeRange(a.startMs, a.stopMs, timezone);
  const hasSeries = kind === "program" && Boolean(a.series) && Boolean(a.title.trim());
  const entries = a.pod?.entries ?? [];
  const podTotal = entries.reduce((n, e) => n + (e.durationMs || 0), 0) || 1;
  const fill = pod
    ? "guideBreakFill"
    : pending
      ? "guidePendingFill"
      : airingNow
        ? "stateAiringSurface"
        : "surfaceElevated";
  const edge = focused
    ? "actionFocus"
    : pod || airingNow
      ? "borderAiring"
      : pending
        ? "guidePendingBorder"
        : "borderDecorative";
  // Now-airing is a neutral tone step (#1909 Q3), so its edge takes the airing border.
  const accent = pod
    ? "transparent"
    : pending
      ? "stateInfo"
      : airingNow
        ? "borderAiring"
        : "guideBlockAccent";

  useEffect(() => {
    if ((selected || (focusable && focusTick)) && focusPending?.current) {
      focusPending.current = false;
      ref.current?.focus();
    }
  }, [focusPending, focusTick, focusable, selected]);

  return (
    <PreviewAnchor content={preview}>
      <Pressable
        accessibilityLabel={`${formatProgrammeTitle(a)}, ${when}`}
        accessibilityRole="button"
        // tabIndex, not focusable: react-native-web ignores focusable on a Pressable.
        tabIndex={focusable ? 0 : -1}
        {...focusHandlers}
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
          backgroundColor={fill}
          borderTopColor={edge}
          borderRightColor={edge}
          borderBottomColor={edge}
          borderLeftColor={accent}
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
                    backgroundColor={clipFill[entry.role]}
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
                    {entry.name && px * share > CLIP_LABEL_MIN ? (
                      <Text numberOfLines={1} textAlign="left" textRole="mono">
                        {entry.name}
                      </Text>
                    ) : null}
                  </Surface>
                );
              })}
            </View>
          ) : kind === "program" || px > LABEL_MIN ? (
            // textAlign: a role=button element centres its text on web, and the mock's labels are
            // flush left.
            <>
              {hasSeries ? (
                <Text numberOfLines={1} textAlign="left" textRole="label">
                  {a.series}
                  {/* A trailing space, collapsed on screen: axe (4.14, WCAG 2.5.3) joins sibling text
                      with nothing between, so "series" + "title" reads as one word against this
                      block's label. */}{" "}
                </Text>
              ) : null}
              <Text
                numberOfLines={1}
                textAlign="left"
                textRole="cardLabel"
                // Secondary, not muted: light muted is 4.1:1 on the pending and elevated fills (axe
                // needs 4.5); dark's two tones are the same grey, so dark doesn't change.
                tone={pending || kind === "flex" ? "secondary" : "primary"}
              >
                {hasSeries ? a.title : formatProgrammeTitle(a)}{" "}
              </Text>
              {!hasSeries && px >= META_MIN ? (
                <Text numberOfLines={1} textAlign="left" textRole="mono">
                  {when}
                </Text>
              ) : null}
            </>
          ) : null}
        </Surface>
      </Pressable>
    </PreviewAnchor>
  );
};

type WaitingBlockProps = Omit<BlockProps, "airing" | "offAir" | "preview" | "px" | "timezone"> & {
  airings: GuideAiringLayout[];
  label: string;
};

// A channel still downloading its titles, with nothing but breaks in the window, is ONE labelled
// block (#2727) instead of hours of unlabelled pod slivers. It stands for every airing in the row,
// so the keyboard model is unchanged: it is selected whenever one of them is.
const WaitingBlock = ({
  airings,
  focusPending,
  focusTick,
  focusable,
  label,
  onOpen,
  onSelect,
  selected,
}: WaitingBlockProps) => {
  const ref = useRef<ComponentRef<typeof View>>(null);
  const first = airings[0];
  const last = airings.at(-1);
  const { focused, ...focusHandlers } = useFocusLook({
    onFocus: () => {
      if (!selected && first) onSelect?.(selectionOf(first));
    },
  });

  useEffect(() => {
    if ((selected || (focusable && focusTick)) && focusPending?.current) {
      focusPending.current = false;
      ref.current?.focus();
    }
  }, [focusPending, focusTick, focusable, selected]);

  if (!first || !last) return null;
  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole="button"
      tabIndex={focusable ? 0 : -1}
      {...focusHandlers}
      onPress={onOpen}
      ref={ref}
      style={{
        bottom: 6,
        left: `${first.startRatio * 100}%`,
        position: "absolute",
        top: 6,
        width: `${(last.startRatio + last.widthRatio - first.startRatio) * 100}%`,
      }}
    >
      <Surface
        backgroundColor="guideBreakFill"
        borderColor={focused ? "actionFocus" : "borderDecorative"}
        borderRadius={4}
        borderWidth={1}
        flex={1}
        justifyContent="center"
        overflow="hidden"
        paddingHorizontal={8}
      >
        {/* Primary, as on a break block: muted on the break fill fails contrast (3.3:1, #2867). */}
        <Text numberOfLines={1} textAlign="left" textRole="cardLabel" tone="primary">
          {label}
        </Text>
      </Surface>
    </Pressable>
  );
};

export { Block, selectionOf, WaitingBlock };
