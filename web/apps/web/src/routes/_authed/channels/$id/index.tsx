import { createFileRoute, redirect } from "@tanstack/react-router";

// The closed set a legacy `?section=` link can name; sections are path segments (V-nav-paths).
// "advanced" is not a section (P7: it is the info panel's diagnostics disclosure), so it is
// deliberately absent and falls back to info like any unknown value.
const SECTION_IDS = ["info", "watch", "programming", "filler", "danger"] as const;

// /channels/$id opens on CHANNEL INFO: what is on the channel and whether it is working, for a
// viewer and an admin alike. Watching is the header's Watch button, its own full-bleed route
// (#2088), so opening a channel does not start a stream.
//
// ⚠ Also the landing spot for an old `?section=` bookmark/shared link: the query is read here
// and turned into the matching path (`watch` goes on to the player), so such a link lands on
// the right section rather than always falling back to info.
// no-loader: redirects to a section before anything is read
const Route = createFileRoute("/_authed/channels/$id/")({
  beforeLoad: ({ params, location }) => {
    const section = (location.search as { section?: unknown }).section;
    const target = SECTION_IDS.includes(section as (typeof SECTION_IDS)[number]) ? section : "info";
    throw redirect({ to: `/channels/$id/${target}` as "/channels/$id/info", params });
  },
});

export { Route };
