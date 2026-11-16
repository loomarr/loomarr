import * as channelsApi from "@loomarr/api/endpoints/channels";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { ChannelFiller } from "@/filler/channel-filler";
import { useChannelDetail } from "./-channel-detail-context";

type ChannelFillerSearch = { from?: "filler" };

// Filler — admin-only (a member gets AdminOnly's no-access state, #1977). This is already a
// dedicated route, so the sandbox is a normal page rather than a permanently-open accordion
// nested inside one.
const FillerScreen = () => {
  const { id, channel } = useChannelDetail();
  const { from } = Route.useSearch();
  return (
    <AdminOnly>
      <ChannelFiller channelId={id} policy={channel.policy} {...(from ? { from } : {})} />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/channels/$id/filler")({
  // `from` only ever adds a return link, so anything but a known origin is dropped rather than
  // trusted: a hand-edited URL gets the plain page.
  validateSearch: (search: Record<string, unknown>): ChannelFillerSearch =>
    search.from === "filler" ? { from: "filler" } : {},
  component: FillerScreen,
  // The coverage meter reads its own endpoint and answers for itself when it fails (a filler-less
  // install answers 501), so it is only warmed. Admin-only like the page, and the settings list
  // the break defaults come from is already warmed by the channel layout's loader.
  loader: async ({ context: { queryClient }, params: { id } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await queryClient.prefetchQuery(
      channelsApi.getChannelFillerCoverageQueryOptions(id, { query: { retry: false } }),
    );
  },
});

export { Route };
