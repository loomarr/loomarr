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
      justifyContent={art ? "center" : "flex-end"}
      opacity={dim ? 0.55 : 1}
      padding={art ? 0 : big ? "section" : "control"}
      state="ready"
      width="100%"
    >
      {art ?? (
        <Text numberOfLines={3} textAlign="center" textRole={big ? "body" : "label"} tone="secondary">
          {name}
        </Text>
      )}
    </ArtworkFrame>
    {children}
  </Stack>
);

export { LibraryPoster, Line, PAGE_GUTTER, Stack };
