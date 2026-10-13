import type { IconName } from "@loomarr/design-system";
import type { NavTo } from "../../app-shell/app-shell.type";

// Alt A (maintainer-approved, #1785/#1659 evidence 2026-10-03): the bar's first positions are
// ALWAYS Home, Watch, Guide, Requests — never reshuffled by role or channel state, so a returning
// viewer's muscle memory for "second tab is Watch" either holds or the tab is absent, never a
// different destination. Everything else lives behind More.
type PhoneDestinationKey =
  | "filler"
  | "guide"
  | "help"
  | "home"
  | "notifications"
  | "people"
  | "requests"
  | "settings"
  | "watch";

type PhoneDestination = {
  /**
   * The bar draws this as a plain dot (today's icon-rail dot); the More sheet draws the number
   * itself (today's labelled-rail numeral). No destination in the current 8/6 lists is both
   * badged and in the overflow set, so only the dot path is exercised today.
   */
  badgeCount?: number;
  icon: IconName;
  key: PhoneDestinationKey;
  label: string;
} & ({ params?: never; to: NavTo } | { params: { id: string }; to: "/channels/$id/watch" });

const PRIMARY_ORDER: readonly PhoneDestinationKey[] = ["home", "watch", "guide", "requests"];
const ADMIN_OVERFLOW: readonly PhoneDestinationKey[] = ["filler", "people", "settings", "help"];
const MEMBER_OVERFLOW: readonly PhoneDestinationKey[] = ["notifications", "help"];

const describe = (
  key: PhoneDestinationKey,
  watchChannelId: string | undefined,
): PhoneDestination | undefined => {
  switch (key) {
    case "home":
      return { icon: "home", key, label: "Home", to: "/dashboard" };
    case "watch":
      // Decision X2: hidden (not greyed, not a held gap) until a channel exists.
      return watchChannelId === undefined
        ? undefined
        : { icon: "play", key, label: "Watch", params: { id: watchChannelId }, to: "/channels/$id/watch" };
    case "guide":
      return { icon: "guide", key, label: "Guide", to: "/guide" };
    case "requests":
      return { icon: "requests", key, label: "Requests", to: "/requests" };
    case "filler":
      return { icon: "filler", key, label: "Filler", to: "/filler" };
    case "people":
      return { icon: "people", key, label: "People", to: "/people" };
    case "settings":
      return { icon: "settings", key, label: "Settings", to: "/settings" };
    case "help":
      return { icon: "help", key, label: "Help", to: "/help" };
    case "notifications":
      return { icon: "notifications", key, label: "Notifications", to: "/settings/notifications" };
  }
};

const withBadge = (
  destination: PhoneDestination,
  badges: Partial<Record<NavTo, number>> | undefined,
): PhoneDestination => {
  const count = destination.to === "/channels/$id/watch" ? 0 : (badges?.[destination.to] ?? 0);
  return count > 0 ? { ...destination, badgeCount: count } : destination;
};

/** Home, Watch (if a channel exists), Guide, Requests — Alt A's bar, independent of role. */
const phoneBarPrimary = (
  watchChannelId: string | undefined,
  badges?: Partial<Record<NavTo, number>>,
): PhoneDestination[] =>
  PRIMARY_ORDER.flatMap((key) => {
    const destination = describe(key, watchChannelId);
    return destination ? [withBadge(destination, badges)] : [];
  });

/** Everything else: admin's Filler/People/Settings/Help, member's Notifications/Help. */
const phoneBarOverflow = (isAdmin: boolean, badges?: Partial<Record<NavTo, number>>): PhoneDestination[] =>
  (isAdmin ? ADMIN_OVERFLOW : MEMBER_OVERFLOW).flatMap((key) => {
    const destination = describe(key, undefined);
    return destination ? [withBadge(destination, badges)] : [];
  });

/** A `/channels/:id/watch` URL is "on Watch" regardless of which channel; otherwise exact-or-nested. */
const phoneDestinationMatches = (pathname: string, destination: Pick<PhoneDestination, "to">): boolean =>
  destination.to === "/channels/$id/watch"
    ? /^\/channels\/[^/]+\/watch$/.test(pathname)
    : pathname === destination.to || pathname.startsWith(`${destination.to}/`);

export type { PhoneDestination, PhoneDestinationKey };
export { phoneBarOverflow, phoneBarPrimary, phoneDestinationMatches };
