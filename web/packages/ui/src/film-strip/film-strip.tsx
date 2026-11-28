import { Layer, Stack, Surface, Text, ThemeScope } from "@loomarr/design-system";
import { type ComponentRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import {
  type LayoutChangeEvent,
  type NativeScrollEvent,
  type NativeSyntheticEvent,
  ScrollView,
} from "react-native";

import type { FilmStripLook, FilmStripProps } from "./film-strip.type";
import { groupSeriesRuns, type StripAiring, type StripItem, type StripProgramme } from "./series-runs";
import { type BlockHandle, type BlockState, type Length, STILL_HEIGHT, StripBlock } from "./strip-block";

// The strip's height beside its stills: the two lines of words under them.
const CAPTION_HEIGHT = 48;
/** The room above the stills for the playhead's head. */
const HEAD_ROOM = 6;
/** Under the picture, a minute's width before the two limits apply. */
const PX_PER_MINUTE = 3.1;
/** Under the picture, the room a break between two blocks stands in. */
const BREAK_SLOT = 12;
/** The trailing fade that says the strip goes on. */
const FADE = 90;
/** The room the Live word needs after its mark, as a width and as a share of a Guide row. */
const LIVE_ROOM = 48;
const LIVE_ROOM_RATIO = 0.88;
/** In a Guide row the words start after the playhead when this much of the programme is left. */
const ROOMY_MS = 24 * 60_000;

const defaultArtwork = (airing: StripAiring) => airing.thumbUrl || airing.backdropImage?.src || undefined;

const clamp = (value: number, min = 0, max = 1) => Math.min(max, Math.max(min, value));

const lookOf = (item: StripProgramme, nowMs: number): FilmStripLook => ({
  airing:
    item.airings.find((a) => a.startMs <= nowMs && nowMs < a.stopMs) ?? (item.airings[0] as StripAiring),
  episodes: item.episodes,
  startMs: item.startMs,
  stopMs: item.stopMs,
});

const stateOf = (item: StripProgramme, nowMs: number): BlockState =>
  item.stopMs <= nowMs ? "ended" : item.startMs > nowMs ? "later" : "now";

/** An item's place across the strip, and the span of time that place stands for. */
interface Placed {
  fromMs: number;
  item: StripItem;
  left: number;
  toMs: number;
  width: number;
}

/**
 * Under the picture the strip is not a clock: each block is as wide as its length allows between the
 * two limits, laid end to end, and a break between two of them takes a slot of its own.
 */
const placeStrip = (
  items: readonly StripItem[],
  startMs: number,
  limits: { max: number; min: number },
): { placed: Placed[]; width: number } => {
  let x = 0;
  let blockEndMs = startMs;
  const placed = items.map((item): Placed => {
    const fromMs = Math.max(item.startMs, startMs);
    if (item.type === "break" && item.startMs < blockEndMs) {
      // Inside a run: the tick stands over the run's block, which is placed already.
      return { fromMs, item, left: Number.NaN, toMs: item.stopMs, width: 0 };
    }
    const width =
      item.type === "break"
        ? BREAK_SLOT
        : clamp(((item.stopMs - fromMs) / 60_000) * PX_PER_MINUTE, limits.min, limits.max);
    const at = { fromMs, item, left: x, toMs: item.stopMs, width };
    x += width;
    if (item.type === "programme") blockEndMs = item.stopMs;
    return at;
  });
  return { placed, width: x };
};

/**
 * One channel's day across (frontend-design §9.2, The Cross): the same part under the picture in a
 * desktop window and as the tuned channel's row in the Guide. It is one Tab stop on the web; Left
 * and Right move between blocks, and Home and End go to the first and last.
 */
const FilmStrip = ({
  airings,
  artworkUri = defaultArtwork,
  blockContext,
  cursor,
  formatTime,
  layout = "strip",
  maxBlockWidth = 330,
  minBlockWidth = 150,
  nowMs,
  onLook,
  onSelect,
  playheadMs = nowMs,
  pointerTakesNoFocus = false,
  ref,
  window,
}: FilmStripProps) => {
  // Focus outlasts a pointer passing over: leaving a block returns the look to the focused one.
  const focusedLook = useRef<FilmStripLook | undefined>(undefined);
  const blocks = useRef(new Map<string, BlockHandle>());
  const [stopId, setStopId] = useState<string | undefined>(undefined);
  const [viewWidth, setViewWidth] = useState(0);
  const [scrolled, setScrolled] = useState(0);
  const scroller = useRef<ComponentRef<typeof ScrollView>>(null);

  const isRow = layout === "row";
  const items = groupSeriesRuns(airings).filter(
    (i) => i.stopMs > window.startMs && i.startMs < window.stopMs,
  );
  const programmes = items.filter((i): i is StripProgramme => i.type === "programme");
  const strip = placeStrip(items, window.startMs, { max: maxBlockWidth, min: minBlockWidth });
  const span = Math.max(1, window.stopMs - window.startMs);
  const ratio = (ms: number) => clamp((ms - window.startMs) / span);

  // Where an instant falls across the strip: a share of a Guide row, points under the picture.
  const xAt = (ms: number): Length | undefined => {
    if (isRow) return ms >= window.startMs && ms < window.stopMs ? `${ratio(ms) * 100}%` : undefined;
    const at = strip.placed.find((p) => p.width > 0 && ms >= p.fromMs && ms < p.toMs);
    return at ? at.left + ((ms - at.fromMs) / (at.toMs - at.fromMs)) * at.width : undefined;
  };

  const pictureOf = (item: StripProgramme) => artworkUri(lookOf(item, nowMs).airing);
  const headMs = Math.min(playheadMs, nowMs);
  const behind = nowMs - headMs >= 1000;
  const headX = xAt(headMs);
  const liveX = xAt(nowMs);
  const under = programmes.find((i) => i.startMs <= headMs && headMs < i.stopMs);
  const liveLeads =
    typeof liveX === "number" ? liveX + LIVE_ROOM > strip.width : ratio(nowMs) > LIVE_ROOM_RATIO;
  // Under the picture the words always stand under the stills, so no tick or playhead crosses them.
  const height = HEAD_ROOM + STILL_HEIGHT + (isRow ? 0 : CAPTION_HEIGHT);

  const cursorId = cursor?.id;
  const takeFocus = cursor?.takeFocus;
  useEffect(() => {
    // A new cursor alone never takes focus from elsewhere.
    if (cursorId && (takeFocus || focusedLook.current)) blocks.current.get(cursorId)?.focus();
  }, [cursorId, takeFocus]);

  const stop =
    programmes.find((i) => i.id === cursorId) ??
    programmes.find((i) => i.id === stopId) ??
    programmes.find((i) => stateOf(i, nowMs) === "now") ??
    programmes[0];

  // The browser brings a focused block only as far as the edge, which the fade covers; scroll on
  // until it stands clear. The track ends with the fade's width of padding, so the last block can too.
  useImperativeHandle(ref, () => ({
    focusFromNow: (by) => {
      const now = programmes.findIndex((i) => stateOf(i, nowMs) === "now");
      const next = programmes[clamp(Math.max(now, 0) + by, 0, programmes.length - 1)];
      if (next) blocks.current.get(next.id)?.focus();
    },
  }));

  const reveal = (id: string) => {
    const at = strip.placed.find((p) => p.item.id === id);
    if (isRow || !at || viewWidth === 0) return;
    const least = Math.max(0, at.left + at.width + FADE - viewWidth);
    scroller.current?.scrollTo?.({ animated: false, x: Math.min(Math.max(scrolled, least), at.left) });
  };

  const onKeyDown = (event: {
    altKey: boolean;
    ctrlKey: boolean;
    key: string;
    metaKey: boolean;
    preventDefault: () => void;
  }) => {
    // With a cursor from outside, the keys bubble to the owner.
    if (cursor || event.altKey || event.ctrlKey || event.metaKey || !stop) return;
    const here = programmes.indexOf(stop);
    const to = { ArrowLeft: here - 1, ArrowRight: here + 1, End: programmes.length - 1, Home: 0 }[event.key];
    if (to === undefined) return;
    event.preventDefault();
    const next = programmes[clamp(to, 0, programmes.length - 1)];
    if (next) blocks.current.get(next.id)?.focus();
  };

  const playhead = (
    <Surface
      backgroundColor="contentPrimary"
      borderRadius={1}
      borderWidth={0}
      height={STILL_HEIGHT + HEAD_ROOM}
      width={2}
    />
  );

  const track = (
    <Layer height={height} inFlow onKeyDown={onKeyDown} width={isRow ? "100%" : strip.width + FADE}>
      <Layer bottom={0} left={0} right={0} top={HEAD_ROOM}>
        {strip.placed.map(({ fromMs, item, left, width }) => {
          if (item.type === "break") {
            const middle = (item.startMs + item.stopMs) / 2;
            const x = !isRow && width > 0 ? left + width / 2 : xAt(middle);
            return x === undefined ? null : (
              <Layer
                aria-label={`Break at ${formatTime(item.startMs)}`}
                key={item.id}
                left={x}
                marginLeft={-1}
                pointerEvents="none"
                role="img"
                testID="film-strip-break"
                top={STILL_HEIGHT * 0.3}
                zIndex={2}
              >
                <Surface
                  backgroundColor="contentSecondary"
                  borderRadius={1}
                  borderWidth={0}
                  height={STILL_HEIGHT * 0.4}
                  width={2}
                />
              </Layer>
            );
          }
          const state = stateOf(item, nowMs);
          const look = lookOf(item, nowMs);
          const shownFrom = isRow ? Math.max(item.startMs, window.startMs) : fromMs;
          const shownTo = isRow ? Math.min(item.stopMs, window.stopMs) : item.stopMs;
          const aired = state === "now" ? clamp((nowMs - shownFrom) / (shownTo - shownFrom)) : 0;
          const roomy = isRow && item.stopMs - nowMs >= ROOMY_MS;
          // Behind live, the Live word stands just after the aired part, where the words would start.
          const pastLive = roomy && aired > 0 && behind && !liveLeads;
          return (
            <StripBlock
              aired={aired}
              airing={look.airing}
              artworkUri={artworkUri(look.airing)}
              context={blockContext}
              cursor={cursor ? item.id === cursorId : undefined}
              blockRef={(node) => {
                if (node) blocks.current.set(item.id, node);
                else blocks.current.delete(item.id);
              }}
              episodes={item.episodes}
              formatTime={formatTime}
              key={item.id}
              layout={layout}
              left={isRow ? `${ratio(item.startMs) * 100}%` : left}
              onBlur={() => {
                focusedLook.current = undefined;
                onLook?.(undefined);
              }}
              onFocus={(fromKeys) => {
                setStopId(item.id);
                focusedLook.current = look;
                onLook?.(look);
                // A click must land on what was under the pointer, so only the keys scroll.
                if (fromKeys) reveal(item.id);
              }}
              onHoverIn={() => onLook?.(look)}
              onHoverOut={() => onLook?.(focusedLook.current)}
              onPress={() => onSelect?.(look)}
              startMs={item.startMs}
              state={state}
              stopMs={item.stopMs}
              pointerTakesNoFocus={pointerTakesNoFocus}
              tabStop={item.id === stop?.id}
              width={isRow ? `${(ratio(item.stopMs) - ratio(item.startMs)) * 100}%` : width}
              wordsFrom={roomy && aired > 0 ? `${aired * 100}%` : 0}
              wordsInset={pastLive ? LIVE_ROOM : 0}
            />
          );
        })}
      </Layer>
      {behind && liveX !== undefined ? (
        <Layer
          aria-label="Live"
          left={liveX}
          marginLeft={-1}
          pointerEvents="none"
          role="img"
          testID="film-strip-live"
          top={0}
          width={2}
          zIndex={3}
        >
          <Surface
            backgroundColor="stateLive"
            borderRadius={1}
            borderWidth={0}
            height={STILL_HEIGHT + HEAD_ROOM}
            width={2}
          />
          <Layer
            testID={`film-strip-live-${liveLeads ? "leading" : "trailing"}`}
            top={HEAD_ROOM + 4}
            width={40}
            {...(liveLeads ? { right: 5 } : { left: 5 })}
          >
            <Stack align={liveLeads ? "end" : "start"}>
              <Surface backgroundColor="surfaceCanvas" borderRadius={4} borderWidth={0} paddingHorizontal={4}>
                <Text textRole="label" tone="live">
                  Live
                </Text>
              </Surface>
            </Stack>
          </Layer>
        </Layer>
      ) : null}
      {headX !== undefined ? (
        <Layer
          left={headX}
          marginLeft={-1}
          pointerEvents="none"
          testID="film-strip-playhead"
          top={0}
          zIndex={3}
        >
          {/* White over a picture; over a flat tone, the page's own text colour. */}
          {under && pictureOf(under) ? <ThemeScope theme="dark">{playhead}</ThemeScope> : playhead}
        </Layer>
      ) : null}
    </Layer>
  );

  const more = viewWidth > 0 && strip.width - scrolled - viewWidth > 1;
  // Scrolled along from its start, the leading edge fades too, so it never cuts words hard.
  const along = scrolled > 1;

  return (
    <Layer
      aria-label="Schedule"
      inFlow
      onLayout={(e: LayoutChangeEvent) => setViewWidth(e.nativeEvent.layout.width)}
      role="group"
      width="100%"
      // The fade says the strip goes on past its trailing edge.
      {...(!isRow && more ? { trailingFade: FADE } : {})}
      {...(!isRow && along ? { leadingFade: FADE } : {})}
    >
      {isRow ? (
        track
      ) : (
        // A scroller, so focus past the trailing edge brings its block into view.
        <ScrollView
          horizontal
          ref={scroller}
          onScroll={(e: NativeSyntheticEvent<NativeScrollEvent>) =>
            setScrolled(e.nativeEvent.contentOffset.x)
          }
          scrollEventThrottle={16}
          showsHorizontalScrollIndicator={false}
        >
          {track}
        </ScrollView>
      )}
    </Layer>
  );
};

export { PlaybarStill as StripStill } from "../watching-surface/player-chrome/playbar-still";
export { FilmStrip };
