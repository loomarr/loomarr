import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { keepPreviousData, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ChannelWatch } from "@/channels/channel-watch";
import { DRAWER_NOW_PERCENT, drawerChannels } from "@/channels/drawer-channels";
import { defaultGuideWindow } from "@/channels/guide-window";
import { useChannelTuner } from "@/channels/use-channel-tuner";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { useChannelDetail } from "./-channel-detail-context";

// The "Open in …" hand-off names and opens the configured media server. `library.flavor` gives the
// name (Emby/Jellyfin) and `library.url` its front door — both non-secret settings, so their
// `value` is populated. An unset URL leaves `mediaServerUrl` undefined and the button hides itself
// rather than doing nothing.
const FLAVOR_NAMES: Record<string, string> = { emby: "Emby", jellyfin: "Jellyfin" };

// How often the drawer's rows move on: minutes left and the strip's window.
const NOW_TICK_MS = 30_000;

// WATCH — play the channel live in the browser (§9.1, V46). A VIEWER surface (like Overview): a
// member reaches it too, so it is not gated on isAdmin here. The channel-level audio/subtitle
// pickers inside gate their own editability on isAdmin — a member sees the values, an admin
// changes them.
const WatchScreen = () => {
  const { id } = Route.useParams();
  const navigate = Route.useNavigate();
  const { channel, isAdmin, savePolicy } = useChannelDetail();
  const settings = useSettingsEntries();
  const mediaServerUrl = settings.find((e) => e.key === "library.url")?.value || undefined;
  const mediaServerName =
    FLAVOR_NAMES[settings.find((e) => e.key === "library.flavor")?.value ?? ""] ?? undefined;
  const channels = channelsApi.useListChannels({ query: { staleTime: 30_000 } });
  const nowNext = channelsApi.useChannelsNowNext({ query: { staleTime: 15_000 } });
  const tune = useCallback(
    (target: { id: string }) => {
      void navigate({ to: "/channels/$id/watch", params: { id: target.id }, replace: true });
    },
    [navigate],
  );
  const tuner = useChannelTuner({
    currentId: id,
    channels: unwrap(channels.data)?.channels ?? [],
    nowNext: unwrap(nowNext.data)?.channels ?? [],
    onTune: tune,
  });
  const tunedChannel = tuner.channel ?? channel;

  // The channels drawer reads the guide window Home and the Guide already share, and the viewer's
  // favourites and recents (#1666). A star or a settled tune answers with both lists, which replace
  // the cached ones.
  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNowMs(Date.now()), NOW_TICK_MS);
    return () => clearInterval(id);
  }, []);
  const guide = channelsApi.useChannelGuide(defaultGuideWindow(nowMs), {
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
  const drawerChannelList = useMemo(
    () => drawerChannels(unwrap(guide.data)?.channels ?? [], nowMs),
    [guide.data, nowMs],
  );

  return (
    <>
      {/* A visually-hidden heading, same as filler.tsx. The Watch surface labels itself visibly through
          the player's own top bar (CH n + channel name) and the "Watch live" poster, none of which is a
          heading; as its own route it still needs a real heading for the page to be provably reachable —
          see the visible-heading reachability check — without adding a caption the mock doesn't have. */}
      <h2 className="sr-only">Watch {tunedChannel.name}</h2>
      <ChannelWatch
        channel={tunedChannel}
        isAdmin={isAdmin}
        onSavePolicy={savePolicy}
        mediaServerName={mediaServerName}
        mediaServerUrl={mediaServerUrl}
        tuner={{
          canSurf: tuner.canSurf,
          requestedChannel: tuner.requestedChannel,
          currentTitle: tuner.currentTitle,
          attempt: tuner.attempt,
          acknowledging: tuner.acknowledging,
          ready: tuner.ready,
          step: tuner.step,
          tune: tuner.tune,
          retry: tuner.retry,
        }}
        drawer={{
          channels: drawerChannelList,
          favourites: mineBody?.favourites.map((f) => f.channelId) ?? [],
          recent: mineBody?.recent.map((r) => r.channelId) ?? [],
          nowPercent: DRAWER_NOW_PERCENT,
          onFavourite: (channelId, favourite) =>
            (favourite ? addFavourite : removeFavourite).mutate({ channelId }),
        }}
        onTuneSettled={(channelId) => recordTune.mutate({ channelId })}
      />
    </>
  );
};

const Route = createFileRoute("/_authed/channels/$id/watch")({
  component: WatchScreen,
});

export { Route };
