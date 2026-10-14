import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";
import type { ClientDestination } from "../client-navigation";

type ClientShellProps = {
  active: ClientDestination;
  /**
   * On a phone, the active destination's screen above the tab bar; without it, the placeholder. A
   * function receives the tab bar and draws its own Screen with it as the footer, for a destination
   * that docks something directly above the bar (Requests' review sheet).
   */
  children?: ReactNode | ((navigation: ReactNode) => ReactNode);
  density: Density;
  onDisconnect(): Promise<void> | void;
  onNavigate(destination: ClientDestination): void;
  /** What needs this person on the phone's Requests tab; 0 draws no badge. */
  requestsBadge?: number;
  serverName?: string;
};

export type { ClientShellProps };
