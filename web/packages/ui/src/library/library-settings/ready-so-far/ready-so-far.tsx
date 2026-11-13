import { Text } from "@loomarr/design-system";

import { LibraryPoster, Line, Stack } from "../../library-poster";
import type { LibraryReadyTitle } from "../library-settings.type";

// The latest few titles that became ready while the import runs (decision 29): a plain 2:3 tile
// each, too narrow to set the name in, so it is the one-line caption under it.
const READY_SO_FAR = 5;
const ReadySoFar = ({ titles }: { titles: readonly LibraryReadyTitle[] }) => (
  <Stack gap="inline">
    <Text textRole="label" tone="secondary">
      Ready so far
    </Text>
    <Line gap="control">
      {titles.slice(0, READY_SO_FAR).map((t) => (
        <Stack key={t.id} width={64}>
          <LibraryPoster art={t.poster} name="" />
          {/* A clipped, min-width-0 box is what lets the design system truncate. */}
          <Stack flexShrink={1} marginTop={6} minWidth={0} overflow="hidden" width={64}>
            <Text numberOfLines={1} textRole="label" tone="secondary">
              {t.name}
            </Text>
          </Stack>
        </Stack>
      ))}
    </Line>
  </Stack>
);

export { ReadySoFar };
