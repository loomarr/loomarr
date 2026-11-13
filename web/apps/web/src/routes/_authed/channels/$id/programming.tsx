import * as channelsApi from "@loomarr/api/endpoints/channels";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { useChannelDetail } from "./-channel-detail-context";
import { ChannelProgramming } from "./-channel-programming";

// Programming — admin-only (a member gets AdminOnly's no-access state, #1977). Lineup,
// Programming rules and Refine as one surface (P7).
const ProgrammingScreen = () => {
  const { id, channel: ch, savePolicy, invalidate } = useChannelDetail();

  return (
    <AdminOnly>
      <ChannelProgramming
        channelId={id}
        channelName={ch.name}
        lineup={ch.lineup ?? []}
        policy={ch.policy}
        onPolicyChange={savePolicy}
        strategy={ch.strategy}
        fromRequest={ch.fromRequest}
        onRefined={invalidate}
      />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/channels/$id/programming")({
  component: ProgrammingScreen,
  // The rule vocabulary the editor waits for, and the saved cycle the seasonal line and Play order
  // read. Each is read by its own part of the page, which answers for itself, so they are warmed
  // (a prefetch never rejects). Admin-only like the page.
  loader: async ({ context: { queryClient }, params: { id } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.prefetchQuery(
        channelsApi.getGetProgrammingVocabularyQueryOptions({
          query: { staleTime: Number.POSITIVE_INFINITY },
        }),
      ),
      queryClient.prefetchQuery(
        channelsApi.getPreviewChannelCycleQueryOptions(id, undefined, { query: { retry: false } }),
      ),
    ]);
  },
});

export { Route };
