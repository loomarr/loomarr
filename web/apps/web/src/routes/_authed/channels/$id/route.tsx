import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
import { ChannelDetailPage } from "@/channels/channel-detail";

const ChannelDetailRoute = () => {
  const { id } = Route.useParams();
  const navigate = Route.useNavigate();
  return <ChannelDetailPage id={id} onDeleted={() => void navigate({ to: "/guide" })} />;
};

const Route = createFileRoute("/_authed/channels/$id")({
  component: ChannelDetailRoute,
  // The header paints from the channel, so the channel is awaited and a failure reaches the route's
  // error view. The reads that finish the header (the air line's now/next and timeline, and the
  // admin's media-server link) are only warmed: a failure there leaves that detail off, as it does
  // once the page is open, and prefetching never rejects.
  loader: async ({ context: { queryClient }, params: { id } }) => {
    const isAdmin = await viewerIsAdmin(queryClient);
    await Promise.all([
      queryClient.ensureQueryData(channelsApi.getGetChannelQueryOptions(id)),
      queryClient.prefetchQuery(channelsApi.getChannelsNowNextQueryOptions()),
      queryClient.prefetchQuery(channelsApi.getChannelTimelineQueryOptions(id, undefined)),
      isAdmin ? queryClient.prefetchQuery(settingsApi.getSettingsListQueryOptions()) : undefined,
    ]);
  },
});

export { Route };
