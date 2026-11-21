import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
import { RequestDetail } from "@/queue/request-detail";
import { ensureRequests } from "@/queue/use-requests";

const RequestDetailScreen = () => {
  const { jobId } = Route.useParams();
  return <RequestDetail jobId={jobId} />;
};

const Route = createFileRoute("/_authed/requests/$jobId")({
  component: RequestDetailScreen,
  // The request itself, and the titles its status line is worked out from.
  loader: async ({ context: { queryClient }, params: { jobId } }) => {
    const isAdmin = await viewerIsAdmin(queryClient);
    await Promise.all([
      queryClient.ensureQueryData(proposalJobsApi.getGetProposalJobQueryOptions(jobId)),
      ensureRequests(queryClient, isAdmin),
    ]);
  },
});

export { Route };
