// PROTOTYPE — throwaway (#2151 step 2). The shared scan-health strip + stage stepper (decision 34).
// Placeholder with the FIXED signature; its builder replaces the body, never the props.
import { Text } from "@loomarr/design-system";
import type { ScanStatus } from "./prototype-data";

interface ScanHealthProps {
  status: ScanStatus;
  // "full": Settings → Library and the wizard (stepper + bar + live signals).
  // "compact": one line at the top of a Library page (headline + bar while running + action).
  density: "full" | "compact";
  onOpenSettings?: () => void;
}

const ScanHealth = ({ status }: ScanHealthProps) => <Text textRole="body">{status.headline}</Text>;

export type { ScanHealthProps };
export { ScanHealth };
