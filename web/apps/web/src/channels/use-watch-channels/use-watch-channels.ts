import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { useGuideWindow } from "@loomarr/core/guide";
import { keepPreviousData, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import { type WatchLineupChannel, watchLineupChannels } from "../watch-lineup";

// The channels the watching screen lists, and the viewer's own.
type WatchChannels = {
  // What the screen's lineup is built from (`@loomarr/core/channel-lineup`): every channel, the
  // viewer's favourites in the order starred, and their recent channels, newest first.
  lineup: {
    channels: readonly WatchLineupChannel[];
    favouriteIds: readonly string[];
    recentIds: readonly string[];
    lastTunedId: string | undefined;
  };
  // A tune has settled on this channel (its first frame played): the viewer's recents record it.
  onTuneSettled: (channelId: string) => void;
};

const NO_CHANNELS: readonly ChannelDTO[] = [];

// Reads the guide window Home and the Guide already share, for what is on now on each channel, and
// the viewer's favourites and recents (#1666). A settled tune answers with both lists, which
// replace the cached ones.
const useWatchChannels = (channels: readonly ChannelDTO[] | undefined): WatchChannels => {
  const { nowMs, window: guideWindow } = useGuideWindow();
  const guide = channelsApi.useChannelGuide(guideWindow, {
    query: { retry: false, placeholderData: keepPreviousData },
  });
  const queryClient = useQueryClient();
  const mine = channelsApi.useMyChannels({ query: { retry: false } });
  const mineBody = unwrap(mine.data);
  const recordTune = channelsApi.useRecordChannelTune({
    mutation: {
      onSuccess: (res: unknown) => queryClient.setQueryData(channelsApi.getMyChannelsQueryKey(), res),
    },
  });

  const listed = channels ?? NO_CHANNELS;
  // Read defensively: a guide body without `channels` lists every channel with nothing on.
  const timelines = unwrap(guide.data)?.channels;
  const lineupChannels = useMemo(
    () => watchLineupChannels(listed, timelines ?? [], nowMs),
    [listed, timelines, nowMs],
  );
  const favouriteIds = useMemo(() => mineBody?.favourites.map((f) => f.channelId) ?? [], [mineBody]);
  const recentIds = useMemo(() => mineBody?.recent.map((r) => r.channelId) ?? [], [mineBody]);

  return {
    lineup: { channels: lineupChannels, favouriteIds, lastTunedId: recentIds[0], recentIds },
    onTuneSettled: (channelId) => recordTune.mutate({ channelId }),
  };
};

export type { WatchChannels };
export { useWatchChannels };
