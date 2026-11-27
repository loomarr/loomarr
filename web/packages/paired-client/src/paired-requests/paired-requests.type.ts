import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import type { RequestsController } from "@loomarr/core/requests";
import type { PairedEvents } from "../paired-events";

type PairedRequestsOptions = {
  credential: PairingCredential;
  /** The device's stream to the server, from `usePairedClient`. */
  events: PairedEvents;
  session: PairingSession;
};

type PairedRequests = {
  controller: RequestsController;
  /** The Requests tab's badge: an admin's pending decisions plus anyone's failed requests. */
  needsYouCount: number;
};

export type { PairedRequests, PairedRequestsOptions };
