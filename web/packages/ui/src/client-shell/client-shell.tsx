import { Screen } from "@loomarr/design-system";

import { ClientNavigation } from "../client-navigation";
import { StatePanel } from "../state-panel";
import type { ClientShellProps } from "./client-shell.type";
import { DestinationBoundary } from "./destination-boundary";

// The paired phone's frame: the active destination's screen with the platform's tab bar docked under
// it. Each destination sets its own gutters, so a picture can run edge to edge (#1659 mock 5e). A
// destination that throws shows its own error with the tab bar still under it, so the other tabs stay
// reachable.
const ClientShell = ({ active, children, onNavigate, requestsBadge }: ClientShellProps) => {
  const navigation = (
    <ClientNavigation active={active} density="touch" onNavigate={onNavigate} requestsBadge={requestsBadge} />
  );
  return (
    <DestinationBoundary
      fallback={(retry) => (
        <Screen density="touch" footer={navigation}>
          <StatePanel
            action={{ label: "Try again", onPress: retry }}
            density="touch"
            kind="error"
            title="Something went wrong"
          />
        </Screen>
      )}
      resetKey={active}
    >
      {typeof children === "function" ? (
        children(navigation)
      ) : (
        <Screen density="touch" flush footer={navigation}>
          {children}
        </Screen>
      )}
    </DestinationBoundary>
  );
};

export { ClientShell };
