import { Action, Surface, TabBar } from "@loomarr/design-system";
import { Platform } from "react-native";

import type {
  ClientDestination,
  ClientNavigationProps,
  ClientNavigationVariant,
} from "./client-navigation.type";

const destinations = [
  { icon: "play", label: "Watching", value: "watching" },
  { icon: "guide", label: "Guide", value: "guide" },
  { icon: "requests", label: "Requests", value: "requests" },
  { icon: "channels", label: "Surf", value: "surf" },
] as const;

/** Requests is a phone destination: the TV and pointer rows keep Watching, Guide and Surf. */
const destinationsFor = (density: ClientNavigationProps["density"]) =>
  density === "touch" ? destinations : destinations.filter(({ value }) => value !== "requests");

const clientDestinationLabel = (destination: ClientDestination): string =>
  destinations.find((item) => item.value === destination)?.label ?? destination;

/** Back closes transient browsing first; the host owns the final exit to platform home. */
const clientBackDestination = (active: ClientDestination): ClientDestination | null =>
  active === "watching" ? null : "watching";

/** A phone gets its own platform's bar; pointer and TV keep the row of actions. */
const resolveClientNavigationVariant = (
  density: ClientNavigationProps["density"],
): ClientNavigationVariant =>
  density !== "touch" ? "actions" : Platform.OS === "android" ? "material" : "tabBar";

const ClientNavigation = ({
  active,
  density = "pointer",
  onNavigate,
  requestsBadge = 0,
  variant,
}: ClientNavigationProps) => {
  const resolved = variant ?? resolveClientNavigationVariant(density);
  const shown = destinationsFor(density);
  if (resolved !== "actions")
    return (
      <TabBar
        accessibilityLabel="Primary navigation"
        idiom={resolved === "material" ? "material" : "ios"}
        items={shown.map((item) => (item.value === "requests" ? { ...item, badge: requestsBadge } : item))}
        onSelect={onNavigate}
        selected={active}
      />
    );
  return (
    <Surface
      aria-label="Primary navigation"
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection="row"
      gap={density === "tv" ? "$section" : "$control"}
      role="navigation"
      width="100%"
    >
      {shown.map((destination) => {
        const selected = active === destination.value;
        return (
          <Action
            density={density}
            hasTVPreferredFocus={density === "tv" && selected}
            icon={destination.icon}
            key={destination.value}
            onPress={() => onNavigate(destination.value)}
            selected={selected}
            style={{ flex: 1 }}
            tone={selected ? "primary" : "secondary"}
          >
            {destination.label}
          </Action>
        );
      })}
    </Surface>
  );
};

export { ClientNavigation, clientBackDestination, clientDestinationLabel };
