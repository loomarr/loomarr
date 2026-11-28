import { Layer, RingTarget, Stack, Surface, Text, ThemeScope } from "@loomarr/design-system";
import { useState } from "react";
import { Platform } from "react-native";

import { groupSeriesRuns, type StripAiring, StripStill } from "../../film-strip";
import type { GuideViewBlock, GuideViewEntry } from "../guide-view.type";

/** The least a focused or pointed block measures across, for its ring and pointer. */
const MIN_TARGET = 44;

/** What a row needs of a mounted block, to move focus to it. */
interface RowHandle {
  focus: () => void;
}

interface RowProps {
  channel: GuideViewEntry;
  artworkUri: (airing: StripAiring) => string | undefined;
  blockRef: (programmeId: string) => (node: RowHandle | null) => void;
  /** The programme the cursor is on, when it is in this row. */
  cursorId: string | undefined;
  formatTime: (ms: number) => string;
  height: number;
  nowMs: number;
  onPress: (programmeId: string) => void;
  startMs: number;
  stopMs: number;
}

const Block = ({
  artwork,
  block,
  blockRef,
  context,
  cursor,
  formatTime,
  height,
  nowMs,
  onPress,
  startMs,
  stopMs,
}: {
  artwork: string | undefined;
  block: GuideViewBlock;
  blockRef: (node: RowHandle | null) => void;
  /** The channel, which ends the block's name. */
  context: string;
  cursor: boolean;
  formatTime: (ms: number) => string;
  height: number;
  nowMs: number;
  onPress: () => void;
  startMs: number;
  stopMs: number;
}) => {
  const [hovered, setHovered] = useState(false);
  const [keyboard, setKeyboard] = useState(false);
  const span = stopMs - startMs;
  const from = Math.max(block.startMs, startMs);
  const to = Math.min(block.stopMs, stopMs);
  const ended = block.stopMs <= nowMs;
  const later = block.startMs > nowMs;
  const on = !ended && !later;
  // A run cut by the span's edge may go on out of sight, so it says no count.
  const cut = block.startMs < startMs || block.stopMs > stopMs;
  const count = !cut && block.episodes && block.episodes > 1 ? `${block.episodes} episodes` : undefined;
  const time = later ? formatTime(block.startMs) : undefined;
  const words = [time, block.title, count].filter(Boolean).join(" ");
  const label = ended
    ? `${words}, ended ${formatTime(block.stopMs)}`
    : on
      ? `${words}, on now until ${formatTime(block.stopMs)}`
      : words;
  const picture = on ? artwork : undefined;
  const aired = on && to > from ? Math.max(0, Math.min(1, (nowMs - from) / (to - from))) : 0;

  const text = (
    <Stack align="center" direction="row" gap="1.5" height="100%" minWidth={0} paddingHorizontal={8}>
      {time ? (
        <Text numberOfLines={1} textRole="mono" tone="secondary">
          {time}
          {/* The spaces are content: without them the words read as one to axe (WCAG 2.5.3). */}{" "}
        </Text>
      ) : null}
      <Text
        flexShrink={1}
        minWidth={0}
        numberOfLines={1}
        textRole="cardLabel"
        tone={ended ? "secondary" : "primary"}
      >
        {block.title}
        {count ? " " : null}
      </Text>
      {count ? (
        <Text numberOfLines={1} textRole="mono" tone="secondary">
          {count}
        </Text>
      ) : null}
    </Stack>
  );

  return (
    <Layer
      left={`${((from - startMs) / span) * 100}%`}
      top={0}
      width={`${((to - from) / span) * 100}%`}
      zIndex={cursor || hovered || keyboard ? 1 : 0}
    >
      <RingTarget
        // The name leads with the words on screen (WCAG 2.5.3), and ends with whose they are.
        accessibilityLabel={`${label}, ${context}`}
        onHoverIn={() => setHovered(true)}
        onHoverOut={() => setHovered(false)}
        onKeyboardFocus={setKeyboard}
        onPress={onPress}
        ref={blockRef}
        // The web's one Tab stop is the cursor; a television's remote has to reach every block.
        tabIndex={Platform.OS !== "web" || cursor ? 0 : -1}
        testID={`guide-view-${ended ? "ended" : later ? "later" : "now"}`}
      >
        {(face) => {
          // Amber is the keys' alone; a cursor a pointer set, or one left behind, draws the plain ring.
          const ring = face.keyboard ? "actionFocus" : cursor ? "contentPrimary" : face.ring;
          return (
            <>
              <Stack paddingHorizontal={1}>
                <Surface
                  backgroundColor={ended ? "surfaceCanvas" : on ? "stateAiringSurface" : "surfaceElevated"}
                  borderColor={ended ? "borderDecorative" : "transparent"}
                  borderRadius={4}
                  borderWidth={1}
                  height={height}
                  overflow="hidden"
                  position="relative"
                >
                  {picture ? <StripStill fill uri={picture} /> : null}
                  {aired > 0 ? (
                    <Surface
                      backgroundColor={picture ? "artworkScrim" : "surfaceCanvas"}
                      borderRadius={0}
                      borderWidth={0}
                      bottom={0}
                      left={0}
                      opacity={picture ? 1 : 0.6}
                      position="absolute"
                      testID="guide-view-aired"
                      top={0}
                      width={`${aired * 100}%`}
                    />
                  ) : null}
                  <Layer fill>
                    {picture ? (
                      <ThemeScope theme="dark">
                        <Surface
                          backgroundColor="artworkScrim"
                          borderRadius={0}
                          borderWidth={0}
                          height="100%"
                        >
                          {text}
                        </Surface>
                      </ThemeScope>
                    ) : (
                      text
                    )}
                  </Layer>
                </Surface>
              </Stack>
              <Layer centred height={height} minWidth={MIN_TARGET} top={0} width="100%">
                <Surface
                  backgroundColor="transparent"
                  borderColor={ring}
                  borderRadius={4}
                  borderWidth={2}
                  height="100%"
                  testID={`guide-view-ring-${ring}`}
                />
              </Layer>
            </>
          );
        }}
      </RingTarget>
    </Layer>
  );
};

