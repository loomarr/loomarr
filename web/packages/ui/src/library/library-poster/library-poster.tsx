import { ActivityIndicator, ArtworkFrame, Surface, Text } from "@loomarr/design-system";
import type { ComponentProps, ReactNode } from "react";

import { posterMonogram, posterTone } from "./stand-in";

// Transparent layout boxes: Surface without its card look.
const Stack = (props: ComponentProps<typeof Surface>) => (
  <Surface backgroundColor="transparent" borderWidth={0} {...props} />
);
const Line = (props: ComponentProps<typeof Surface>) => (
  <Stack alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline" {...props} />
);

// The Library's page gutter, as in the mock: 16px on a phone, 32px from a tablet up.
const PAGE_GUTTER = { phone: 16, wide: 32 } as const;

// A title's poster (design/library): the shared ArtworkFrame on the artwork-placeholder token, held
// to 2:3. ArtworkFrame fixes a 16:9 ratio, so a 2:3 box sizes it (a frame given both width and
// height ignores its own ratio). With no artwork the frame holds the title's colour field, under its
// monogram.
//
// ⚠ The monogram sits OVER the frame, not in it: `dim` fades the frame, and faded text falls under
// the contrast minimum (axe color-contrast). It stands in for artwork, so assistive tech skips it,
// as it skips art; whatever holds the poster names the title once.
const LibraryPoster = ({
  name,
  id,
  art,
  big = false,
  small = false,
  dim = false,
  children,
}: {
  /** The title, for the stand-in's monogram; empty draws a plain tile. */
  name: string;
  /** Picks the stand-in's colour; absent picks it from the name. */
  id?: string;
  /** The title's artwork, filling the frame; absent draws the stand-in. */
  art?: ReactNode;
  big?: boolean;
  /** A thumbnail beside a row's words. */
  small?: boolean;
  dim?: boolean;
  /** Overlays: a badge or a dot in a corner. */
  children?: ReactNode;
}) => {
  const monogram = art ? "" : posterMonogram(name);
  const seed = id ?? name;
  return (
    <Stack aspectRatio={2 / 3} position="relative" width="100%">
      <ArtworkFrame
        borderRadius="control"
        height="100%"
        justifyContent="center"
        opacity={dim ? 0.55 : 1}
        state="ready"
        width="100%"
      >
        {/* A child, even an empty one, keeps the frame from captioning itself "No artwork". */}
        {art ?? (
          <Surface
            backgroundColor={seed ? posterTone(seed) : "transparent"}
            borderRadius={0}
            borderWidth={0}
            height="100%"
            width="100%"
          />
        )}
      </ArtworkFrame>
      {monogram ? (
        <Stack
          alignItems="center"
          aria-hidden
          bottom={0}
          justifyContent="center"
          left={0}
          pointerEvents="none"
          position="absolute"
          right={0}
          top={0}
        >
          <Text textRole={small ? "label" : big ? "display" : "headline"} tone="secondary">
            {monogram}
          </Text>
        </Stack>
      ) : null}
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
