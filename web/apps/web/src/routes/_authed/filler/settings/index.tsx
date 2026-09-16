import { createFileRoute, redirect } from "@tanstack/react-router";

const Route = createFileRoute("/_authed/filler/settings/")({
  beforeLoad: () => {
    throw redirect({ to: "/filler/settings/$section", params: { section: "downloads" } });
  },
});

export { Route };
