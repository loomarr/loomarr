import { createFileRoute, redirect } from "@tanstack/react-router";

// The nav's Notifications item lives under Settings; the bare `/notifications` someone types or
// trims a path down to lands on that page rather than Not Found.
// no-loader: redirects to Settings → Notifications before anything is read
const Route = createFileRoute("/_authed/notifications")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/notifications", replace: true });
  },
});

export { Route };
