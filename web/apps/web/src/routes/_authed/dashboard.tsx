import * as channelsApi from "@loomarr/api/endpoints/channels";
import { createFileRoute } from "@tanstack/react-router";
import { defaultGuideWindow } from "@/channels/guide-window";
import { HomePage } from "@/home/home-page";

// Home (#1659, Q-H1): the landing page for both roles. The route keeps its `/dashboard` path so
// bookmarks keep working. The old Dashboard's operator panels moved to Settings → This server:
// playout to Playback, services, activity and restart to Diagnostics › Health (Q-H7).
const Route = createFileRoute("/_authed/dashboard")({
  component: HomePage,
  // The guide is what Home paints: its channel wall, and the gate every later section waits behind
  // (the sections below it read their own endpoints and hide when empty). Home reads the same
  // quantised window as the Guide, so this is also the entry the Guide warms and vice versa.
  //
  // Awaited, but a prefetch rather than an ensure: Home owns its failure states (a whole-page
  // "Couldn't load your channels" with its own Try again), so a failed guide mounts the page to
  // say so instead of replacing it with the router's error view. The page keeps its loading state
  // too: it pins its window at mount, and a click that lands a minute later than this loader's
  // `Date.now()` asks for the next window's entry.
  loader: async ({ context: { queryClient } }) => {
    await queryClient.prefetchQuery(channelsApi.getChannelGuideQueryOptions(defaultGuideWindow(Date.now())));
  },
});

export { Route };
