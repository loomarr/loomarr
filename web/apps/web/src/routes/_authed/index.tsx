import { createFileRoute, redirect } from "@tanstack/react-router";

// The app home (`/`) redirects to Channels — the first real surface (§12).
const Route = createFileRoute("/_authed/")({
  beforeLoad: () => {
    throw redirect({ to: "/channels" });
  },
});

export { Route };
