import { createFileRoute, redirect } from "@tanstack/react-router";

// The closed set `?section=` used to accept, before the section became a path segment
// (V-nav-paths). "advanced" was never a real section (P7 folded it into the info panel's
// diagnostics disclosure), so it is deliberately absent here too — the same fallback the old
// validateSearch gave it.
const SECTION_IDS = ["info", "programming", "filler", "danger"] as const;

// /channels/$id opens on Channel info — the default section, and the only one a viewer can
// reach (the layout hides the tab bar entirely for a non-admin).
//
// ⚠ Also the landing spot for an old `?section=` bookmark/shared link: the query is read here
// and turned into the matching path, so a link from before the move still lands on the right
// section rather than always falling back to info. Every existing plain link to the bare
// channel route (the guide grid, the command palette, the approval queue, channel-row-menu)
// keeps working unchanged; it now takes one extra hop through here.
const Route = createFileRoute("/_authed/channels/$id/")({
  beforeLoad: ({ params, location }) => {
    const section = (location.search as { section?: unknown }).section;
    const target = SECTION_IDS.includes(section as (typeof SECTION_IDS)[number]) ? section : "info";
    throw redirect({ to: `/channels/$id/${target}` as "/channels/$id/info", params });
  },
});

export { Route };
