import { createFileRoute, redirect } from "@tanstack/react-router";

// Watch moved out of the channel page to its own full-bleed route (#2088 step 1). An old bookmark
// or shared `/channels/$id/watch` link still lands on the player.
const Route = createFileRoute("/_authed/channels/$id/watch")({
  beforeLoad: ({ params }) => {
    throw redirect({ to: "/watch/$id", params, replace: true });
  },
});

export { Route };
