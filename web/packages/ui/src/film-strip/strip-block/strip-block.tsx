import { formatProgrammeTitle } from "@loomarr/core/guide";
import {
  Layer,
  RingTarget,
  type RingTargetFace,
  Stack,
  Surface,
  Text,
  ThemeScope,
} from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { Platform } from "react-native";

import { PlaybarStill } from "../../watching-surface/player-chrome/playbar-still";
import type { StripAiring } from "../series-runs";

const STILL_HEIGHT = 84;
/** The least a focused or pointed block of a Guide row measures across, for its ring and pointer. */
const MIN_TARGET = 44;

type BlockState = "ended" | "later" | "now";
type Length = number | `${number}%`;

/**
 * What the strip needs of a mounted block, to move focus to it. ⚠ Not React Native's View type:
 * Storybook's docgen follows a prop's types into their imports and cannot parse react-native's source.
 */
interface BlockHandle {
  focus: () => void;
}

interface StripBlockProps {
  airing: StripAiring;
  /** Words that end the block's name, such as its channel. */
  context?: string | undefined;
  /** How much of the block has aired, 0 to 1; only the block on now has any. */
  aired: number;
  artworkUri: string | undefined;
  blockRef: (node: BlockHandle | null) => void;
  /** With a cursor from outside, whether it is on this block; undefined lets focus draw the ring. */
  cursor?: boolean | undefined;
  episodes: number | undefined;
  formatTime: (ms: number) => string;
  layout: "row" | "strip";
  left: Length;
  onBlur: () => void;
  /** `fromKeys` is false when a pointer gave the focus. */
  onFocus: (fromKeys: boolean) => void;
  onHoverIn: () => void;
  onHoverOut: () => void;
  onPress: () => void;
  startMs: number;
  state: BlockState;
  stopMs: number;
  /** Whether this block is the strip's one Tab stop. */
  tabStop: boolean;
  /** Web: a pointer press takes no focus. */
  pointerTakesNoFocus: boolean;
  width: Length;
  /** Where the words start across the block, clear of the playhead. */
  wordsFrom: Length;
  /** Further room in points before the words, for the Live word. */
  wordsInset: number;
}

/**
 * The ring a block draws. With a cursor from outside, only its block may be amber, and only while the
 * keyboard or a remote holds it; otherwise the cursor's ring is the plain one.
 */
const ringOf = (face: RingTargetFace, cursor: boolean | undefined) => {
  if (cursor === undefined) return face.ring;
  if (cursor) return face.keyboard ? "actionFocus" : "contentPrimary";
  return face.keyboard ? "transparent" : face.ring;
};

const percent = (ratio: number) => `${ratio * 100}%` as const;

/** Words over a picture are light on ink in both themes. */
const OnInk = ({ children, ink }: { children: ReactNode; ink: boolean }) =>
  ink ? <ThemeScope theme="dark">{children}</ThemeScope> : children;

