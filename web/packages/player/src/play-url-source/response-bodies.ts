// What the player reads, derived from the generated contract. Loaded with `import()` on the first
// read, so the generated schemas stay out of the web app's first download.

import { ChannelPlayUrlResponse, ListChannelsResponse } from "@loomarr/api/zod/channels";

// An older server sends no clock reading and no still address, so those are optional here. The
// expiry is any string: one that is not a date leaves the source with no lifetime, not unplayable.
const PlayUrlBody = ChannelPlayUrlResponse.pick({ relativeUrl: true, url: true }).extend({
  ...ChannelPlayUrlResponse.pick({ relativeStillUrl: true, serverTimeMs: true, stillUrl: true }).partial()
    .shape,
  expiresAt: ChannelPlayUrlResponse.shape.url,
});
const CatalogBody = ListChannelsResponse.pick({ channels: true }).extend({
  channels: ListChannelsResponse.shape.channels.element
    .pick({ id: true, inAppPlayable: true, name: true, number: true })
    .array(),
});

export { CatalogBody, PlayUrlBody };
