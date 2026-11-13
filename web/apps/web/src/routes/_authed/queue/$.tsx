import { createFileRoute, redirect } from "@tanstack/react-router";

// The legacy Queue tab paths, mapped to where their contents live (#1405). Anything else under
// /queue falls back to the Requests page rather than a Not Found.
const MOVED = {
  approval: "/requests/needs-you",
  flight: "/requests/in-progress",
  history: "/requests/done",
} as const;

// no-loader: redirects to the new path before anything is read
const Route = createFileRoute("/_authed/queue/$")({
  beforeLoad: ({ params }) => {
    const tab = params._splat as keyof typeof MOVED;
    throw redirect({ to: MOVED[tab] ?? "/requests", replace: true });
  },
});

export { Route };
