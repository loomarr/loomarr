import {
  ActivityIndicator,
  ArtworkFrame,
  coverWash,
  decadeTint,
  Surface,
  Text,
  ThemeScope,
  useWindowWidth,
} from "@loomarr/design-system";
import type { ComponentProps, ReactNode } from "react";

import type { CoverFacts } from "./stand-in";

// Transparent layout boxes: Surface without its card look.
const Stack = (props: ComponentProps<typeof Surface>) => (
  <Surface backgroundColor="transparent" borderWidth={0} {...props} />
);
const Line = (props: ComponentProps<typeof Surface>) => (
  <Stack alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline" {...props} />
);

// The Library's page gutter, as in the mock: 16px on a phone, 32px from a tablet up.
const PAGE_GUTTER = { phone: 16, wide: 32 } as const;

// Covers the box it sits in without taking the pointer from the poster.
const OVER = { bottom: 0, left: 0, pointerEvents: "none", position: "absolute", right: 0, top: 0 } as const;

// The words of a drawn cover: the title at the head and the year at the foot, under a hairline in
// the decade's ink. With no year there is no foot. Every word on the cover is also in the caption
// that names the tile, so a speech-input user never reads a word the name lacks. A phone's tiles,
// and a narrow poster such as a collection's pile, take the smaller title.
const CoverWords = ({
  name,
  year,
  compact,
}: Pick<CoverFacts, "year"> & { name: string; compact: boolean }) => {
  const wide = useWindowWidth() >= 768 && !compact;
  const ink = decadeTint(year && Math.floor(year / 10) * 10);
  return (
    <Stack
      {...OVER}
      aria-hidden
      justifyContent="space-between"
      overflow="hidden"
      padding={wide ? 14 : 9}
      testID="library-cover"
    >
      <Text
        numberOfLines={5}
        overflowWrap="anywhere"
        textRole={wide ? "coverTitle" : "coverTitleSmall"}
        textWrap="balance"
        tracking={-0.01}
      >
        {name}
      </Text>
      {year ? (
        <Stack flexShrink={0} gap={wide ? 8 : 5}>
          <Surface backgroundColor={ink} borderRadius={0} borderWidth={0} height={1} opacity={0.35} />
          <Text fontVariant={["tabular-nums"]} textRole="label" tone={ink}>
            {String(year)}
          </Text>
        </Stack>
      ) : null}
    </Stack>
  );
};

// A title's poster (design/library): the shared ArtworkFrame on the artwork-placeholder token, held
// to 2:3. ArtworkFrame fixes a 16:9 ratio, so a 2:3 box sizes it (a frame given both width and
// height ignores its own ratio). With no artwork the poster draws a cover: the title on its decade's
// wash. A cover stands in for artwork, so it takes the dark colours in both themes.
//
// ⚠ The words sit OVER the frame, not in it: `dim` fades the frame, and faded text falls under the
// contrast minimum (axe color-contrast). The dark ground under a dimmed cover keeps it dark on a
// light page. Assistive tech skips the cover, as it skips art; whatever holds the poster names the
// title once.
const LibraryPoster = ({
  name,
  year,
  art,
  small = false,
  compact = false,
  dim = false,
  children,
}: CoverFacts & {
  /** The title, for the drawn cover; empty draws a plain tile. */
  name: string;
  /** The title's artwork, filling the frame; absent draws the cover. */
  art?: ReactNode;
  /** A thumbnail beside a row's words: its cover is the wash alone, with no room for words. */
  small?: boolean;
  /** A narrow poster on any window, such as a collection's pile: its cover takes the smaller title. */
  compact?: boolean;
  dim?: boolean;
  /** Overlays: a badge or a dot in a corner. */
  children?: ReactNode;
}) => {
  const cover = !art && name !== "";
  const frame = (
    <ArtworkFrame borderRadius="control" height="100%" opacity={dim ? 0.55 : 1} state="ready" width="100%">
      {/* A child, even an empty one, keeps the frame from captioning itself "No artwork". */}
      {art ?? (
        <Surface
          backgroundColor={cover ? coverWash(year) : "transparent"}
          borderRadius={0}
          borderWidth={0}
          height="100%"
          width="100%"
        />
      )}
    </ArtworkFrame>
  );
  return (
    <Stack aspectRatio={2 / 3} position="relative" width="100%">
      {cover ? (
        <ThemeScope theme="dark">
          <Surface backgroundColor="surfaceCanvas" borderRadius="control" borderWidth={0} height="100%">
            {frame}
          </Surface>
          <Surface
            {...OVER}
            backgroundColor="transparent"
            borderColor={decadeTint(year && Math.floor(year / 10) * 10)}
            borderRadius="control"
            borderWidth={1}
            opacity={0.18}
          />
          {small ? null : <CoverWords compact={compact} name={name} year={year} />}
        </ThemeScope>
      ) : (
        frame
      )}
      {children}
    </Stack>
  );
};

// The design system's compact twirl, beside words that already say what is getting ready, so it is
// decorative. Its tone is the info hue a preparation ProgressTrack (`tune`) fills with. The testID
// is how a test finds it: a decorative indicator has no role or name.
const Twirl = () => (
  <Stack testID="library-twirl">
    <ActivityIndicator decorative size="compact" tone="info" />
  </Stack>
);

export { LibraryPoster, Line, PAGE_GUTTER, Stack, Twirl };
