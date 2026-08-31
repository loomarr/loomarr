import { requireOptionalNativeModule } from "expo";

type LegacyPairingCredential = { serverUrl: string; token: string };
type LegacyPairingNativeModule = { read(): Promise<LegacyPairingCredential | null> };

const nativeModule = requireOptionalNativeModule<LegacyPairingNativeModule>("LoomarrLegacyPairing");

const legacyPairingSource = {
  async read() {
    return (await nativeModule?.read()) ?? undefined;
  },
};

export { legacyPairingSource };
