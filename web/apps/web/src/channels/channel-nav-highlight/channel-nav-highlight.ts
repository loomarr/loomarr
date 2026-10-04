// Decision X2 (redesign-map critique roll-up row 5, maintainer-clarified 2026-10-03): the four
// routes an admin edits a channel through, in SECTIONS order
// (web/apps/web/src/routes/_authed/channels/$id/route.tsx).
const CHANNEL_MANAGEMENT_SECTIONS = ["info", "programming", "filler", "danger"] as const;

const isWatchRoute = (pathname: string): boolean => /^\/channels\/[^/]+\/watch$/.test(pathname);

const isChannelManagementRoute = (pathname: string): boolean =>
  new RegExp(`^/channels/[^/]+/(?:${CHANNEL_MANAGEMENT_SECTIONS.join("|")})$`).test(pathname);

type ChannelNavHighlight = "guide" | "watch" | undefined;

/**
 * Decision X2: one channel never lights two nav items. Any channel's Watch tab highlights the
 * rail's Watch entry; any of its management tabs (Info/Programming/Filler/Danger) highlight Guide
 * instead — even though the URL stays under `/channels/$id/...` for both. `undefined` for every
 * other route, where each shell's normal path-match nav logic applies unchanged.
 *
 * Shared by the desktop rail and the phone bottom bar (#1849) so the transfer rule can't drift
 * between them — neither shell re-derives it from the URL on its own.
 */
const channelNavHighlight = (pathname: string): ChannelNavHighlight => {
  if (isWatchRoute(pathname)) return "watch";
  if (isChannelManagementRoute(pathname)) return "guide";
  return undefined;
};

export type { ChannelNavHighlight };
export { channelNavHighlight, isWatchRoute };
