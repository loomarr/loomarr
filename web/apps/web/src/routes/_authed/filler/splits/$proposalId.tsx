import { createFileRoute } from "@tanstack/react-router";
import { SplitReviewPage } from "@/filler";

// The split-review gate (§10 V34). The route exists so review survives the detection job:
// the proposal is persisted, the SSE frame merely hands over its id, and this page reads
// the proposal back. A SIBLING of /filler, not a child — the catalog page renders no
// <Outlet/>, and nesting would make the review unreachable.
const SplitReviewScreen = () => {
  const { proposalId } = Route.useParams();
  return <SplitReviewPage proposalId={proposalId} />;
};

const Route = createFileRoute("/_authed/filler/splits/$proposalId")({
  component: SplitReviewScreen,
});

export { Route };
