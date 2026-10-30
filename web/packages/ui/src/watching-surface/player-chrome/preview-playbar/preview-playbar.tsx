import { type Density, playerChrome, Text } from "@loomarr/design-system";
import { useState } from "react";
import { type LayoutChangeEvent, Pressable, View } from "react-native";

import { glass, gradientOr, webOnly } from "../glass";
import { layoutPlaybar } from "../playbar-layout";
import { PlaybarStill } from "../playbar-still";
import type { PlaybarBreak, PlaybarSegment, PlaybarWindow } from "../player-chrome.type";
import { CARD_WIDTH, PreviewCard } from "../preview-card";

// The preview playbar (§9.3, plane 3), on a glass tray. Time-true: a block is as wide as its share
// of the window, fifteen minutes behind live to about two hours ahead. Programmes carry their still,
// name and start; a break carries one thumbnail per spot, sized by duration. Depth is light, never
// size (§9.2): every block is the same height and nothing moves; now has an amber edge, the block
// under the pointer is fully lit with a white ring, and the rest of the future is dimmed. Pointing
// at a block opens its preview card above it (pointer only), joined to it by a connector.

const GAP = 4;
const MIN_BREAK = 36;
const TICK_MINUTES = [30, 60, 90] as const;

interface PreviewPlaybarProps {
  density: Density;
  formatTime: (ms: number) => string;
  /** Called as a preview card opens and closes, so the band can step back behind it. */
  onPreviewChange?: (open: boolean) => void;
  playheadMs: number;
  /** Pointer devices preview on hover; touch has no card. */
  pointer: boolean;
  segments: readonly PlaybarSegment[];
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
  density,
  formatTime,
  onPreviewChange,
  playheadMs,
  pointer,
  segments,
  window,
}: PreviewPlaybarProps) => {
  const [width, setWidth] = useState(0);
  const [hovered, setHovered] = useState<string | undefined>(undefined);
  const hover = (id: string | undefined) => {
    setHovered(id);
    onPreviewChange?.(id !== undefined);
  };
  const height = pointer ? 64 : 44;
  const { boxes, xAt } = layoutPlaybar(segments, window, width, { gap: GAP, minBreak: MIN_BREAK });
  const head = xAt(Math.min(playheadMs, window.nowMs));
  const liveX = xAt(window.nowMs);
  const behind = playheadMs < window.nowMs - 1000;
  const openBox = boxes.find((b) => b.id === hovered);
  const openMid = openBox ? openBox.x + openBox.width / 2 : 0;

  return (
    <View aria-label="Schedule" role="group">
      {openBox && pointer ? (
        <>
          <View
            pointerEvents="none"
            style={{
              bottom: "100%",
              left: Math.min(Math.max(openMid + 6 - CARD_WIDTH / 2, 0), Math.max(0, width + 12 - CARD_WIDTH)),
              marginBottom: 20,
              position: "absolute",
              zIndex: 20,
            }}
          >
            <PreviewCard
              density={density}
              formatTime={formatTime}
              nowMs={window.nowMs}
              onNowSpot={
                openBox.segment.kind === "break" ? onNowSpot(openBox.segment, playheadMs) : undefined
              }
              segment={openBox.segment}
            />
          </View>
          <View
            pointerEvents="none"
            style={{
              bottom: "100%",
              height: 26,
              left: openMid + 6,
              position: "absolute",
              width: 1,
              zIndex: 20,
              ...gradientOr(
                `linear-gradient(0deg, ${playerChrome.focusRing}, transparent)`,
                playerChrome.focusRing,
              ),
            }}
          />
        </>
      ) : null}

      {/* The tray: a lit glass shelf the blocks stand on, so the bar has a surface of its own. */}
      <View style={{ borderRadius: 12, padding: 6, ...glass(playerChrome.shadowPlane) }}>
        <View onLayout={(e: LayoutChangeEvent) => setWidth(e.nativeEvent.layout.width)} style={{ height }}>
          {width > 0
            ? boxes.map((box) => {
                const { segment } = box;
                const lit = segment.id === hovered;
                const isNow = segment.startMs <= window.nowMs && window.nowMs < segment.stopMs;
                const future = segment.startMs > window.nowMs;
                const filter = lit ? playerChrome.litFilter : future ? playerChrome.futureFilter : "none";
                const ring =
                  segment.kind === "break"
                    ? { color: lit ? playerChrome.breakAccent : playerChrome.breakRing, width: lit ? 2 : 1 }
                    : {
                        color: lit
                          ? playerChrome.focusRing
                          : isNow
                            ? playerChrome.signal
                            : playerChrome.glassRing,
                        width: lit ? 2 : 1,
                      };
                return (
                  <Pressable
                    accessibilityLabel={
                      segment.kind === "break"
                        ? `Break at ${formatTime(segment.startMs)}`
                        : `${segment.name}, ${formatTime(segment.startMs)}`
                    }
                    focusable={false}
                    key={segment.id}
                    onHoverIn={pointer ? () => hover(segment.id) : undefined}
                    onHoverOut={pointer ? () => hover(undefined) : undefined}
                    style={{
                      borderRadius: 6,
                      height: "100%",
                      left: box.x,
                      overflow: "hidden",
                      position: "absolute",
                      width: box.width,
                      zIndex: lit ? 10 : 1,
                      ...webOnly({
                        boxShadow: `inset 0 0 0 ${ring.width}px ${ring.color}`,
                        filter,
                        transition: "filter 220ms ease, box-shadow 220ms ease",
                      }),
                    }}
                  >
                    {segment.kind === "break" ? (
                      <View
                        style={{
                          backgroundColor: playerChrome.breakTint,
                          flexDirection: "row",
                          gap: 2,
                          height: "100%",
                          padding: 2,
                        }}
                      >
                        {segment.spots.map((spot) => (
                          <PlaybarStill
                            key={spot.id}
                            style={{
                              borderRadius: 3,
                              flexBasis: 0,
                              flexGrow: spot.durationMs,
                              height: "100%",
                            }}
                            uri={spot.stillUri}
                          />
                        ))}
                      </View>
                    ) : (
                      <>
                        <PlaybarStill
                          style={{ height: "100%", position: "absolute", width: "100%" }}
                          uri={segment.stillUri}
                        />
                        <View
                          style={{
                            bottom: 0,
                            justifyContent: "flex-end",
                            left: 0,
                            paddingBottom: 6,
                            paddingHorizontal: 10,
                            position: "absolute",
                            right: 0,
                            top: 0,
                            ...gradientOr(
                              "linear-gradient(90deg, rgba(0, 0, 0, 0.8), rgba(0, 0, 0, 0.4) 50%, transparent)",
                              playerChrome.scrimMid,
                            ),
                          }}
                        >
                          <Text density={density} numberOfLines={1} strong textRole="cardMeta" tone="primary">
                            {segment.name}
                          </Text>
                          {pointer ? (
                            <Text density={density} numberOfLines={1} textRole="guideMeta" tone="secondary">
                              {formatTime(segment.startMs)}
                            </Text>
                          ) : null}
                        </View>
                      </>
                    )}
                  </Pressable>
                );
              })
            : null}

          {/* What's been watched: a warm wash up to the playhead, under the glowing playhead. */}
          <View
            pointerEvents="none"
            style={{
              borderBottomLeftRadius: 6,
              borderTopLeftRadius: 6,
              bottom: 0,
              left: 0,
              position: "absolute",
              top: 0,
              width: Math.max(0, head),
              zIndex: 15,
              ...gradientOr(
                `linear-gradient(90deg, transparent, ${playerChrome.signalWash})`,
                playerChrome.signalPast,
              ),
            }}
          />
          <View
            pointerEvents="none"
            style={{
              backgroundColor: playerChrome.signal,
              bottom: 0,
              left: head - 1,
              position: "absolute",
              top: -8,
              width: 2,
              zIndex: 30,
              ...webOnly({ boxShadow: playerChrome.signalGlow }),
            }}
          >
            <View
              style={{
                backgroundColor: playerChrome.signal,
                borderColor: playerChrome.signalPastRing,
                borderRadius: 999,
                borderWidth: 3,
                height: 14,
                left: -6,
                position: "absolute",
                top: -6,
                width: 14,
                ...webOnly({ backgroundClip: "padding-box", boxShadow: playerChrome.signalGlow }),
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
              style={{ left: 6, position: "absolute" }}
              textRole="guideMeta"
              tone="muted"
            >
              −15m
            </Text>
            <Text
              density={density}
              strong
              style={{ left: liveX + 6, position: "absolute", transform: [{ translateX: "-50%" }] }}
              textRole="guideMeta"
              tone="live"
            >
              LIVE
            </Text>
            {TICK_MINUTES.map((m) => {
              const at = window.nowMs + m * 60_000;
              if (at >= window.endMs) return null;
              return (
                <Text
                  density={density}
                  key={m}
                  style={{ left: xAt(at) + 6, position: "absolute", transform: [{ translateX: "-50%" }] }}
                  textRole="guideMeta"
                  tone="muted"
                >
                  {formatTime(at)}
                </Text>
              );
            })}
          </>
        ) : null}
      </View>
    </View>
  );
};

export type { PreviewPlaybarProps };
export { PreviewPlaybar };
