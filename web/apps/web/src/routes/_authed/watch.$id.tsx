import { createFileRoute } from "@tanstack/react-router";

// WATCH — `/watch/$id`, play a channel live, full-bleed (#2088). The route draws nothing itself: the
// authed layout draws WatchScreen for it, at a place in the tree that survives leaving the route, so
// a channel in the browser's picture-in-picture keeps playing while the viewer goes elsewhere
// (#2088 decision 4). See `channels/watch-screen`.
const Route = createFileRoute("/_authed/watch/$id")({
  component: () => null,
});

export { Route };
