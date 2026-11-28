import { Surface, Text, ThemeScope } from "@loomarr/design-system";
import { useRef } from "react";
import { View } from "react-native";

import type { FilmStripLook, FilmStripProps } from "./film-strip.type";
import { groupSeriesRuns, type StripAiring, type StripProgramme } from "./series-runs";
import { type BlockState, STILL_HEIGHT, StripBlock } from "./strip-block";

// The strip's height beside its stills: the two lines of words under them.
const CAPTION_HEIGHT = 48;
/** The room above the stills for the playhead's head. */
const HEAD_ROOM = 6;

const defaultArtwork = (airing: StripAiring) => airing.thumbUrl || airing.backdropImage?.src || undefined;

const clamp = (value: number) => Math.min(1, Math.max(0, value));

const lookOf = (item: StripProgramme, nowMs: number): FilmStripLook => ({
  airing:
    item.airings.find((a) => a.startMs <= nowMs && nowMs < a.stopMs) ?? (item.airings[0] as StripAiring),
  episodes: item.episodes,
  startMs: item.startMs,
  stopMs: item.stopMs,
});

const stateOf = (item: StripProgramme, nowMs: number): BlockState =>
  item.stopMs <= nowMs ? "ended" : item.startMs > nowMs ? "later" : "now";

/**
 * One channel's day across (frontend-design §9.2, The Cross): the same part under the picture in a
 * desktop window and as the tuned channel's row in the Guide.
 */
const FilmStrip = ({
  airings,
  artworkUri = defaultArtwork,
  formatTime,
  layout = "strip",
  nowMs,
  onLook,
  onSelect,
  playheadMs = nowMs,
  window,
}: FilmStripProps) => {
  // Focus outlasts a pointer passing over: leaving a block returns the look to the focused one.
  const focusedLook = useRef<FilmStripLook | undefined>(undefined);
  const span = Math.max(1, window.stopMs - window.startMs);
  const at = (ms: number) => clamp((ms - window.startMs) / span);
  const items = groupSeriesRuns(airings).filter(
    (i) => i.stopMs > window.startMs && i.startMs < window.stopMs,
  );
  const programmes = items.filter((i): i is StripProgramme => i.type === "programme");
  const breaks = items.filter((i) => i.type === "break");
  const pictured = programmes.some((i) => artworkUri(lookOf(i, nowMs).airing));
  const headMs = Math.min(playheadMs, nowMs);
  const behind = nowMs - headMs >= 1000;
  const inWindow = (ms: number) => ms >= window.startMs && ms < window.stopMs;
  const height = HEAD_ROOM + STILL_HEIGHT + (layout === "strip" && pictured ? CAPTION_HEIGHT : 0);

  const playhead = (
    <Surface
      backgroundColor="contentPrimary"
      borderRadius={1}
      borderWidth={0}
      height={STILL_HEIGHT + HEAD_ROOM}
      width={2}
    />
  );

  return (
    <View aria-label="Schedule" role="group" style={{ height, overflow: "hidden", width: "100%" }}>
      <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: HEAD_ROOM }}>
        {programmes.map((item) => {
          const state = stateOf(item, nowMs);
          const look = lookOf(item, nowMs);
          const left = at(item.startMs);
          const width = at(item.stopMs) - left;
          const shownFrom = Math.max(item.startMs, window.startMs);
          const shownTo = Math.min(item.stopMs, window.stopMs);
          return (
            <StripBlock
              aired={state === "now" ? clamp((nowMs - shownFrom) / (shownTo - shownFrom)) : 0}
              airing={look.airing}
              artworkUri={artworkUri(look.airing)}
              episodes={item.episodes}
              formatTime={formatTime}
              key={item.id}
              layout={layout}
              left={left}
              onBlur={() => {
                focusedLook.current = undefined;
                onLook?.(undefined);
              }}
              onFocus={() => {
                focusedLook.current = look;
                onLook?.(look);
              }}
              onHoverIn={() => onLook?.(look)}
              onHoverOut={() => onLook?.(focusedLook.current)}
              onPress={() => onSelect?.(look)}
              startMs={item.startMs}
              state={state}
              stopMs={item.stopMs}
              width={width}
            />
          );
        })}
        {breaks.map((item) => {
          const middle = (item.startMs + item.stopMs) / 2;
          return inWindow(middle) ? (
            <View
              accessibilityLabel={`Break at ${formatTime(item.startMs)}`}
              key={item.id}
              pointerEvents="none"
              role="img"
              style={{
                left: `${at(middle) * 100}%`,
                marginLeft: -1,
                position: "absolute",
                top: STILL_HEIGHT * 0.3,
              }}
              testID="film-strip-break"
            >
              <Surface
                backgroundColor="contentSecondary"
                borderRadius={1}
                borderWidth={0}
                height={STILL_HEIGHT * 0.4}
                width={2}
              />
            </View>
          ) : null;
        })}
      </View>
      {behind && inWindow(nowMs) ? (
        <View
          accessibilityLabel="Live"
          pointerEvents="none"
          role="img"
          style={{ left: `${at(nowMs) * 100}%`, marginLeft: -1, position: "absolute", top: 0 }}
          testID="film-strip-live"
        >
          <Surface
            backgroundColor="stateLive"
            borderRadius={1}
            borderWidth={0}
            height={STILL_HEIGHT + HEAD_ROOM}
            width={2}
          />
          <View style={{ left: 5, position: "absolute", top: HEAD_ROOM + 4, width: 40 }}>
            <Surface
              alignSelf="flex-start"
              backgroundColor="surfaceCanvas"
              borderRadius={4}
              borderWidth={0}
              paddingHorizontal={4}
            >
              <Text textRole="label" tone="live">
                Live
              </Text>
            </Surface>
          </View>
        </View>
      ) : null}
      {inWindow(headMs) ? (
        <View
          pointerEvents="none"
          style={{ left: `${at(headMs) * 100}%`, marginLeft: -1, position: "absolute", top: 0 }}
          testID="film-strip-playhead"
        >
          {/* White over a picture; over a flat tone, the page's own text colour. */}
          {pictured ? <ThemeScope theme="dark">{playhead}</ThemeScope> : playhead}
        </View>
      ) : null}
    </View>
  );
};

export { FilmStrip };
