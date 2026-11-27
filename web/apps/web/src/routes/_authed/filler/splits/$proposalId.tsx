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
  // Admin-only, so a member's request is never sent. The proposal is what the page is made of, so it
  // opens on it and a failed read is the route's error view. The settings list is warmed: it only
  // adds the shortest-clip rule to the editor.
  loader: async ({ context: { queryClient }, params: { proposalId } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.ensureQueryData(fillerApi.getGetFillerSplitQueryOptions(proposalId)),
      queryClient.prefetchQuery(settingsApi.getSettingsListQueryOptions({ query: { retry: false } })),
    ]);
  },
  component: SplitReviewScreen,
});

export { Route };
