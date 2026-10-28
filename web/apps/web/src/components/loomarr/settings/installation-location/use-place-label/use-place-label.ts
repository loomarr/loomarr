import * as settingsApi from "@loomarr/api/endpoints/settings";
import { unwrap } from "@loomarr/api/unwrap";

// A saved place (filler.home_place, a channel's place) is an opaque location-index id, never
// something to show. This resolves it to the label the operator picked; undefined while it loads,
// when there is no place, or when the id no longer resolves.
const usePlaceLabel = (place: string | undefined): string | undefined => {
  const saved = settingsApi.useLocationsGet(place ?? "", {
    query: { enabled: Boolean(place), retry: false },
  });
  return place ? unwrap(saved.data, (body) => body.location)?.label : undefined;
};

export { usePlaceLabel };
