import { createFileRoute, redirect } from "@tanstack/react-router";

// Watch is its own full-bleed route, not a channel-page section (#2088 step 1). An old bookmark
// or shared `/channels/$id/watch` link lands on the player.
// no-loader: redirects to the player before anything is read
const Route = createFileRoute("/_authed/channels/$id/watch")({
  beforeLoad: ({ params }) => {
    throw redirect({ to: "/watch/$id", params, replace: true });
  },
});

export { Route };
