import type { ClientDiagnosticsIdentity } from "@loomarr/core/client-diagnostics";
import type { PairingCredential } from "@loomarr/core/pairing";
import type {
  CatalogRefresher,
  CatalogRefreshState,
  PlayerController,
  PlayerSnapshot,
  PlayerTuneReport,
} from "@loomarr/player";
import type { NativePlayerTransport, PlaybackMarks } from "@loomarr/player/native";
import type { LastChannelStore } from "../last-channel";

interface UsePlayerOptions {
  credential: Pick<PairingCredential, "serverUrl">;
  diagnostics?: ClientDiagnosticsIdentity;
  initialTune?: "first" | "none";
  lastChannel?: LastChannelStore;
  marks?: PlaybackMarks;
  onTune?: (report: PlayerTuneReport) => void;
  /** The paired, authenticated fetch every request of the client goes through. */
  request: typeof globalThis.fetch;
}

interface PlayerRuntime {
  catalogRefresher: CatalogRefresher;
  catalogState: CatalogRefreshState;
  controller: PlayerController;
  snapshot: PlayerSnapshot;
  transport: NativePlayerTransport;
}

export type { PlayerRuntime, UsePlayerOptions };
