import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { layoutGuide, useGuideWindow } from "@loomarr/core/guide";
import { type SurfChannelData, type WatchingScheduleData, watchingPanelFromGuide } from "@loomarr/ui";
import { keepPreviousData, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import type { DrawerChannel } from "../channel-drawer";
import { DRAWER_NOW_PERCENT, drawerChannels } from "../drawer-channels";

// The channels around the one being watched.
type WatchChannels = {
  // The channels drawer (#1659 W1): every channel read from the guide, and the viewer's own
  // favourites and recent channels. Also the list a typed number is looked up in.
  drawer: {
    channels: DrawerChannel[];
    favourites: string[];
    recent: string[];
    nowPercent: number;
    onFavourite: (channelId: string, favourite: boolean) => void;
  };
  // What a portrait phone shows under the picture (#1785, native mock 5e): the tuned channel's now
  // and next, the viewer's favourites, the channel Previous returns to, and the wall clock.
  phone?: {
    schedule?: WatchingScheduleData;
    favourites: readonly SurfChannelData[];
    previousId?: string;
    clockLabel?: string;
  };
  // A tune has settled on this channel (its first frame played): the viewer's recents record it.
  onTuneSettled: (channelId: string) => void;
};

// Reads the guide window Home and the Guide already share, and the viewer's favourites and recents
// (#1666). A star or a settled tune answers with both lists, which replace the cached ones. The
// phone panel reads the same guide the way the native Watching does: now and next for the tuned
// channel, the favourites with what each has on, and Previous as the newest recent that isn't it.
const useWatchChannels = (tunedId: string | undefined): WatchChannels => {
  const { nowMs, window: guideWindow } = useGuideWindow();
  const guide = channelsApi.useChannelGuide(guideWindow, {
    query: { retry: false, placeholderData: keepPreviousData },
  });
  const queryClient = useQueryClient();
  const mine = channelsApi.useMyChannels({ query: { retry: false } });
  const mineBody = unwrap(mine.data);
  const storeMine = {
    onSuccess: (res: unknown) => queryClient.setQueryData(channelsApi.getMyChannelsQueryKey(), res),
  };
  const addFavourite = channelsApi.useAddFavouriteChannel({ mutation: storeMine });
  const removeFavourite = channelsApi.useRemoveFavouriteChannel({ mutation: storeMine });
  const recordTune = channelsApi.useRecordChannelTune({ mutation: storeMine });

  const list = useMemo(() => drawerChannels(unwrap(guide.data)?.channels ?? [], nowMs), [guide.data, nowMs]);
  const favourites = useMemo(() => mineBody?.favourites.map((f) => f.channelId) ?? [], [mineBody]);
  const recent = useMemo(() => mineBody?.recent.map((r) => r.channelId) ?? [], [mineBody]);
  const phone = useMemo(() => {
    if (!tunedId) return undefined;
    const body = unwrap(guide.data);
    // Read defensively, as the drawer does: a body without `channels` must not reach the layout.
    return watchingPanelFromGuide({
      channelId: tunedId,
      favouriteIds: favourites,
      layout: body?.channels ? layoutGuide(body, nowMs) : undefined,
      nowMs,
      playableChannelIds: (body?.channels ?? []).filter((c) => c.status === "live").map((c) => c.channelId),
      recentIds: recent,
    });
  }, [guide.data, nowMs, tunedId, favourites, recent]);

  return {
    drawer: {
      channels: list,
      favourites,
      recent,
      nowPercent: DRAWER_NOW_PERCENT,
      onFavourite: (channelId, favourite) =>
        (favourite ? addFavourite : removeFavourite).mutate({ channelId }),
    },
    phone,
    onTuneSettled: (channelId) => recordTune.mutate({ channelId }),
  };
};

export type { WatchChannels };
export { useWatchChannels };
