import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { SplitReviewPage } from "@/filler/split-review-page";
import { validateSplitReviewSearch } from "@/filler/split-review-search";

// The split-review gate (§10 V34). The route exists so review survives the detection job:
// the proposal is persisted, the SSE frame merely hands over its id, and this page reads
// the proposal back. A SIBLING of /filler, not a child — the catalog page renders no
// <Outlet/>, and nesting would make the review unreachable.
//
// Admin-only: reviewing a split decides what enters the catalog. A member gets AdminOnly's
// no-access state and the proposal is never fetched.
const SplitReviewScreen = () => {
  const { proposalId } = Route.useParams();
  const { from } = Route.useSearch();
  return (
    <AdminOnly>
      <SplitReviewPage proposalId={proposalId} from={from ?? "library"} />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/filler/splits/$proposalId")({
  validateSearch: validateSplitReviewSearch,
  // Admin-only, so a member's request is never sent. The proposal is what the page is made of and
  // it shows its own state while it loads, so it is warmed with the settings list it reads.
  loader: async ({ context: { queryClient }, params: { proposalId } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.prefetchQuery(fillerApi.getGetFillerSplitQueryOptions(proposalId)),
      queryClient.prefetchQuery(settingsApi.getSettingsListQueryOptions({ query: { retry: false } })),
    ]);
  },
  component: SplitReviewScreen,
});

export { Route };
