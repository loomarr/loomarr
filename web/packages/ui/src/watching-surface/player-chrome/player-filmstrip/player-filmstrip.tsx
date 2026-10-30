import { type Density, Icon, icons, playerChrome, Text } from "@loomarr/design-system";
import { type ComponentRef, useEffect, useRef, useState } from "react";
import { type LayoutChangeEvent, Pressable, ScrollView, View } from "react-native";

import { webOnly } from "../glass";
import { PlaybarStill } from "../playbar-still";
import type { PlaybarBreak, PlaybarProgramme, PlaybarSegment, PlaybarWindow } from "../player-chrome.type";

// The portrait playbar (§9.3, mock "portrait"): touch has no hover card, so what's coming is a row of
// preview tiles the viewer scrolls — the fifteen minutes still on the stream as one rewind tile, then
// each programme with its still and each break with its spots — over a thin time-true track that
// keeps the window, the playhead and live. A tile that has begun is seekable, as the playbar's are.

const TICK_MINUTES = [30, 60, 90] as const;

interface PlayerFilmstripProps {
  density: Density;
  formatTime: (ms: number) => string;
  /** Seek to an instant in the past; without it the tiles only preview. */
  onSeek?: (wallClockMs: number) => void;
  playheadMs: number;
  segments: readonly PlaybarSegment[];
  window: PlaybarWindow;
}

const minutes = (ms: number) => Math.round(ms / 60_000);

const Badge = ({
  density,
  label,
  now,
  tint,
}: {
  density: Density;
  label: string;
  now: boolean;
  tint: string;
}) => (
  <View
    style={{
      backgroundColor: now ? tint : playerChrome.tooltip,
      borderRadius: 4,
      left: 6,
      paddingHorizontal: 6,
      paddingVertical: 1,
      position: "absolute",
      top: 6,
    }}
  >
    <Text density={density} strong textRole="guideMeta" tone={now ? "inverse" : "primary"}>
      {label}
    </Text>
  </View>
);

