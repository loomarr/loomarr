import type { ComponentType, ReactNode } from "react";

// The app's top-level nav destinations — literals so TanStack `Link to={to}` stays
// type-checked against the route tree.
type NavTo = "/channels" | "/board" | "/suggest" | "/filler" | "/users" | "/settings" | "/help";

interface NavItem {
  to: NavTo;
  label: string;
  icon: ComponentType<{ className?: string }>;
  admin?: boolean;
}

interface AppShellProps {
  children: ReactNode;
  isAdmin?: boolean;
  userName?: string;
  onOpenCommand?: () => void;
  onLogout?: () => void;
}

export type { AppShellProps, NavItem };
