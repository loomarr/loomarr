import { createFileRoute, redirect } from "@tanstack/react-router";

// /settings/system opens on Tasks — the only sub-page that exists today, and the one an
// operator comes to System for most often (is the sync running, when did it last go).
const Route = createFileRoute("/_authed/settings/system/")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/system/tasks" });
  },
});

export { Route };
