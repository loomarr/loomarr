import { createFileRoute, redirect } from "@tanstack/react-router";

// The page was called Queue until #1405; bookmarks and links to `/queue` keep working.
const Route = createFileRoute("/_authed/queue/")({
  beforeLoad: () => {
    throw redirect({ to: "/requests", replace: true });
  },
});

export { Route };
