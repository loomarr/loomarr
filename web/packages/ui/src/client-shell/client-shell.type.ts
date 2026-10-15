import type { ReactNode } from "react";
import type { ClientDestination } from "../client-navigation";

type ClientShellProps = {
  active: ClientDestination;
  /**
   * The active destination's screen above the tab bar. A function receives the tab bar and draws its
   * own Screen with it as the footer, for a destination that docks something directly above the bar
   * (Requests' review sheet).
   */
  children?: ReactNode | ((navigation: ReactNode) => ReactNode);
  onNavigate(destination: ClientDestination): void;
  /** What needs this person on the Requests tab; 0 draws no badge. */
  requestsBadge?: number;
  /** A host sheet (the Device sheet) docked directly above the tab bar, on whichever tab is active. */
  sheet?: ReactNode;
};

export type { ClientShellProps };
