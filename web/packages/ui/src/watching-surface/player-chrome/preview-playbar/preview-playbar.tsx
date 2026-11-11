import { type Density, playerChrome, Text } from "@loomarr/design-system";
import { useState } from "react";
import { type LayoutChangeEvent, Platform, Pressable, View } from "react-native";

import { webOnly } from "../flat";
import { layoutPlaybar } from "../playbar-layout";
import { PlaybarStill } from "../playbar-still";
import type { PlaybarBreak, PlaybarSegment, PlaybarWindow } from "../player-chrome.type";
import { CARD_WIDTH, PreviewCard } from "../preview-card";

// The film strip (§9.2, variant J): the timeline of the programme on air and what follows, flat.
// Time-true: a block is as wide as its share of the window, fifteen minutes behind live to about two
// hours ahead; a break never shrinks below a legible tile. A programme is its still with a solid label
// bar; a break is one tile with a thumbnail per spot. The playhead is white, so amber stays on the
// channel block and focus. Pointing at a block (or, on a television, moving focus to it) opens its
// preview card above. It is the band's foot on desktop and television, and More's schedule on a phone.

const GAP = 4;
const MIN_BREAK = 40;
const TICK_MINUTES = [30, 60, 90] as const;
/** The block on air's quiet outline. */
const NOW_EDGE = "rgba(255, 255, 255, 0.4)";

interface PreviewPlaybarProps {
  /**
   * How far above the strip the card's foot sits, besides its 12px gap: the band's header, so a card
   * covers the picture and not the band.
   */
  cardLift?: number;
  density: Density;
  formatTime: (ms: number) => string;
  /** Called as a preview card opens and closes. */
  onPreviewChange?: (open: boolean) => void;
  /** Seek to an instant in the past; without it the strip only previews. */
  onSeek?: (wallClockMs: number) => void;
  playheadMs: number;
  /** Pointer devices preview on hover. */
  pointer: boolean;
  /** The block on air carries no name: the band above it has just said it (§9.2). */
  quietNow?: boolean;
  segments: readonly PlaybarSegment[];
  /** Television: every block takes remote focus, focus opens its card, and focus starts on now. */
  tv?: boolean;
  window: PlaybarWindow;
}

// The spot on air when the playhead is inside this break: walk the pod by duration.
const onNowSpot = (segment: PlaybarBreak, playheadMs: number) => {
  if (playheadMs < segment.startMs || playheadMs >= segment.stopMs) return undefined;
  let at = segment.startMs;
  const index = segment.spots.findIndex((spot) => {
    at += spot.durationMs;
    return playheadMs < at;
  });
  return index < 0 ? undefined : index;
};

