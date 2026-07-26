import type { ComponentType, ReactNode } from "react";

// The app's top-level nav destinations — literals so TanStack `Link to={to}` stays
// type-checked against the route tree.
type NavTo =
  | "/dashboard"
  | "/channels"
  | "/guide"
  | "/queue"
  | "/suggest"
  | "/filler"
  | "/people"
  | "/settings"
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
  onOpenCommand?: () => void;
  onLogout?: () => void;
}

export type { AppShellProps, NavItem };
