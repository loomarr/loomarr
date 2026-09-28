import { createFileRoute } from "@tanstack/react-router";
import { HomePage } from "@/home/home-page";

// Home (#1659, Q-H1): the landing page for both roles. The route keeps its `/dashboard` path so
// bookmarks keep working. The old Dashboard's operator panels moved to Settings → This server:
// playout to Playback, services, activity and restart to Diagnostics › Health (Q-H7).
const Route = createFileRoute("/_authed/dashboard")({
  component: HomePage,
});

export { Route };
