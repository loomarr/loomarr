// PROTOTYPE — throwaway (#2151 step 2). Settings → Library (decision 29) — placeholder.
import { Text } from "@loomarr/design-system";
import type { Device, ScanState } from "./prototype-state";

const SettingsLibrary = ({ scan }: { scan: ScanState; device: Device }) => <Text textRole="body">Settings → Library · {scan}</Text>;

export { SettingsLibrary };