/** A compact row: one channel's blocks on the shared clock, with its breaks as thin ticks. */
const Row = ({
  channel,
  artworkUri,
  blockRef,
  cursorId,
  formatTime,
  height,
  nowMs,
  onPress,
  startMs,
  stopMs,
}: RowProps) => {
  const drawn = channel.programmes.filter((p) => p.stopMs > startMs && p.startMs < stopMs);
  const breaks = groupSeriesRuns(channel.airings).filter(
    (item) => item.type === "break" && item.stopMs > startMs && item.startMs < stopMs,
  );
  return (
    <Layer height={height} inFlow width="100%">
      {drawn.length ? (
        drawn.map((block) => (
          <Block
            artwork={artworkUri(
              block.airings.find((a) => a.startMs <= nowMs && nowMs < a.stopMs) ??
                (block.airings[0] as StripAiring),
            )}
            block={block}
            blockRef={blockRef(block.id)}
            context={`channel ${channel.number}, ${channel.name}`}
            cursor={block.id === cursorId}
            formatTime={formatTime}
            height={height}
            key={block.id}
            nowMs={nowMs}
            onPress={() => onPress(block.id)}
            startMs={startMs}
            stopMs={stopMs}
          />
        ))
      ) : (
        <Stack height="100%" justify="center" paddingHorizontal={8}>
          <Text textRole="label" tone="secondary">
            Nothing scheduled
          </Text>
        </Stack>
      )}
      {breaks.map((item) => {
        const middle = (item.startMs + item.stopMs) / 2;
        return middle >= startMs && middle < stopMs ? (
          <Layer
            aria-label={`Break at ${formatTime(item.startMs)}`}
            key={item.id}
            left={`${((middle - startMs) / (stopMs - startMs)) * 100}%`}
            marginLeft={-1}
            pointerEvents="none"
            role="img"
            top={height * 0.3}
            zIndex={2}
          >
            <Surface
              backgroundColor="contentSecondary"
              borderRadius={1}
              borderWidth={0}
              height={height * 0.4}
              width={2}
            />
          </Layer>
        ) : null;
      })}
    </Layer>
  );
};

export type { RowHandle };
export { Row };
