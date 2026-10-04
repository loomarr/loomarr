import type { NavTo } from "../app-shell/app-shell.type";

interface PhoneBottomBarProps {
  isAdmin: boolean;
  /** Same shape AppShell takes — today only `/requests` carries a count. */
  badges?: Partial<Record<NavTo, number>>;
  /** Decision X2: undefined hides Watch's slot entirely rather than holding a gap. */
  watchChannelId?: string;
}

export type { PhoneBottomBarProps };
