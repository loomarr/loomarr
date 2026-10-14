import { createFileRoute, redirect } from "@tanstack/react-router";

const Route = createFileRoute("/_authed/filler/advanced")({
  beforeLoad: () => {
    throw redirect({ to: "/filler/manage" });
  },
});

export { Route };
