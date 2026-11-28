import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { Surface, Text } from "@loomarr/design-system";
import { WatchFrame } from "@loomarr/ui";
import { createFileRoute, useCanGoBack, useRouter } from "@tanstack/react-router";
import { type ReactNode, useCallback, useMemo, useRef } from "react";
import { useAuth } from "@/auth/use-auth";
import { ChannelWatch } from "@/channels/channel-watch";
import { useWatchChannels } from "@/channels/use-watch-channels";
import { useWatchPlayer, type WatchTuneReport } from "@/channels/use-watch-player";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useDocumentTitle } from "@/lib/use-document-title";

// The loading and error states sit in the middle of the black frame.
const Centered = ({ children }: { children: ReactNode }) => (
  <Surface
    alignItems="center"
    backgroundColor="transparent"
    borderWidth={0}
    flex={1}
    justifyContent="center"
    padding={24}
  >
    {children}
  </Surface>
);

// WATCH: play a channel live, full-bleed (#2088 step 1, `playback-clients.md` "Watch (web)"), on
// The Cross's watching screen (#4731). Its own route, outside the channel page: the authed layout
// drops the app shell here, so the picture owns the window and the named back control is the way
// out. A VIEWER surface: a member reaches it too.
const WatchRoute = () => {
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
  const channels = channelsApi.useListChannels();
  const nowNext = channelsApi.useChannelsNowNext({ query: { staleTime: 15_000 } });
  const follow = useCallback(
    (channelId: string) => {
      void navigate({ to: "/watch/$id", params: { id: channelId }, replace: true });
    },
    [navigate],
  );
  // The watching screen listens here for each tune the player begins.
  const tuneReports = useRef<((report: WatchTuneReport) => void) | undefined>(undefined);
  const onTune = useCallback((report: WatchTuneReport) => tuneReports.current?.(report), []);
  const listed = unwrap(channels.data)?.channels;
  const player = useWatchPlayer({
    channelId: id,
    channels: listed,
    nowNext: unwrap(nowNext.data)?.channels ?? [],
    onNavigate: follow,
    onTune,
  });
  const tunedChannel = player.channel ?? channel;
  // Leaving goes back where the viewer came from; a deep link with no history lands on the Guide.
  // The back control is named for where it goes.
  const back = useMemo(
    () => ({
      label: canGoBack ? "Back" : "Guide",
      onPress: () => {
        if (canGoBack) router.history.back();
        else void navigate({ to: "/guide" });
      },
    }),
    [canGoBack, router, navigate],
  );
  // Until the Guide is a view of this screen, the Guide control and G open the Guide page.
  const openGuide = useCallback(() => void navigate({ to: "/guide" }), [navigate]);

  const around = useWatchChannels(listed);

  if (channelQuery.error && !tunedChannel) {
    return (
      <WatchFrame title="Watch">
        <Centered>
          <ErrorState error={channelQuery.error} onRetry={() => channelQuery.refetch()} />
        </Centered>
      </WatchFrame>
    );
  }
  if (!tunedChannel) {
    return (
      <WatchFrame title="Watch">
        <Centered>
          <Text textRole="hint" tone="muted">
            Loading channel…
          </Text>
        </Centered>
      </WatchFrame>
    );
  }

  return (
    <WatchFrame title={`Watch ${tunedChannel.name}`}>
      <ChannelWatch
        back={back}
        channel={tunedChannel}
        channels={around}
        isAdmin={isAdmin}
        onGuide={openGuide}
        player={player}
        tuneReports={tuneReports}
      />
    </WatchFrame>
  );
};

const Route = createFileRoute("/_authed/watch/$id")({
  // The channel list and the on-air line the player tunes from. Surfing changes `$id` and must not
  // wait on the target's own read: ChannelWatch paints the target from the list, so when the list
  // has it the channel's detail is only started. A link to a channel the list does not hold (or a
  // list that failed) waits on the detail, so the page opens on the channel or its error.
  loader: async ({ context: { queryClient }, params: { id } }) => {
    const [list] = await Promise.all([
      queryClient.fetchQuery(channelsApi.getListChannelsQueryOptions()).catch(() => undefined),
      queryClient.prefetchQuery(channelsApi.getChannelsNowNextQueryOptions({ query: { staleTime: 15_000 } })),
    ]);
    const listed = list?.status === 200 && list.data.channels?.some((channel) => channel.id === id);
    const detail = queryClient.prefetchQuery(channelsApi.getGetChannelQueryOptions(id));
    if (listed) return;
    await detail;
  },
  component: WatchRoute,
});

export { Route };
