import { createFileRoute, redirect } from "@tanstack/react-router";

// The closed set `?section=` used to accept, before the section became a path segment
// (V-nav-paths). "advanced" was never a real section (P7 folded it into the info panel's
// diagnostics disclosure), so it is deliberately absent here too — the same fallback the old
// validateSearch gave it.
const SECTION_IDS = ["info", "watch", "programming", "filler", "danger"] as const;

// /channels/$id opens on WATCH (§9.1 V54) — the first tab, and a viewer surface, so the default
// stays reachable for a non-admin (the layout hides the tab bar entirely for them). Opening a
// channel now shows what is ON it; Channel info is one tab over.
//
// ⚠ Also the landing spot for an old `?section=` bookmark/shared link: the query is read here
// and turned into the matching path, so a link from before the move still lands on the right
// section rather than always falling back to info. Every existing plain link to the bare
// channel route (the guide grid, the command palette, the approval queue, channel-row-menu)
// keeps working unchanged; it now takes one extra hop through here.
const Route = createFileRoute("/_authed/channels/$id/")({
  beforeLoad: ({ params, location }) => {
    const section = (location.search as { section?: unknown }).section;
    const target = SECTION_IDS.includes(section as (typeof SECTION_IDS)[number]) ? section : "watch";
    throw redirect({ to: `/channels/$id/${target}` as "/channels/$id/info", params });
  },
});

export { Route };
