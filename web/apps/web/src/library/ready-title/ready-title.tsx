import type { LibraryItemDTO } from "@loomarr/api/models/libraryItemDTO";
import type { LibraryReadyTitle } from "@loomarr/ui";
import { ServedImage } from "@/components/loomarr/served-image";

// A "Ready so far" tile is 64px wide.
const READY_POSTER_SIZES = "64px";

// A title that became ready, as "Ready so far" shows it: its poster, else a plain tile.
const readyTitleOf = (item: LibraryItemDTO): LibraryReadyTitle => ({
  id: item.id,
  name: item.name,
  poster: item.poster ? <ServedImage alt="" image={item.poster} sizes={READY_POSTER_SIZES} /> : undefined,
});

export { readyTitleOf };
