import { createFileRoute, redirect } from "@tanstack/react-router";

// /settings opens on Connections — the page an operator most often came here for, and
// the one carrying the troubleshooting checklist (§5, §13).
const Route = createFileRoute("/_authed/settings/")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/connections" });
  },
});

export { Route };