const PreviewPlaybar = ({
  cardLift = 0,
  density,
  formatTime,
  onPreviewChange,
  onSeek,
  playheadMs,
  pointer,
  quietNow = false,
  segments,
  tv = false,
  window,
}: PreviewPlaybarProps) => {
  const [width, setWidth] = useState(0);
  const [open, setOpen] = useState<string | undefined>(undefined);
  const show = (id: string | undefined) => {
    setOpen(id);
    onPreviewChange?.(id !== undefined);
  };
  const height = pointer || tv ? 72 : 52;
  const { boxes, xAt } = layoutPlaybar(segments, window, width, { gap: GAP, minBreak: MIN_BREAK });
  const head = xAt(Math.min(playheadMs, window.nowMs));
  const liveX = xAt(window.nowMs);
  const behind = playheadMs < window.nowMs - 1000;
  const openBox = boxes.find((b) => b.id === open);
  const openMid = openBox ? openBox.x + openBox.width / 2 : 0;
  const previews = pointer || tv;

  return (
    <View aria-label="Schedule" role="group">
      {openBox && previews ? (
        <View
          pointerEvents="none"
          style={{
            bottom: "100%",
            left: Math.min(Math.max(openMid - CARD_WIDTH / 2, 0), Math.max(0, width - CARD_WIDTH)),
            marginBottom: (cardLift > 0 ? cardLift + 14 : 0) + 12,
            position: "absolute",
            zIndex: 20,
          }}
        >
          <PreviewCard
            density={density}
            formatTime={formatTime}
            nowMs={window.nowMs}
            onNowSpot={openBox.segment.kind === "break" ? onNowSpot(openBox.segment, playheadMs) : undefined}
            segment={openBox.segment}
          />
        </View>
      ) : null}

      <View onLayout={(e: LayoutChangeEvent) => setWidth(e.nativeEvent.layout.width)} style={{ height }}>
        {width > 0
          ? boxes.map((box) => {
              const { segment } = box;
              const isNow = segment.startMs <= window.nowMs && window.nowMs < segment.stopMs;
              const future = segment.startMs > window.nowMs;
              const lit = segment.id === open;
              // The past is seekable (#2088 decision 2): a block that has begun takes the viewer to
              // its start, or to the window's when the block began before it. The future previews.
              const seek = onSeek && !future ? () => onSeek(segment.startMs) : undefined;
              const name =
                segment.kind === "break"
                  ? `the break at ${formatTime(segment.startMs)}`
                  : `${segment.name}, ${formatTime(segment.startMs)}`;
              const edge = lit && tv ? playerChrome.signal : isNow ? NOW_EDGE : undefined;
              const labelled = segment.kind === "programme" && box.width >= 72 && !(quietNow && isNow);
              return (
                <Pressable
                  accessibilityLabel={seek ? `Seek to ${name}` : name.charAt(0).toUpperCase() + name.slice(1)}
                  focusable={tv || seek !== undefined}
                  hasTVPreferredFocus={tv && isNow}
                  key={segment.id}
                  onBlur={tv ? () => show(undefined) : undefined}
                  onFocus={tv ? () => show(segment.id) : undefined}
                  onHoverIn={pointer ? () => show(segment.id) : undefined}
                  onHoverOut={pointer ? () => show(undefined) : undefined}
                  onPress={seek}
                  // A past tile seeks, so it is a button; any other is a labelled preview, a group
                  // that names it without claiming an action.
                  role={seek ? "button" : "group"}
                  style={{
                    borderRadius: 6,
                    height: "100%",
                    left: box.x,
                    position: "absolute",
                    width: box.width,
                    zIndex: lit ? 10 : 1,
                    ...webOnly({
                      filter: future && !lit ? playerChrome.futureFilter : "none",
                      outlineColor: edge ?? "transparent",
                      outlineOffset: -1,
                      outlineStyle: edge ? "solid" : "none",
                      outlineWidth: 2,
                      transition: "filter 120ms ease",
                    }),
                    // Native has no outline: the same edge as a border.
                    ...(edge && Platform.OS !== "web" ? { borderColor: edge, borderWidth: 2 } : null),
                  }}
                  testID={isNow ? "strip-block-now" : undefined}
                >
                  {segment.kind === "break" ? (
                    <View
                      style={{
                        backgroundColor: playerChrome.breakTint,
                        borderColor: lit ? playerChrome.breakAccent : playerChrome.hairline,
                        borderRadius: 6,
                        borderWidth: 1,
                        flexDirection: "row",
                        gap: 1,
                        height: "100%",
                        padding: 2,
                      }}
                    >
                      {segment.spots.map((spot) => (
                        <PlaybarStill
                          key={spot.id}
                          style={{ borderRadius: 3, flexBasis: 0, flexGrow: spot.durationMs, height: "100%" }}
                          {...(spot.stillUri ? { uri: spot.stillUri } : {})}
                        />
                      ))}
                    </View>
                  ) : (
                    <View
                      style={{
                        borderColor: playerChrome.hairline,
                        borderRadius: 6,
                        borderWidth: 1,
                        height: "100%",
                        overflow: "hidden",
                      }}
                    >
                      <PlaybarStill
                        style={{ height: "100%", position: "absolute", width: "100%" }}
                        {...(segment.stillUri ? { uri: segment.stillUri } : {})}
                      />
                      {labelled ? (
                        <View
                          style={{
                            alignItems: "baseline",
                            backgroundColor: playerChrome.tooltip,
                            bottom: 0,
                            flexDirection: "row",
                            gap: 8,
                            justifyContent: "space-between",
                            left: 0,
                            paddingHorizontal: 8,
                            paddingVertical: 3,
                            position: "absolute",
                            right: 0,
                          }}
                        >
                          {/* Left-aligned by hand: a seekable tile is a button, which the web centres. */}
                          <Text
                            density={density}
                            numberOfLines={1}
                            strong
                            style={{ flexShrink: 1, minWidth: 0, textAlign: "left" }}
                            textRole="cardMeta"
                            tone="primary"
                          >
                            {/* The space keeps the name and the time two words on the web. */}
                            {`${segment.name} `}
                          </Text>
                          {(pointer || tv) && box.width >= 150 ? (
                            <Text density={density} textRole="mono" tone="secondary">
                              {formatTime(segment.startMs)}
                            </Text>
                          ) : null}
                        </View>
                      ) : null}
                    </View>
                  )}
                </Pressable>
              );
            })
          : null}

        {/* What's been watched: a faint wash up to the playhead, under the white playhead. */}
        <View
          pointerEvents="none"
          style={{
            backgroundColor: playerChrome.signalPast,
            borderBottomLeftRadius: 6,
            borderTopLeftRadius: 6,
            bottom: 0,
            left: 0,
            position: "absolute",
            top: 0,
            width: Math.max(0, head),
            zIndex: 15,
          }}
        />
        <View
          pointerEvents="none"
          style={{
            backgroundColor: playerChrome.text,
            bottom: 0,
            left: head - 1,
            position: "absolute",
            top: -8,
            width: 2,
            zIndex: 30,
          }}
          testID="strip-playhead"
        >
          <View
            style={{
              backgroundColor: playerChrome.text,
              borderRadius: 999,
              height: 10,
              left: -4,
              position: "absolute",
              top: -4,
              width: 10,
            }}
          />
        </View>
        {behind ? (
          <View
            pointerEvents="none"
            style={{
              backgroundColor: playerChrome.live,
              bottom: 0,
              left: liveX - 1,
              opacity: 0.8,
              position: "absolute",
              top: 0,
              width: 2,
              zIndex: 30,
            }}
          />
        ) : null}
      </View>

      {/* The scale: the seekable past's edge, live, and the next hours. */}
      <View
        accessibilityElementsHidden
        importantForAccessibility="no-hide-descendants"
        style={{ height: 16, marginTop: 6 }}
      >
        {width > 0 ? (
          <>
            <Text
              density={density}
              style={{ left: 0, position: "absolute" }}
              textRole="cardMeta"
              tone="muted"
            >
              −15m
            </Text>
            <Text
              density={density}
              strong
              style={{ left: liveX, position: "absolute", transform: [{ translateX: "-50%" }] }}
              textRole="cardMeta"
              tone="live"
            >
              Live
            </Text>
            {pointer || tv
              ? TICK_MINUTES.map((m) => {
                  const at = window.nowMs + m * 60_000;
                  if (at >= window.endMs) return null;
                  return (
                    <Text
                      density={density}
                      key={m}
                      style={{ left: xAt(at), position: "absolute", transform: [{ translateX: "-50%" }] }}
                      textRole="mono"
                      tone="muted"
                    >
                      {formatTime(at)}
                    </Text>
                  );
                })
              : null}
          </>
        ) : null}
      </View>
    </View>
  );
};

export type { PreviewPlaybarProps };
export { PreviewPlaybar };
