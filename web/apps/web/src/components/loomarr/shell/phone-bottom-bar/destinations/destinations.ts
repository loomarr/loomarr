import type { IconName } from "@loomarr/design-system";
import { isWatchRoute } from "@/channels/channel-nav-highlight";
import type { NavTo } from "../../app-shell/app-shell.type";

// Alt A (maintainer-approved, #1785/#1659 evidence 2026-10-03): the bar's first three are ALWAYS
// Home, Watch, Guide — never reshuffled by role or channel state, so a returning viewer's muscle
// memory for "second tab is Watch" either holds or the tab is absent, never a different
// destination. The fourth is the role's: an admin's Requests, a member's Library (#4476, where a
// member browses the titles). Everything else lives behind More.
type PhoneDestinationKey =
  | "filler"
  | "guide"
  | "help"
  | "home"
  | "library"
  | "notifications"
  | "people"
  | "requests"
  | "settings"
  | "watch";

type PhoneDestination = {
  /**
   * The bar draws this as a plain dot (today's icon-rail dot); the More sheet draws the number
   * itself (today's labelled-rail numeral), and More carries the dot for it.
   */
  badgeCount?: number;
  icon: IconName;
  key: PhoneDestinationKey;
  label: string;
} & ({ params?: never; to: NavTo } | { params: { id: string }; to: "/watch/$id" });

const ADMIN_PRIMARY: readonly PhoneDestinationKey[] = ["home", "watch", "guide", "requests"];
const MEMBER_PRIMARY: readonly PhoneDestinationKey[] = ["home", "watch", "guide", "library"];
// In the sidebar's order: Library sits after Guide and before Requests, so it leads the overflow.
const ADMIN_OVERFLOW: readonly PhoneDestinationKey[] = ["library", "filler", "people", "settings", "help"];
const MEMBER_OVERFLOW: readonly PhoneDestinationKey[] = ["requests", "notifications", "help"];

const destinationFor = (
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
        : { icon: "play", key, label: "Watch", params: { id: watchChannelId }, to: "/watch/$id" };
    case "guide":
      return { icon: "guide", key, label: "Guide", to: "/guide" };
    case "requests":
      return { icon: "requests", key, label: "Requests", to: "/requests" };
    case "library":
      return { icon: "library", key, label: "Library", to: "/library" };
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
  const count = destination.to === "/watch/$id" ? 0 : (badges?.[destination.to] ?? 0);
  return count > 0 ? { ...destination, badgeCount: count } : destination;
};

/** Home, Watch (if a channel exists), Guide, then an admin's Requests or a member's Library. */
const phoneBarPrimary = (
  isAdmin: boolean,
  watchChannelId: string | undefined,
  badges?: Partial<Record<NavTo, number>>,
): PhoneDestination[] =>
  (isAdmin ? ADMIN_PRIMARY : MEMBER_PRIMARY).flatMap((key) => {
    const destination = destinationFor(key, watchChannelId);
    return destination ? [withBadge(destination, badges)] : [];
  });

/** Everything else: admin's Library/Filler/People/Settings/Help, member's Requests/Notifications/Help. */
const phoneBarOverflow = (isAdmin: boolean, badges?: Partial<Record<NavTo, number>>): PhoneDestination[] =>
  (isAdmin ? ADMIN_OVERFLOW : MEMBER_OVERFLOW).flatMap((key) => {
    const destination = destinationFor(key, undefined);
    return destination ? [withBadge(destination, badges)] : [];
  });

/** A `/watch/:id` URL is "on Watch" regardless of which channel; otherwise exact-or-nested. */
const phoneDestinationMatches = (pathname: string, destination: Pick<PhoneDestination, "to">): boolean =>
  destination.to === "/watch/$id"
    ? isWatchRoute(pathname)
    : pathname === destination.to || pathname.startsWith(`${destination.to}/`);

export type { PhoneDestination, PhoneDestinationKey };
export { phoneBarOverflow, phoneBarPrimary, phoneDestinationMatches };
