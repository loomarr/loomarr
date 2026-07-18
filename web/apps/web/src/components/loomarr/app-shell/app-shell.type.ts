import type { ComponentType, ReactNode } from "react";

interface NavItem {
  to: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
  admin?: boolean;
}

interface AppShellProps {
  children: ReactNode;
  isAdmin?: boolean;
  userName?: string;
  onOpenCommand?: () => void;
}

export type { AppShellProps, NavItem };
