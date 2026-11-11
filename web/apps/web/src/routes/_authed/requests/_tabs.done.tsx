import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
import { RequestList } from "@/queue/request-lists";

const DoneScreen = () => <RequestList tab="done" />;

const Route = createFileRoute("/_authed/requests/_tabs/done")({
  component: DoneScreen,
  // The Requests layout's loader ensured the lists; what only this tab adds is the admin's decided
  // filler pulls, an admin-only list that shows its own error, so it is warmed and not awaited.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await queryClient.prefetchQuery(
      fillerApi.getListFillerPullsQueryOptions({}, { query: { retry: false } }),
    );
  },
});

export { Route };
