import { formatProgrammeTitle } from "@loomarr/core/guide";
import { Surface, Text, ThemeScope, useFocusLook } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { Platform, Pressable, View, type ViewStyle } from "react-native";

import { PlaybarStill } from "../watching-surface/player-chrome/playbar-still";
import type { StripAiring } from "./series-runs";

const STILL_HEIGHT = 84;

type BlockState = "ended" | "later" | "now";

interface StripBlockProps {
  airing: StripAiring;
  /** How much of the block has aired, 0 to 1; only the block on now has any. */
  aired: number;
  artworkUri: string | undefined;
  episodes: number;
  formatTime: (ms: number) => string;
  layout: "row" | "strip";
  left: number;
  onBlur: () => void;
  onFocus: () => void;
  onHoverIn: () => void;
  onHoverOut: () => void;
  onPress: () => void;
  startMs: number;
  state: BlockState;
  stopMs: number;
  width: number;
}

// The browser's own test of keyboard focus, so a click never draws the amber ring. A native view has
// no `matches`, and there focus only ever comes from a remote or a keyboard.
const fromKeyboard = (event: unknown) => {
  const target = (event as { target?: { matches?: (selector: string) => boolean } } | undefined)?.target;
  return target?.matches ? target.matches(":focus-visible") : true;
};

// The ring below is the block's only focus and hover drawing, so the browser's outline is off.
const noOutline = Platform.OS === "web" ? ({ outlineStyle: "none" } as unknown as ViewStyle) : null;

const percent = (ratio: number) => `${ratio * 100}%` as const;

/** Words over a picture are light on ink in both themes. */
const OnInk = ({ children, ink }: { children: ReactNode; ink: boolean }) =>
  ink ? <ThemeScope theme="dark">{children}</ThemeScope> : children;

const StripBlock = ({
  aired,
  airing,
  artworkUri,
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
  width,
}: StripBlockProps) => {
  const [hovered, setHovered] = useState(false);
  const [keyboard, setKeyboard] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook({
    onBlur: () => onBlur(),
    onFocus: (event: unknown) => {
      setKeyboard(fromKeyboard(event));
      onFocus();
    },
  });
  const ring = focused && keyboard ? "actionFocus" : hovered ? "contentPrimary" : "transparent";

  const name = airing.series?.trim() || formatProgrammeTitle(airing);
  const time =
    state === "now"
      ? `Now · until ${formatTime(stopMs)}`
      : state === "later"
        ? formatTime(startMs)
        : undefined;
  const line = [time, episodes > 1 ? `${episodes} episodes` : undefined].filter(Boolean).join(" · ");
  // The accessible name starts with the words on screen, in their order; an ended block shows no
  // time, so its name adds one.
  const spoken = [line, name].filter(Boolean).join(" ");
  const label = state === "ended" ? `${spoken}, ended ${formatTime(stopMs)}` : spoken;

  const inside = layout === "row" || !artworkUri;
  const words = (
    <View style={{ gap: 2, minWidth: 0 }}>
      {line ? (
        <Text numberOfLines={1} textAlign="left" textRole="mono">
          {line}{" "}
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
    </View>
  );

  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole="button"
      {...focusHandlers}
      onHoverIn={() => {
        setHovered(true);
        onHoverIn();
      }}
      onHoverOut={() => {
        setHovered(false);
        onHoverOut();
      }}
      onPress={onPress}
      style={[{ left: percent(left), position: "absolute", top: 0, width: percent(width) }, noOutline]}
      // tabIndex, not focusable: react-native-web ignores focusable on a Pressable.
      tabIndex={0}
      testID={`film-strip-${state}`}
    >
      <View style={{ gap: 6, paddingHorizontal: 1 }}>
        <Surface
          backgroundColor={
            state === "now" ? "stateAiringSurface" : state === "later" ? "surfaceElevated" : "surfaceRaised"
          }
          borderColor={ring}
          borderRadius={8}
          borderWidth={2}
          height={STILL_HEIGHT}
          overflow="hidden"
          // The picture, the dim and the words are placed against this box.
          position="relative"
          testID={ring === "transparent" ? undefined : `film-strip-ring-${ring}`}
        >
          {artworkUri ? (
            <PlaybarStill
              style={{ height: "100%", left: 0, position: "absolute", top: 0, width: "100%" }}
              uri={artworkUri}
            />
          ) : null}
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
            <OnInk ink={Boolean(artworkUri)}>
              <Surface
                backgroundColor={artworkUri ? "artworkScrim" : "transparent"}
                borderRadius={0}
                borderWidth={0}
                bottom={0}
                left={0}
                paddingHorizontal={8}
                paddingVertical={6}
                position="absolute"
                right={0}
                {...(artworkUri ? {} : { justifyContent: "center", top: 0 })}
              >
                {words}
              </Surface>
            </OnInk>
          ) : null}
        </Surface>
        {inside ? null : words}
      </View>
    </Pressable>
  );
};

export type { BlockState };
export { STILL_HEIGHT, StripBlock };
