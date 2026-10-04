import type { Density } from "@loomarr/design-system";

type ClientDestination = "watching" | "guide" | "requests" | "surf";

/**
 * `actions` is the pointer and TV row of buttons. The phone bars follow each platform (#1659
 * native mock): `tabBar` is iPhone's (5d, 5e), `material` is Android's with its pill (5f, 5g).
 */
type ClientNavigationVariant = "actions" | "material" | "tabBar";

type ClientNavigationProps = {
  active: ClientDestination;
  density?: Density;
  onNavigate: (destination: ClientDestination) => void;
  /** What needs this person on the Requests tab (a phone's); 0 draws no badge. */
  requestsBadge?: number;
  /** Defaults to the platform's own bar at touch density, and `actions` otherwise. */
  variant?: ClientNavigationVariant;
};

export type { ClientDestination, ClientNavigationProps, ClientNavigationVariant };
