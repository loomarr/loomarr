import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { layoutGuide } from "@loomarr/core/guide";
import { watchingPanelFromGuide } from "@loomarr/ui";
import { keepPreviousData, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useCanGoBack, useRouter } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { ChannelWatch } from "@/channels/channel-watch";
import { DRAWER_NOW_PERCENT, drawerChannels } from "@/channels/drawer-channels";
import { defaultGuideWindow } from "@/channels/guide-window";
import { useChannelTuner } from "@/channels/use-channel-tuner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useDocumentTitle } from "@/lib/use-document-title";

// How often the drawer's rows move on: minutes left and the strip's window.
const NOW_TICK_MS = 30_000;

// WATCH — play a channel live, full-bleed (#2088 step 1, `playback-clients.md` "Watch (web)"). Its
// own route, outside the channel page: the authed layout drops the app shell here, so the picture
// owns the window and the player's ✕ is the way out. A VIEWER surface: a member reaches it too.
const WatchScreen = () => {
  const { id } = Route.useParams();
  const navigate = Route.useNavigate();
  const router = useRouter();
  const canGoBack = useCanGoBack();
  const { isAdmin } = useAuth();
  // A surf changes `$id` before the replacement GET completes. Retaining the previous DTO keeps the
  // one player/decoder mounted while ChannelWatch renders the target from the already-loaded list.
  const channelQuery = channelsApi.useGetChannel(id, { query: { placeholderData: (previous) => previous } });
  const channel = unwrap(channelQuery.data);
  useDocumentTitle(channel?.name);
  const channels = channelsApi.useListChannels({ query: { staleTime: 30_000 } });
  const nowNext = channelsApi.useChannelsNowNext({ query: { staleTime: 15_000 } });
  const tune = useCallback(
    (target: { id: string }) => {
      void navigate({ to: "/watch/$id", params: { id: target.id }, replace: true });
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
  // Leaving goes back where the viewer came from; a deep link with no history lands on the Guide.
  const leave = useCallback(() => {
    if (canGoBack) router.history.back();
    else void navigate({ to: "/guide" });
  }, [canGoBack, router, navigate]);

  // The channels drawer reads the guide window Home and the Guide already share, and the viewer's
  // favourites and recents (#1666). A star or a settled tune answers with both lists, which replace
  // the cached ones.
  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const tick = setInterval(() => setNowMs(Date.now()), NOW_TICK_MS);
    return () => clearInterval(tick);
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

  // A portrait phone's panel under the picture (#1785, mock 5e) reads the same guide the way the
  // native Watching does: now and next for the tuned channel, the favourites with what each has on,
  // and Previous as the newest recent that isn't this channel.
  const favouriteIds = useMemo(() => mineBody?.favourites.map((f) => f.channelId) ?? [], [mineBody]);
  const recentIds = useMemo(() => mineBody?.recent.map((r) => r.channelId) ?? [], [mineBody]);
  const tunedId = tunedChannel?.id;
  const phone = useMemo(() => {
    if (!tunedId) return undefined;
    const body = unwrap(guide.data);
    // Read defensively, as the drawer does: a body without `channels` must not reach the layout.
    return watchingPanelFromGuide({
      channelId: tunedId,
      favouriteIds,
      layout: body?.channels ? layoutGuide(body, nowMs) : undefined,
      nowMs,
      playableChannelIds: (body?.channels ?? []).filter((c) => c.status === "live").map((c) => c.channelId),
      recentIds,
    });
  }, [guide.data, nowMs, tunedId, favouriteIds, recentIds]);

  if (channelQuery.error && !tunedChannel) {
    return (
      <main className="bg-background flex min-h-dvh items-center justify-center p-6">
        <ErrorState error={channelQuery.error} onRetry={() => channelQuery.refetch()} />
      </main>
    );
  }
  if (!tunedChannel) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-black">
        <p className="text-muted-foreground text-sm">Loading channel…</p>
      </main>
    );
  }

  // Exactly the window's height, not at least it: the picture's `h-full` needs a definite height to
  // fill, or it falls back to 16:9 and leaves a band. A portrait phone's panel scrolls under it.
  return (
    <main className="flex h-dvh flex-col overflow-y-auto bg-black">
      {/* A visually-hidden heading: the player labels itself visibly through its top bar (CH n and
          the channel name), which is not a heading, and the page still needs one to be provably
          reachable (the visible-heading reachability check). */}
      <h1 className="sr-only">Watch {tunedChannel.name}</h1>
      <ChannelWatch
        channel={tunedChannel}
        isAdmin={isAdmin}
        onLeave={leave}
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
          favourites: favouriteIds,
          recent: recentIds,
          nowPercent: DRAWER_NOW_PERCENT,
          onFavourite: (channelId, favourite) =>
            (favourite ? addFavourite : removeFavourite).mutate({ channelId }),
        }}
        onTuneSettled={(channelId) => recordTune.mutate({ channelId })}
        phone={phone}
      />
    </main>
  );
};

const Route = createFileRoute("/_authed/watch/$id")({
  component: WatchScreen,
});

export { Route };
