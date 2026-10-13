import type { DevicePollOutputBody } from "@loomarr/api/models/devicePollOutputBody";
import type { DeviceStartOutputBody } from "@loomarr/api/models/deviceStartOutputBody";

type PairingCredential = { deviceName: string; serverUrl: string; token: string };

type PairingState =
  | { status: "loading" }
  | { status: "needs-server" }
  | {
      deviceCode: string;
      expiresAtMs: number;
      // True only for the code that immediately replaced one `begin()`'s poll loop found dead
      // (`poll()` returning `status: "expired"`), so the shell can say a new code just appeared
      // without the UI guessing it from the countdown. Cleared the moment the session emits any
      // other state — the next awaiting-approval included, if it was requested explicitly
      // (`session.pair()`/"Get a new code") rather than a silent expiry.
      refreshedFromExpired: boolean;
      serverUrl: string;
      status: "awaiting-approval";
      userCode: string;
      verificationUri: string;
      verificationUriComplete: string;
    }
  | ({ status: "paired" } & PairingCredential)
  | { serverUrl: string; status: "revoked" }
  | { message: string; retryable: boolean; serverUrl?: string; status: "failed" };

type PairingStart = { body: DeviceStartOutputBody; serverDate: string | undefined };
type PairingPoll =
  | { status: "pending" }
  | { status: "expired" }
  | { body: DevicePollOutputBody; status: "paired" };
type PairingTransport = {
  poll(deviceCode: string, signal: AbortSignal): Promise<PairingPoll>;
  start(deviceName: string, signal: AbortSignal): Promise<PairingStart>;
};
type PairingCredentialStore = {
  clear(): Promise<void>;
  read(): Promise<PairingCredential | undefined>;
  write(credential: PairingCredential): Promise<void>;
};
type PairingSessionOptions = {
  createTransport(serverUrl: string): PairingTransport;
  deviceName: string;
  now?: () => number;
  revokeCredential?: (credential: PairingCredential, signal: AbortSignal) => Promise<void>;
  sleep?: (milliseconds: number, signal: AbortSignal) => Promise<void>;
  store: PairingCredentialStore;
  validateCredential?: (credential: PairingCredential, signal: AbortSignal) => Promise<boolean>;
};

export type {
  PairingCredential,
  PairingCredentialStore,
  PairingPoll,
  PairingSessionOptions,
  PairingStart,
  PairingState,
  PairingTransport,
};