const ProgrammeTile = ({
  density,
  formatTime,
  now,
  onPress,
  playheadMs,
  segment,
}: {
  density: Density;
  formatTime: (ms: number) => string;
  now: boolean;
  onPress?: () => void;
  playheadMs: number;
  segment: PlaybarProgramme;
}) => {
  const touch = density === "touch";
  const width = now ? (touch ? 192 : 240) : touch ? 144 : 176;
  const progress = (playheadMs - segment.startMs) / (segment.stopMs - segment.startMs);
  return (
    <Pressable
      accessibilityLabel={`${onPress ? "Seek to " : ""}${segment.name}, ${formatTime(segment.startMs)}`}
      accessibilityRole={onPress ? "button" : undefined}
      disabled={!onPress}
      focusable={onPress !== undefined}
      onPress={onPress}
      style={{ gap: 6, width }}
    >
      <View
        style={{
          aspectRatio: 16 / 9,
          borderRadius: 6,
          overflow: "hidden",
          width: "100%",
          ...webOnly({ boxShadow: `inset 0 0 0 1px ${now ? playerChrome.signal : playerChrome.glassRing}` }),
        }}
      >
        <PlaybarStill
          style={{ height: "100%", position: "absolute", width: "100%" }}
          uri={segment.stillUri}
        />
        <Badge
          density={density}
          label={now ? "NOW" : formatTime(segment.startMs)}
          now={now}
          tint={playerChrome.live}
        />
        <View
          style={{
            backgroundColor: playerChrome.tooltip,
            borderRadius: 4,
            bottom: 6,
            paddingHorizontal: 4,
            position: "absolute",
            right: 6,
          }}
        >
          <Text density={density} textRole="guideMeta" tone="primary">
            {`${minutes(segment.stopMs - segment.startMs)}m`}
          </Text>
        </View>
        {now ? (
          <View
            style={{
              backgroundColor: playerChrome.scrimMid,
              bottom: 0,
              height: 4,
              left: 0,
              position: "absolute",
              right: 0,
            }}
          >
            <View
              style={{
                backgroundColor: playerChrome.signal,
                height: "100%",
                width: `${Math.round(Math.min(1, Math.max(0, progress)) * 100)}%`,
              }}
            />
          </View>
        ) : null}
      </View>
      {/* Left-aligned by hand: a seekable tile is a button, which the web centres. */}
      <View>
        <Text
          density={density}
          numberOfLines={1}
          strong
          style={{ textAlign: "left" }}
          textRole="cardMeta"
          tone="primary"
        >
          {segment.name}
        </Text>
        {segment.line ? (
          <Text
            density={density}
            numberOfLines={1}
            style={{ textAlign: "left" }}
            textRole="guideMeta"
            tone="secondary"
          >
            {segment.line}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
};

const BreakTile = ({
  airing,
  density,
  formatTime,
  onPress,
  segment,
}: {
  airing: boolean;
  density: Density;
  formatTime: (ms: number) => string;
  onPress?: () => void;
  segment: PlaybarBreak;
}) => {
  const touch = density === "touch";
  const length = `${minutes(segment.stopMs - segment.startMs)} min`;
  return (
    <Pressable
      accessibilityLabel={`${onPress ? "Seek to the" : "The"} break at ${formatTime(segment.startMs)}, ${length}`}
      accessibilityRole={onPress ? "button" : undefined}
      disabled={!onPress}
      focusable={onPress !== undefined}
      onPress={onPress}
      style={{ gap: 6, width: touch ? 112 : 144 }}
    >
      <View
        style={{
          aspectRatio: 16 / 9,
          backgroundColor: playerChrome.discInk,
          borderRadius: 6,
          flexDirection: "row",
          flexWrap: "wrap",
          gap: 1,
          overflow: "hidden",
          width: "100%",
          ...webOnly({
            boxShadow: `inset 0 0 0 1px ${airing ? playerChrome.breakAccent : playerChrome.breakRing}`,
          }),
        }}
      >
        {/* Up to six of the pod's spots, three by two. */}
        {segment.spots.slice(0, 6).map((spot) => (
          <PlaybarStill key={spot.id} style={{ height: "49.5%", width: "33%" }} uri={spot.stillUri} />
        ))}
        <Badge
          density={density}
          label={airing ? "NOW" : formatTime(segment.startMs)}
          now={airing}
          tint={playerChrome.breakAccent}
        />
      </View>
      <View>
        <Text
          density={density}
          numberOfLines={1}
          strong
          style={{ textAlign: "left" }}
          textRole="cardMeta"
          tone="info"
        >
          {`Break · ${length}`}
        </Text>
        <Text
          density={density}
          numberOfLines={1}
          style={{ textAlign: "left" }}
          textRole="guideMeta"
          tone="secondary"
        >
          {`${segment.spots.length} spots · ${segment.spots.map((spot) => spot.name).join(", ")}`}
        </Text>
      </View>
    </Pressable>
  );
};

const PlayerFilmstrip = ({
  density,
  formatTime,
  onSeek,
  playheadMs,
  segments,
  window,
}: PlayerFilmstripProps) => {
  const span = window.endMs - window.startMs;
  const pct = (ms: number) =>
    `${(span > 0 ? Math.min(1, Math.max(0, (ms - window.startMs) / span)) : 0) * 100}%` as const;
  const head = Math.min(playheadMs, window.nowMs);
  const playing = segments.find((s) => s.startMs <= head && head < s.stopMs);

  // The row opens on what's playing: a programme that ended inside the window sits before it, a
  // scroll back.
  const scrollRef = useRef<ComponentRef<typeof ScrollView>>(null);
  const [nowX, setNowX] = useState<number | undefined>(undefined);
  useEffect(() => {
    if (nowX !== undefined) scrollRef.current?.scrollTo({ animated: false, x: nowX });
  }, [nowX]);
  // The track's labels are placed in pixels, so LIVE can stand clear of −15m when live is near it.
  const [trackWidth, setTrackWidth] = useState(0);
  const xOf = (ms: number) =>
    span > 0 ? Math.min(1, Math.max(0, (ms - window.startMs) / span)) * trackWidth : 0;
  const EDGE_LABEL = 40;

  return (
    <View aria-label="Schedule" role="group" style={{ gap: 8 }}>
      <View style={{ alignItems: "flex-start", flexDirection: "row", gap: 10 }}>
        {/* The fifteen minutes the stream still holds, as one tile back to their start: held in
            place beside the row, as the mock draws it, however far the row is scrolled. */}
        <Pressable
          accessibilityLabel="Back to the start of the last 15 minutes"
          accessibilityRole={onSeek ? "button" : undefined}
          disabled={!onSeek}
          onPress={onSeek ? () => onSeek(window.startMs) : undefined}
          style={{
            alignItems: "center",
            aspectRatio: 9 / 16,
            borderColor: playerChrome.glassRing,
            borderRadius: 6,
            borderStyle: "dashed",
            borderWidth: 1,
            gap: 4,
            justifyContent: "center",
            width: 48,
          }}
        >
          <Icon color={playerChrome.textSecondary} decorative glyph={icons.startOver} size="compact" />
          <Text density={density} textRole="guideMeta" tone="secondary">
            −15m
          </Text>
        </Pressable>
        <ScrollView
          contentContainerStyle={{ alignItems: "flex-start", gap: 10, paddingBottom: 4 }}
          horizontal
          ref={scrollRef}
          showsHorizontalScrollIndicator={false}
          style={{ flex: 1 }}
        >
          {segments.map((segment) => {
            const seek =
              onSeek && segment.startMs <= window.nowMs ? () => onSeek(segment.startMs) : undefined;
            const isPlaying = segment.id === playing?.id;
            return (
              <View
                key={segment.id}
                onLayout={isPlaying ? (e: LayoutChangeEvent) => setNowX(e.nativeEvent.layout.x) : undefined}
              >
                {segment.kind === "break" ? (
                  <BreakTile
                    airing={isPlaying}
                    density={density}
                    formatTime={formatTime}
                    onPress={seek}
                    segment={segment}
                  />
                ) : (
                  <ProgrammeTile
                    density={density}
                    formatTime={formatTime}
                    now={isPlaying}
                    onPress={seek}
                    playheadMs={head}
                    segment={segment}
                  />
                )}
              </View>
            );
          })}
        </ScrollView>
      </View>

      {/* The track: the window to scale, what's been watched, the breaks, the playhead and live. */}
      <View
        accessibilityElementsHidden
        importantForAccessibility="no-hide-descendants"
        onLayout={(e: LayoutChangeEvent) => setTrackWidth(e.nativeEvent.layout.width)}
      >
        <View style={{ backgroundColor: playerChrome.track, borderRadius: 2, height: 4, overflow: "hidden" }}>
          <View
            style={{
              backgroundColor: playerChrome.signalWash,
              bottom: 0,
              left: 0,
              position: "absolute",
              top: 0,
              width: pct(window.nowMs),
            }}
          />
          <View
            style={{
              backgroundColor: playerChrome.signal,
              bottom: 0,
              left: 0,
              position: "absolute",
              top: 0,
              width: pct(head),
            }}
          />
          {segments
            .filter((s) => s.kind === "break")
            .map((s) => (
              <View
                key={s.id}
                style={{
                  backgroundColor: playerChrome.breakRing,
                  bottom: 0,
                  left: pct(s.startMs),
                  position: "absolute",
                  top: 0,
                  width: `${((Math.min(s.stopMs, window.endMs) - Math.max(s.startMs, window.startMs)) / span) * 100}%`,
                }}
              />
            ))}
        </View>
        <View
          style={{
            backgroundColor: playerChrome.signal,
            borderRadius: 999,
            height: 12,
            left: pct(head),
            marginLeft: -6,
            position: "absolute",
            top: -4,
            width: 12,
            ...webOnly({ boxShadow: playerChrome.signalGlow }),
          }}
        />
        <View style={{ height: 16, marginTop: 4 }}>
          <Text density={density} style={{ left: 0, position: "absolute" }} textRole="guideMeta" tone="muted">
            −15m
          </Text>
          {/* LIVE starts at the live line, or just clear of −15m where live is closer than that. */}
          <Text
            density={density}
            strong
            style={{ left: Math.max(xOf(window.nowMs), EDGE_LABEL), position: "absolute" }}
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
                style={{ left: xOf(at), position: "absolute", transform: [{ translateX: "-50%" }] }}
                textRole="guideMeta"
                tone="muted"
              >
                {formatTime(at)}
              </Text>
            );
          })}
        </View>
      </View>
    </View>
  );
};

export type { PlayerFilmstripProps };
export { PlayerFilmstrip };
