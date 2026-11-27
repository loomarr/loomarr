import { createFileRoute, redirect } from "@tanstack/react-router";

// The page was called Queue until #1405; bookmarks and links to `/queue` keep working.
// no-loader: redirects to /requests before anything is read
const Route = createFileRoute("/_authed/queue/")({
  beforeLoad: () => {
    throw redirect({ to: "/requests", replace: true });
  },
});

export { Route };
