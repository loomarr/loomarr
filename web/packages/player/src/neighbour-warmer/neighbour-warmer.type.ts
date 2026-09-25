import type { DevicePlaybackProfile, PlayerChannel, PlayerSourcePort } from "../player-source";

interface NeighbourWarmerOptions {
  profile: DevicePlaybackProfile;
  /** How many channels each side of the current one to warm. The server still bounds admission. */
  radius?: number;
  source: Pick<PlayerSourcePort, "warm">;
}

interface NeighbourWarmer {
  /** Aborts every in-flight warm and starts nothing more. */
  cancel: () => void;
  /** Warms the neighbours of `currentId`, superseding whatever the previous call started. */
  retarget: (catalog: readonly PlayerChannel[], currentId: string) => void;
}

export type { NeighbourWarmer, NeighbourWarmerOptions };
