import type { DevicePlaybackProfile, PlayerChannel, PlayerSource, PlayerSourcePort } from "../player-source";

type TuneDirection = -1 | 1;

type TuneReason = "catalog" | "channel" | "number" | "previous" | "retry" | "step";

type PlayerStatus = "empty" | "idle" | "tuning" | "playing" | "failed";

type PlayerTransportEvent =
  | { attemptId: number; type: "first-frame" }
  | { attemptId: number; error: string; type: "error" };

interface PlayerTransport {
  dispose: () => void;
  pause: () => void;
  play: () => Promise<void> | void;
  replace: (source: PlayerSource, context: { attemptId: number; signal: AbortSignal }) => Promise<void>;
  subscribe: (listener: (event: PlayerTransportEvent) => void) => () => void;
}

interface PlayerSnapshot {
  attemptId?: number;
  catalog: readonly PlayerChannel[];
  channel?: PlayerChannel;
  error?: string;
  previousChannelId?: string;
  recentChannelIds: readonly string[];
  status: PlayerStatus;
  tuneReason?: TuneReason;
}

interface PlayerController {
  dispose: () => void;
  getSnapshot: () => PlayerSnapshot;
  previous: () => Promise<void>;
  reconcile: (channels: readonly PlayerChannel[]) => Promise<void>;
  retry: () => Promise<void>;
  step: (direction: TuneDirection) => Promise<void>;
  subscribe: (listener: (snapshot: PlayerSnapshot) => void) => () => void;
  tuneChannel: (channelId: string) => Promise<void>;
  tuneNumber: (digits: string) => Promise<void>;
}

interface PlayerControllerOptions {
  initialTune?: "first" | "none";
  profile: DevicePlaybackProfile;
  source: PlayerSourcePort;
  transport: PlayerTransport;
}

export type {
  PlayerController,
  PlayerControllerOptions,
  PlayerSnapshot,
  PlayerStatus,
  PlayerTransport,
  PlayerTransportEvent,
  TuneDirection,
  TuneReason,
};
