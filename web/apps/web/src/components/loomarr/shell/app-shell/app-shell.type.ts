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
}

export type { AppShellProps, NavItem };
