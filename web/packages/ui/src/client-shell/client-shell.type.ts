import type { Density } from "@loomarr/design-system";

type ClientDestination = "watching" | "guide" | "surf";
type ClientShellProps = {
  active: ClientDestination;
  density: Density;
  onNavigate(destination: ClientDestination): void;
  serverName?: string;
};

export type { ClientDestination, ClientShellProps };
