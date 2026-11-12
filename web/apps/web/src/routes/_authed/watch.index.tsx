import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import { unwrap } from "@loomarr/api/unwrap";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { watchChannelId } from "@/channels/watch-channel-id";

// Bare `/watch` is what someone gets by trimming a Watch URL. It tunes the same channel the nav's
// Watch item does (`watchChannelId`: last tuned, else the lowest-numbered playable channel), and
// with nothing to watch it lands on the Guide rather than Not Found.
const Route = createFileRoute("/_authed/watch/")({
  beforeLoad: async ({ context }) => {
    const channels = unwrap(
      await context.queryClient
        .ensureQueryData(channelsApi.getListChannelsQueryOptions())
        .catch(() => undefined),
    )?.channels;
    if (channels && channels.length > 0) {
      // Where the viewer left off is a nicety: an unreadable answer falls back to the first channel.
      const viewing = await context.queryClient
        .ensureQueryData(dashboardApi.getHouseholdViewingQueryOptions({ query: { staleTime: 30_000 } }))
        .catch(() => undefined);
      const id = watchChannelId(channels, unwrap(viewing)?.continueWatching);
      if (id) throw redirect({ to: "/watch/$id", params: { id }, replace: true });
    }
    throw redirect({ to: "/guide", replace: true });
  },
});

export { Route };