const StripBlock = ({
  aired,
  airing,
  artworkUri,
  blockRef,
  context,
  cursor,
  episodes,
  formatTime,
  layout,
  left,
  onBlur,
  onFocus,
  onHoverIn,
  onHoverOut,
  onPress,
  startMs,
  state,
  stopMs,
  tabStop,
  pointerTakesNoFocus,
  width,
  wordsFrom,
  wordsInset,
}: StripBlockProps) => {
  const [hovered, setHovered] = useState(false);
  const [keyboard, setKeyboard] = useState(false);

  const name = airing.series?.trim() || formatProgrammeTitle(airing);
  const time =
    state === "now"
      ? `Now · until ${formatTime(stopMs)}`
      : state === "later"
        ? formatTime(startMs)
        : undefined;
  const line = [time, episodes && episodes > 1 ? `${episodes} episodes` : undefined]
    .filter(Boolean)
    .join(" · ");
  // The accessible name starts with the words on screen, in their order; an ended block shows no
  // time, so its name adds one.
  const spoken = [line, name].filter(Boolean).join(" ");
  const label = [spoken, state === "ended" ? `ended ${formatTime(stopMs)}` : undefined, context]
    .filter(Boolean)
    .join(", ");

  // Under the picture the words stand under the still, with or without one: a break's tick and the
  // playhead cross the still and never the words. A Guide row keeps them inside.
  const inside = layout === "row";
  // An ended block with no picture recedes into the page: canvas fill, quiet edge.
  const receded = state === "ended" && !artworkUri;
  const words = (
    <Stack flexShrink={0} gap="0.5" minWidth={0} position="relative" zIndex={0}>
      {line ? (
        <Text numberOfLines={1} textAlign="left" textRole="mono">
          {line}
          {/* The space is content: without it the two lines read as one word to axe (WCAG 2.5.3). */}{" "}
        </Text>
      ) : null}
      <Text
        numberOfLines={1}
        textAlign="left"
        textRole="cardLabel"
        tone={state === "ended" ? "secondary" : "primary"}
      >
        {name}
      </Text>
    </Stack>
  );

  const face = (given: RingTargetFace) => {
    const ring = ringOf(given, cursor);
    const ringed = ring !== "transparent";
    // A Guide row's block can be a few pixels wide, so its ring is drawn on a target of its own.
    const wideRing = ringed && layout === "row";
    return (
      <>
        <Stack flexShrink={0} gap="1.5" paddingHorizontal={1} position="relative" zIndex={0}>
          <Surface
            backgroundColor={
              receded ? "surfaceCanvas" : state === "now" ? "stateAiringSurface" : "surfaceElevated"
            }
            borderColor={ringed && !wideRing ? ring : receded ? "borderDecorative" : "transparent"}
            borderRadius={8}
            borderWidth={2}
            height={STILL_HEIGHT}
            overflow="hidden"
            // The picture, the dim and the words are placed against this box.
            position="relative"
            testID={
              ringed && !wideRing ? `film-strip-ring-${ring}` : receded ? "film-strip-receded" : undefined
            }
          >
            {artworkUri ? <PlaybarStill fill uri={artworkUri} /> : null}
            {artworkUri && state === "ended" ? (
              <Surface
                backgroundColor="artworkScrim"
                borderRadius={0}
                borderWidth={0}
                bottom={0}
                left={0}
                position="absolute"
                right={0}
                testID="film-strip-ended-dim"
                top={0}
              />
            ) : null}
            {aired > 0 ? (
              <Surface
                // Over a picture the dim is ink; over a flat tone it is a step toward the page.
                backgroundColor={artworkUri ? "artworkScrim" : "surfaceCanvas"}
                borderRadius={0}
                borderWidth={0}
                bottom={0}
                left={0}
                opacity={artworkUri ? 1 : 0.6}
                position="absolute"
                testID="film-strip-aired"
                top={0}
                width={percent(aired)}
              />
            ) : null}
            {inside ? (
              <Layer
                bottom={0}
                left={wordsFrom}
                right={0}
                testID="film-strip-words"
                {...(artworkUri ? {} : { top: 0 })}
              >
                <Stack paddingLeft={wordsInset} {...(artworkUri ? {} : { height: "100%" })}>
                  <OnInk ink={Boolean(artworkUri)}>
                    <Surface
                      backgroundColor={artworkUri ? "artworkScrim" : "transparent"}
                      borderRadius={0}
                      borderWidth={0}
                      paddingHorizontal={8}
                      paddingVertical={6}
                      {...(artworkUri ? {} : { height: "100%", justifyContent: "center" })}
                    >
                      {words}
                    </Surface>
                  </OnInk>
                </Stack>
              </Layer>
            ) : null}
          </Surface>
          {inside ? null : words}
        </Stack>
        {wideRing ? (
          // Centred on the block and above its neighbours; the drawn width stays as it is.
          <Layer
            centred
            height={STILL_HEIGHT}
            minWidth={MIN_TARGET}
            testID="film-strip-target"
            top={0}
            width="100%"
          >
            <Surface
              backgroundColor="transparent"
              borderColor={ring}
              borderRadius={8}
              borderWidth={2}
              height="100%"
              testID={`film-strip-ring-${ring}`}
            />
          </Layer>
        ) : null}
      </>
    );
  };

  return (
    // The layer places the block; the button fills it.
    <Layer left={left} top={0} width={width} zIndex={cursor || hovered || keyboard ? 1 : 0}>
      <RingTarget
        accessibilityLabel={label}
        onBlur={onBlur}
        onFocus={onFocus}
        onHoverIn={() => {
          setHovered(true);
          onHoverIn();
        }}
        onHoverOut={() => {
          setHovered(false);
          onHoverOut();
        }}
        onKeyboardFocus={setKeyboard}
        onPress={onPress}
        pointerTakesNoFocus={pointerTakesNoFocus}
        ref={blockRef}
        // The roving Tab stop is the web's: a television's remote has to reach every block, and a
        // native view with a negative tabIndex takes no focus.
        tabIndex={Platform.OS !== "web" || tabStop ? 0 : -1}
        testID={`film-strip-${state}`}
      >
        {face}
      </RingTarget>
    </Layer>
  );
};

export type { BlockHandle, BlockState, Length };
export { STILL_HEIGHT, StripBlock };
