import { createFileRoute, redirect } from "@tanstack/react-router";

// no-loader: redirects to Manage before anything is read
const Route = createFileRoute("/_authed/filler/advanced")({
  beforeLoad: () => {
    throw redirect({ to: "/filler/manage" });
  },
});

export { Route };
