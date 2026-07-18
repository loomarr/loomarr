import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr";

const BoardScreen = () => (
  <Placeholder title="Board" hint="Nothing in flight — approved acquisitions show their journey here." />
);

const Route = createFileRoute("/_authed/board")({
  component: BoardScreen,
});

export { Route };
