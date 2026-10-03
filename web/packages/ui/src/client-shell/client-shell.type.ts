import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";
import type { ClientDestination } from "../client-navigation";

type ClientShellProps = {
  active: ClientDestination;
  /** On a phone, the active destination's screen above the tab bar; without it, the placeholder. */
  children?: ReactNode;
  density: Density;
  onDisconnect(): Promise<void> | void;
  onNavigate(destination: ClientDestination): void;
  serverName?: string;
};

export type { ClientShellProps };
