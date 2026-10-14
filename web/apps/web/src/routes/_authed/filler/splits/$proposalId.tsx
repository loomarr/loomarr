import { createFileRoute } from "@tanstack/react-router";
import { SplitReviewPage, validateSplitReviewSearch } from "@/filler/split-review-page";

// The split-review gate (§10 V34). The route exists so review survives the detection job:
// the proposal is persisted, the SSE frame merely hands over its id, and this page reads
// the proposal back. A SIBLING of /filler, not a child — the catalog page renders no
// <Outlet/>, and nesting would make the review unreachable.
const SplitReviewScreen = () => {
  const { proposalId } = Route.useParams();
  const { from } = Route.useSearch();
  return <SplitReviewPage proposalId={proposalId} from={from ?? "library"} />;
};

const Route = createFileRoute("/_authed/filler/splits/$proposalId")({
  validateSearch: validateSplitReviewSearch,
  component: SplitReviewScreen,
});

export { Route };
