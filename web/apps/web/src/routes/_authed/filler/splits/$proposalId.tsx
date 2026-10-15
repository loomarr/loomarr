import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { SplitReviewPage, validateSplitReviewSearch } from "@/filler/split-review-page";

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
  component: SplitReviewScreen,
});

export { Route };
