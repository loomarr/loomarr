import { createFileRoute, redirect } from "@tanstack/react-router";

// The closed set `?section=` used to accept, before the section became a path segment
// (V-nav-paths). "advanced" was never a real section (P7 folded it into the info panel's
// diagnostics disclosure), so it is deliberately absent here too — the same fallback the old
// validateSearch gave it.
const SECTION_IDS = ["info", "watch", "programming", "filler", "danger"] as const;

// /channels/$id opens on CHANNEL INFO: what is on the channel and whether it is working, for a
// viewer and an admin alike. Watching is the header's Watch button, its own full-bleed route
// (#2088), so opening a channel no longer starts a stream.
//
// ⚠ Also the landing spot for an old `?section=` bookmark/shared link: the query is read here
// and turned into the matching path (`watch` goes on to the player), so a link from before the
// move still lands on the right section rather than always falling back to info.
const Route = createFileRoute("/_authed/channels/$id/")({
  beforeLoad: ({ params, location }) => {
    const section = (location.search as { section?: unknown }).section;
    const target = SECTION_IDS.includes(section as (typeof SECTION_IDS)[number]) ? section : "info";
    throw redirect({ to: `/channels/$id/${target}` as "/channels/$id/info", params });
  },
});

export { Route };
