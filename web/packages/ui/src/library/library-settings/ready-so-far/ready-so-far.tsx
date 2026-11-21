import { Skeleton, Text } from "@loomarr/design-system";

import { LibraryPoster, Line, Stack } from "../../library-poster";
import type { LibraryReadyTitle } from "../library-settings.type";

// The latest few titles the import found (decision 29, option A), evolving with the scan with no
// words about it: a pulsing loading tile while its place waits; the title, faded, under its name once
// the walk writes it (its poster when its folder has one, else a plain tile); the title in full once
// its file is read and it is ready; and its poster arriving when its details are looked up. The row
// keeps its five places from the start, so nothing under it moves.
const READY_SO_FAR = 5;

const Slot = ({ title }: { title?: LibraryReadyTitle }) => (
  <Stack aria-hidden={title ? undefined : true} width={64}>
    {title ? (
      // The caption names it, so the tile itself sets no name.
      <LibraryPoster art={title.poster} dim={!title.ready} name="" />
    ) : (
      <Skeleton height={96} shape="media" width={64} />
    )}
    {/* A clipped, min-width-0 box is what lets the design system truncate. */}
    <Stack flexShrink={1} marginTop={6} minWidth={0} overflow="hidden" width={64}>
      <Text numberOfLines={1} textRole="label" tone="secondary">
        {title?.name ?? " "}
      </Text>
    </Stack>
  </Stack>
);

const ReadySoFar = ({ titles }: { titles: readonly LibraryReadyTitle[] }) => {
  const shown = titles.slice(0, READY_SO_FAR);
  // No heading: the tiles filling in, then turning into posters, say it.
  return (
    <Line gap="control">
      {Array.from({ length: READY_SO_FAR }, (_, i) => (
        <Slot key={shown[i]?.id ?? `empty-${i}`} title={shown[i]} />
      ))}
    </Line>
  );
};

export { ReadySoFar };
