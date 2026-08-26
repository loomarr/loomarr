import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";
import type { ClientDestination } from "../client-navigation";

type ClientShellProps = {
  active: ClientDestination;
  children?: ReactNode;
  density: Density;
  onDisconnect(): Promise<void> | void;
  onNavigate(destination: ClientDestination): void;
  serverName?: string;
};

export type { ClientShellProps };
