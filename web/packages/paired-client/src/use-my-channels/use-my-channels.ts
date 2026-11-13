import type { MyChannelsController, MyChannelsSnapshot } from "@loomarr/core/my-channels";
import { createMyChannelsController, createMyChannelsPort } from "@loomarr/core/my-channels";
import { useEffect, useMemo, useRef, useSyncExternalStore } from "react";
import type { LastChannelStore } from "../last-channel";
import { useDisposedWith } from "../use-disposed-with";

interface UseMyChannelsOptions {
  /** Where the device remembers the channel it last watched; a settled tune is saved to it. */
  lastChannel?: LastChannelStore;
  request: typeof globalThis.fetch;
  /** The channel whose tune has settled (its first frame plays); undefined while tuning or idle. */
  settledChannelId?: string;
}

/**
 * The viewer's favourites and recents: made for the request, disposed when replaced or unmounted.
 * A tune that settles joins the viewer's recents (#1666), once per tune, as the web Watch page
 * records it; a warmed neighbour never plays, so it never counts. Pausing and resuming settles the
 * same channel again; only a tune to another channel counts.
 */
const useMyChannels = ({
  lastChannel,
  request,
  settledChannelId,
}: UseMyChannelsOptions): { myChannels: MyChannelsController; myChannelsSnapshot: MyChannelsSnapshot } => {
  const myChannels = useMemo(
    () => createMyChannelsController({ port: createMyChannelsPort(request) }),
    [request],
  );
  useDisposedWith(myChannels, (built) => built.dispose());
  const myChannelsSnapshot = useSyncExternalStore(
    myChannels.subscribe,
    myChannels.getSnapshot,
    myChannels.getSnapshot,
  );
  const recordedChannelId = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!settledChannelId || settledChannelId === recordedChannelId.current) return;
    recordedChannelId.current = settledChannelId;
    void myChannels.recordTune(settledChannelId).catch(() => undefined);
    // An unwritable store only means the next launch starts on the first channel.
    void lastChannel?.save(settledChannelId).catch(() => undefined);
  }, [lastChannel, myChannels, settledChannelId]);
  return { myChannels, myChannelsSnapshot };
};

export type { UseMyChannelsOptions };
export { useMyChannels };
