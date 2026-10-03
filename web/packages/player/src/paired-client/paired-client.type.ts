import type { ClientDiagnosticsIdentity } from "@loomarr/core/client-diagnostics";
import type { GuideController, GuideControllerSnapshot } from "@loomarr/core/guide";
import type { MyChannelsController, MyChannelsSnapshot } from "@loomarr/core/my-channels";
import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";

import type { CatalogRefreshState } from "../catalog-refresher";
import type { NativePlayerTransport } from "../native/native";
import type { PlaybackMarks } from "../playback-marks";
import type { PlayerController, PlayerSnapshot, PlayerTuneReport } from "../player-controller";

interface PairedClientOptions {
  credential: PairingCredential;
  /**
   * Who reports playback diagnostics. Only a client the diagnostics API names can report, so a
   * client without one (the phone, until the API lists it) runs without them.
   */
  diagnostics?: ClientDiagnosticsIdentity;
  /** How much of the schedule the guide loads from now; the phone reads 2 hours, as the web phone does. */
  guideWindowMinutes?: number;
  /**
   * Whether the first catalog tunes its first channel. A client that shows no picture until the
   * viewer asks (the phone, opening on its Guide) passes "none" so no stream starts unwatched.
   */
  initialTune?: "first" | "none";
  /** Shield certification marks (#1037): the transport and the tunes write them when enabled. */
  marks?: PlaybackMarks;
  /** Every tune the controller starts; the latest function is always called. */
  onTune?: (report: PlayerTuneReport) => void;
  session: PairingSession;
}

/** One paired server's playback, guide and the viewer's channels, shared by the TV and the phone. */
interface PairedClient {
  catalogState: CatalogRefreshState;
  controller: PlayerController;
  guide: GuideController;
  guideSnapshot: GuideControllerSnapshot;
  myChannels: MyChannelsController;
  myChannelsSnapshot: MyChannelsSnapshot;
  /** Reload the channel catalog and the server's version, swallowing a failure the state shows. */
  refresh: () => void;
  serverVersion?: string;
  snapshot: PlayerSnapshot;
  transport: NativePlayerTransport;
}

export type { PairedClient, PairedClientOptions };
