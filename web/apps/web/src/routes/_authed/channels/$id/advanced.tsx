import { createFileRoute, redirect } from "@tanstack/react-router";

// "Advanced" was folded into the info page's diagnostics disclosure (P7), so a link or a trimmed
// path ending in `/advanced` lands on Info rather than Not Found.
// no-loader: redirects to the info section before anything is read
const Route = createFileRoute("/_authed/channels/$id/advanced")({
  beforeLoad: ({ params }) => {
    throw redirect({ to: "/channels/$id/info", params, replace: true });
  },
});

export { Route };
