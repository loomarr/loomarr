import type { ComponentType, ReactNode } from "react";

// The app's top-level nav destinations — literals so TanStack `Link to={to}` stays
// type-checked against the route tree.
// `/channels` and `/suggest` are absent because the routes are: both folded into
// `/guide`, which is now the channels surface AND the origination door (§12).
type NavTo =
  | "/dashboard"
  | "/guide"
  | "/requests"
  | "/filler"
  | "/people"
  | "/settings"
  | "/settings/notifications"
  | "/help";

interface NavItem {
  to: NavTo;
  label: string;
  icon: ComponentType<{ className?: string }>;
  // No `admin?: boolean`. Role is expressed by WHICH LIST an item is in (§12: two authored
  // navs), not by a flag a filter reads — a flag can hide an entry but cannot rename one,
  // and the member nav needs different names for the same routes.
}

/**
 * The rail's Watch entry (#1817 item 3, decision X2): a channel id, not a `NavTo` literal, since
 * its target moves with `watchChannelId`. Kept out of `NavItem`/`NavTo` rather than widening
 * either — `badges` stays keyed by the stable literal routes, and this is the one item neither
 * list owns statically (inserted into both ADMIN_NAV and MEMBER_NAV only once a channel exists).
 */
interface WatchNavItem {
  to: "/channels/$id/watch";
  channelId: string;
  label: "Watch";
  icon: ComponentType<{ className?: string }>;
}

interface AppShellProps {
  children: ReactNode;
  isAdmin?: boolean;
  userName?: string;
  /** Server-reported build identity from GET /v1/system/version. */
  serverVersion?: string;
  /** A count pill on a nav entry — e.g. Requests' "needs you" count. Omitted or 0 shows nothing. */
  badges?: Partial<Record<NavTo, number>>;
  onOpenCommand?: () => void;
  onLogout?: () => void;
  /**
   * Watch's target (#1785/#1817 item 3, decision X2): the shared `watchChannelId` selector's
   * result — the caller's last-tuned channel, or the lowest-numbered playable one before anyone
   * has tuned. `undefined` hides Watch's slot entirely, in both the desktop rail and
   * PhoneBottomBar — the same prop drives both shells so neither computes its own fallback.
   */
  watchChannelId?: string;
}

export type { AppShellProps, NavItem, NavTo, WatchNavItem };
