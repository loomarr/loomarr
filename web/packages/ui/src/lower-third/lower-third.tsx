import { decadeTint, Icon, icons, Surface, Text, VisuallyHidden } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { PictureInk } from "../picture-ink";
import type { QuietRowDevice } from "../quiet-row";
import { RingedControl } from "../ringed-control";
import { Box, Line } from "./layout";
import { behindWords, type LowerThirdState, summaryOf } from "./summary";

interface LowerThirdProps {
  channelName: string;
  channelNumber: number;
  /** The decade the watched channel plays, 1980 for the 1980s. */
  decade?: number;
  /** `full` is a desktop at full screen, `window` a desktop window, `portrait` an upright phone. */
  device: QuietRowDevice;
  onGoLive?: () => void;
  /** An upright phone's More. */
  onMore?: () => void;
  /** An upright phone's Pause, or Play while paused. */
  onTogglePause?: () => void;
  state: LowerThirdState;
}

// The numeral and title roles per device (frontend-design §2.2 and §9.2).
const TYPE = {
  full: { density: "pointer", numeral: "thirdNumeral", title: "thirdTitle" },
  landscape: { density: "touch", numeral: "thirdNumeralSide", title: "thirdTitle" },
  portrait: { density: "touch", numeral: "thirdNumeral", title: "thirdTitle" },
  tv: { density: "tv", numeral: "thirdNumeral", title: "thirdTitle" },
  window: { density: "pointer", numeral: "thirdNumeralWindow", title: "thirdTitleWindow" },
} as const;

const Dot = () => (
  <Surface aria-hidden backgroundColor="stateLive" borderRadius={4} borderWidth={0} height={7} width={7} />
);

const Meta = ({ children, tone = "secondary" }: { children: ReactNode; tone?: "info" | "secondary" }) => (
  <Text flexShrink={1} minWidth={0} numberOfLines={1} textRole="compact" tone={tone}>
    {children}
  </Text>
);

const joined = (...parts: (string | undefined)[]) => parts.filter(Boolean).join(" · ");

/**
 * The lower third of The Cross (frontend-design §9.2, target): the channel's numeral, the title, one
 * line of facts and what is next, as type with no box. A screen reader hears one summary; only the
 * controls are read on their own.
 */
const LowerThird = ({
  channelName,
  channelNumber,
  decade,
  device,
  onGoLive,
  onMore,
  onTogglePause,
  state,
}: LowerThirdProps) => {
  const portrait = device === "portrait";
  const touch = portrait || device === "landscape";
  const typing = state.kind === "typing";
  const behind = state.kind === "on" && !state.paused ? (state.behindSeconds ?? 0) : 0;
  const paused = state.kind === "on" && Boolean(state.paused);

  const title =
    state.kind === "typing"
      ? (state.target?.name ?? "No such channel")
      : state.kind === "on" || state.kind === "break"
        ? state.title
        : channelName;

  const facts = (): ReactNode => {
    switch (state.kind) {
      case "typing":
        return (
          <Meta>
            {state.target
              ? joined(state.target.onNow, "press Enter, or wait")
              : `There is no channel ${state.digits}`}
          </Meta>
        );
      case "tuning":
        return <Meta>Tuning in… Starting the channel</Meta>;
      case "nothing":
        return <Meta>Nothing scheduled</Meta>;
      case "break":
        return (
          <>
            <Meta>{channelName} ·</Meta>
            <Meta tone="info">Back in {state.backInMinutes} min</Meta>
          </>
        );
      case "on": {
        if (behind > 0) return <Meta>{behindWords(behind)}</Meta>;
        // An upright phone's line is short, so the rating waits in the programme's facts.
        const where = joined(channelName, state.episode, portrait ? undefined : state.rating);
        if (state.paused) return <Meta>{joined(where, "Paused")}</Meta>;
        return (
          <>
            <Dot />
            <Meta>{joined(where, `${state.minutesLeft} min left`)}</Meta>
          </>
        );
      }
    }
  };

  const next = "next" in state ? state.next : undefined;
  const goLive = behind > 0 && device !== "tv";

  const body = (
    <Box
      alignItems="center"
      aria-label={`Channel ${channelNumber}, ${channelName}`}
      flexDirection="row"
      gap={20}
      maxWidth={640}
      paddingHorizontal={16}
      role="group"
    >
      <VisuallyHidden>
        <Text textRole="body">{summaryOf(channelNumber, channelName, state)}</Text>
      </VisuallyHidden>
      <Box alignItems="flex-end" aria-hidden minWidth={44}>
        <Text
          density={TYPE[device].density}
          testID="lower-third-numeral"
          textRole={TYPE[device].numeral}
          tone={typing ? "signal" : decadeTint(decade)}
        >
          {state.kind === "typing" ? state.digits : channelNumber}
        </Text>
        {typing ? (
          <Surface
            alignSelf="stretch"
            backgroundColor="actionPrimary"
            borderRadius={0}
            borderWidth={0}
            height={3}
            testID="lower-third-typing-rule"
          />
        ) : null}
      </Box>
      <Box flexShrink={1} minWidth={0}>
        <Box aria-hidden>
          <Text
            density={TYPE[device].density}
            numberOfLines={portrait ? 1 : 2}
            testID="lower-third-title"
            textRole={TYPE[device].title}
            tone="primary"
          >
            {title}
          </Text>
        </Box>
        <Line>
          {goLive ? (
            // The target is a touch's height while the line keeps its own.
            <Box marginVertical={touch ? -8 : 0}>
              <RingedControl
                accessibilityLabel="Go live"
                faceHeight={28}
                onPress={onGoLive}
                size={touch ? 44 : 28}
              >
                <Dot />
                <Text textRole="label" tone="live">
                  Go live
                </Text>
              </RingedControl>
            </Box>
          ) : null}
          <Box alignItems="center" aria-hidden flexDirection="row" flexShrink={1} gap={6} minWidth={0}>
            {facts()}
          </Box>
        </Line>
        {next ? (
          <Line aria-hidden>
            <Text textRole="compact" tone="secondary">
              Next
            </Text>
            <Text textRole="mono" tone="secondary">
              {next.startsAt}
            </Text>
            <Text flexShrink={1} minWidth={0} numberOfLines={1} textRole="compact" tone="primary">
              {next.title}
            </Text>
          </Line>
        ) : null}
      </Box>
      {portrait ? (
        <Box flexDirection="row">
          <RingedControl accessibilityLabel={paused ? "Play" : "Pause"} onPress={onTogglePause} size={44}>
            <Icon decorative glyph={icons[paused ? "play" : "pause"]} size="touch" tone="content" />
          </RingedControl>
          <RingedControl accessibilityLabel="More" onPress={onMore} size={44}>
            <Icon decorative glyph={icons.more} size="touch" tone="content" />
          </RingedControl>
        </Box>
      ) : null}
    </Box>
  );

  return device === "full" || device === "tv" || device === "landscape" ? (
    <PictureInk edge="side">{body}</PictureInk>
  ) : (
    body
  );
};

export type { LowerThirdProps };
export { LowerThird };
