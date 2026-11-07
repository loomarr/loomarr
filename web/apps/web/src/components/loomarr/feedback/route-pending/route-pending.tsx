import { Surface } from "@loomarr/design-system";
import { StatePanel } from "@loomarr/ui";

// RoutePending — the router's default pending state, shown only once a route's guard or loader
// has taken longer than the router's pending delay.
const RoutePending = () => (
  <Surface backgroundColor="transparent" borderWidth={0} padding="section">
    <StatePanel kind="loading" title="Loading" />
  </Surface>
);

export { RoutePending };
