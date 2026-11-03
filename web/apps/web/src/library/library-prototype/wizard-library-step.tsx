// PROTOTYPE — throwaway (#2151 step 2). The wizard's Library step (decision 30) — placeholder.
import { Text } from "@loomarr/design-system";
import type { Device, ScanState } from "./prototype-state";

const WizardLibraryStep = ({ scan }: { scan: ScanState; device: Device }) => <Text textRole="body">Wizard · Library · {scan}</Text>;

export { WizardLibraryStep };
