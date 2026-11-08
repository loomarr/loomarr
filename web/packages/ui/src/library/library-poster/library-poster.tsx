import { ArtworkFrame, Surface, Text } from "@loomarr/design-system";
import type { ComponentProps, ReactNode } from "react";

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
// height ignores its own ratio). With no artwork the title is set in the frame.
//
// ⚠ The set name sits OVER the frame, not in it: `dim` fades the frame, and faded text fell to
// 2.64:1 (axe color-contrast). It stands in for artwork, so assistive tech skips it, as it skips
// art; whatever holds the poster names the title once.
const LibraryPoster = ({
  name,
  art,
  big = false,
  dim = false,
  children,
}: {
  name: string;
  /** The title's artwork, filling the frame; absent draws the name instead. */
  art?: ReactNode;
  big?: boolean;
  dim?: boolean;
  /** Overlays: a badge or a dot in a corner. */
  children?: ReactNode;
}) => (
  <Stack aspectRatio={2 / 3} position="relative" width="100%">
    <ArtworkFrame
      borderRadius="control"
      height="100%"
      justifyContent="center"
      opacity={dim ? 0.55 : 1}
      state="ready"
      width="100%"
    >
      {/* An empty child keeps the frame from captioning itself "No artwork" under the name. */}
      {art ?? <Stack />}
    </ArtworkFrame>
    {art ? null : (
      <Stack
        aria-hidden
        bottom={0}
        justifyContent="flex-end"
        left={0}
        padding={big ? "section" : "control"}
        pointerEvents="none"
        position="absolute"
        right={0}
        top={0}
      >
        <Text numberOfLines={3} textAlign="center" textRole={big ? "body" : "label"} tone="secondary">
          {name}
        </Text>
      </Stack>
    )}
    {children}
  </Stack>
);

export { LibraryPoster, Line, PAGE_GUTTER, Stack };
