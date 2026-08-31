import type { Density } from "@loomarr/design-system";

type ClientDestination = "watching" | "guide" | "surf";

type ClientNavigationProps = {
  active: ClientDestination;
  density?: Density;
  onNavigate: (destination: ClientDestination) => void;
};

export type { ClientDestination, ClientNavigationProps };
