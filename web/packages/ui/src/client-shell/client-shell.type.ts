import type { Density } from "@loomarr/design-system";

type ClientDestination = "watching" | "guide" | "surf";
type ClientShellProps = {
  active: ClientDestination;
  density: Density;
  onDisconnect(): Promise<void> | void;
  onNavigate(destination: ClientDestination): void;
  serverName?: string;
};

export type { ClientDestination, ClientShellProps };
