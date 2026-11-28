import type { IconName } from "@loomarr/design-system";
import { isWatchRoute } from "@/channels/channel-nav-highlight";
import type { NavTo } from "../../app-shell/app-shell.type";

// Alt A (maintainer-approved, #1785/#1659 evidence 2026-10-03): the bar's first three are ALWAYS
// Home, Watch, Guide — never reshuffled by role or channel state, so a returning viewer's muscle
// memory for "second tab is Watch" either holds or the tab is absent, never a different
// destination. The fourth is Library for both roles (#4476): a member browses the titles there, and
// an admin's carries the Needs you count. Everything else lives behind More.
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
   * The bar and the More sheet both draw the number; More itself carries a plain dot when any
   * destination behind it has one.
   */
  badgeCount?: number;
  /** The words after the count in the bar's tab name; the tab bar's own when omitted. */
  badgeWords?: { one: string; other: string };
  icon: IconName;
  key: PhoneDestinationKey;
  label: string;
} & ({ params?: never; to: NavTo } | { params: { id: string }; to: "/watch/$id" });

const PRIMARY: readonly PhoneDestinationKey[] = ["home", "watch", "guide", "library"];
// In the sidebar's order: Requests sits right after Library, so it leads the overflow.
const ADMIN_OVERFLOW: readonly PhoneDestinationKey[] = ["requests", "filler", "people", "settings", "help"];
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
      // The Library's own phrasing, as its wall and its Needs you page say it.
      return {
        badgeWords: { one: "needs you", other: "need you" },
        icon: "library",
        key,
        label: "Library",
        to: "/library",
      };
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

/** Home, Watch (if a channel exists), Guide, Library: the same bar for both roles. */
const phoneBarPrimary = (
  watchChannelId: string | undefined,
  badges?: Partial<Record<NavTo, number>>,
): PhoneDestination[] =>
  PRIMARY.flatMap((key) => {
    const destination = destinationFor(key, watchChannelId);
    return destination ? [withBadge(destination, badges)] : [];
  });

/** Everything else: admin's Requests/Filler/People/Settings/Help, member's Requests/Notifications/Help. */
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
