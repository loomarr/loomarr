import type { LibraryItemDTO } from "@loomarr/api/models/libraryItemDTO";
import type { LibraryReadyTitle } from "@loomarr/ui";
import { ServedImage } from "@/components/loomarr/served-image";

// A tile in the import's strip of titles found is 64px wide.
const READY_POSTER_SIZES = "64px";

// A title the import found, as its strip shows it: whether it is ready, and its poster if it has one.
const readyTitleOf = (item: LibraryItemDTO): LibraryReadyTitle => ({
  id: item.id,
  name: item.name,
  ready: item.ready,
  poster: item.poster ? <ServedImage alt="" image={item.poster} sizes={READY_POSTER_SIZES} /> : undefined,
});

export { readyTitleOf };
