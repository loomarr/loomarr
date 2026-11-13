import type { ReactNode } from "react";

import { LibraryPoster, Stack } from "../../library-poster";

// A poster small enough for a row: the tile alone.
const NeedThumb = ({ name, art, width }: { name: string; art?: ReactNode; width: number }) => (
  <Stack flexShrink={0} width={width}>
    <LibraryPoster art={art} name={art ? name : ""} />
  </Stack>
);

export { NeedThumb };
