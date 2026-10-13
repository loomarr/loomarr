import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import type { RequestsController } from "@loomarr/core/requests";

type PairedRequestsOptions = {
  credential: PairingCredential;
  session: PairingSession;
};

type PairedRequests = {
  controller: RequestsController;
  /** The Requests tab's badge: an admin's pending decisions plus anyone's failed requests. */
  needsYouCount: number;
};

export type { PairedRequests, PairedRequestsOptions };
